package state

import (
	"testing"
	"time"

	"github.com/RyanRezaie/unified-dashboard-personal/internal/config"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/gab"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/homelab"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/model"
)

// The attention rule is the one piece of real logic in the backend, and it is
// the thing that decides whether the panel goes amber at 3am. These tests pin
// each of the four sources, and — just as importantly — pin the deliberate
// exclusion of approaching deadlines.

func testStore() *Store {
	return &Store{cfg: config.Config{
		AttentionContainers: []string{"qdrant"},
		AttentionGPUTempC:   80,
		AttentionDiskPct:    90,
	}}
}

func at(now time.Time, offset time.Duration) model.LocalTime {
	return model.LocalTime{Time: now.Add(offset)}
}

func TestAttentionQuietWhenHealthy(t *testing.T) {
	now := time.Now()
	g := gab.Snapshot{
		// Due in 3 hours: close, but NOT an attention source by design.
		Assignments: []model.Assignment{{ID: "a1", Course: "PHYS", Title: "lab", Due: at(now, 3*time.Hour)}},
		Reminders:   []model.Reminder{{ID: "r1", Text: "stretch", Due: at(now, time.Hour), Enabled: true}},
	}
	l := homelab.Snapshot{
		GPUs:       []model.GPU{{TempC: 61}},
		Containers: []model.Container{{Name: "qdrant", Running: true, Watched: true}},
		Disks:      []model.Disk{{Name: "nas", UsedPct: 44}},
	}

	got := testStore().attention(now, g, l)
	if got.Active {
		t.Fatalf("expected no attention, got %+v", got.Reasons)
	}
}

func TestAttentionApproachingDeadlineIsNotASource(t *testing.T) {
	now := time.Now()
	// An assignment due in ten minutes must still not raise attention: the
	// countdown already shows it, and including it would leave the panel amber
	// most of a normal week.
	g := gab.Snapshot{Assignments: []model.Assignment{
		{ID: "a1", Course: "PHYS", Title: "lab", Due: at(now, 10*time.Minute)},
	}}

	if got := testStore().attention(now, g, homelab.Snapshot{}); got.Active {
		t.Fatalf("approaching deadline must not raise attention, got %+v", got.Reasons)
	}
}

func TestAttentionOverdueReminder(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		rem  model.Reminder
		want bool
	}{
		{"overdue and pending", model.Reminder{Due: at(now, -time.Hour), Enabled: true}, true},
		{"overdue but acknowledged", model.Reminder{Due: at(now, -time.Hour), Enabled: true, Ack: true}, false},
		{"overdue but disabled", model.Reminder{Due: at(now, -time.Hour)}, false},
		{"not yet due", model.Reminder{Due: at(now, time.Hour), Enabled: true}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.rem.Text = "take out the trash"
			g := gab.Snapshot{Reminders: []model.Reminder{tc.rem}}
			if got := testStore().attention(now, g, homelab.Snapshot{}).Active; got != tc.want {
				t.Fatalf("attention = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAttentionOnlyWatchedContainersCount(t *testing.T) {
	now := time.Now()
	// An unwatched container being stopped is normal, not a fault.
	l := homelab.Snapshot{Containers: []model.Container{
		{Name: "scratch-vm", Status: "stopped", Running: false, Watched: false},
	}}
	if got := testStore().attention(now, gab.Snapshot{}, l); got.Active {
		t.Fatalf("unwatched container must not raise attention, got %+v", got.Reasons)
	}

	l.Containers[0].Watched = true
	got := testStore().attention(now, gab.Snapshot{}, l)
	if !got.Active || got.Reasons[0].Source != model.SourceContainer {
		t.Fatalf("watched container down should raise a container reason, got %+v", got)
	}
}

func TestAttentionThresholdsAreInclusive(t *testing.T) {
	now := time.Now()
	// Exactly at the threshold must fire — the spec says "at or above".
	l := homelab.Snapshot{
		GPUs:  []model.GPU{{Name: "rtx", TempC: 80}},
		Disks: []model.Disk{{Name: "nas", UsedPct: 90}},
	}
	got := testStore().attention(now, gab.Snapshot{}, l)
	if len(got.Reasons) != 2 {
		t.Fatalf("expected gpu + disk reasons at threshold, got %+v", got.Reasons)
	}

	l.GPUs[0].TempC = 79.9
	l.Disks[0].UsedPct = 89.9
	if got := testStore().attention(now, gab.Snapshot{}, l); got.Active {
		t.Fatalf("just under threshold must stay quiet, got %+v", got.Reasons)
	}
}

func TestAttentionReasonsAccumulate(t *testing.T) {
	now := time.Now()
	g := gab.Snapshot{Reminders: []model.Reminder{
		{Text: "trash", Due: at(now, -107*time.Minute), Enabled: true},
	}}
	l := homelab.Snapshot{
		GPUs:       []model.GPU{{Name: "rtx", TempC: 92}},
		Containers: []model.Container{{Name: "qdrant", Status: "stopped", Watched: true}},
		Disks:      []model.Disk{{Name: "nas", UsedPct: 95}},
	}

	got := testStore().attention(now, g, l)
	if len(got.Reasons) != 4 {
		t.Fatalf("expected all four sources, got %d: %+v", len(got.Reasons), got.Reasons)
	}
	// The NEEDS YOU panel prints these verbatim, so the wording matters.
	if want := "1h47m"; !contains(got.Reasons[0].Detail, want) {
		t.Errorf("overdue detail = %q, want it to contain %q", got.Reasons[0].Detail, want)
	}
}

func TestShortDur(t *testing.T) {
	cases := map[time.Duration]string{
		107 * time.Minute: "1h47m",
		12 * time.Minute:  "12m",
		60 * time.Minute:  "1h00m",
	}
	for in, want := range cases {
		if got := shortDur(in); got != want {
			t.Errorf("shortDur(%s) = %q, want %q", in, got, want)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestPrivateReminderTextIsRedactedServerSide(t *testing.T) {
	now := time.Now()
	secret := "pick up prescription"
	g := gab.Snapshot{Reminders: []model.Reminder{
		{Text: secret, Due: at(now, -30*time.Minute), Enabled: true, Private: true},
	}}

	got := testStore().attention(now, g, homelab.Snapshot{})
	if !got.Active {
		t.Fatal("a private reminder must still raise attention when overdue")
	}
	// The NEEDS YOU panel is monitor-only, so these words must never be sent.
	if contains(got.Reasons[0].Text, secret) {
		t.Errorf("private reminder text leaked into the attention reason: %q", got.Reasons[0].Text)
	}
	if !contains(got.Reasons[0].Detail, "AGO") {
		t.Errorf("the overdue duration should survive redaction, got %q", got.Reasons[0].Detail)
	}
}
