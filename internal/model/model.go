// Package model holds the wire types shared by every source and the API layer.
//
// These are deliberately in their own package so that the source packages
// (gab, homelab, pipeline) and the API layer can all speak the same shapes
// without importing each other.
package model

import (
	"fmt"
	"strings"
	"time"
)

// ============================================================
// TIME — G.A.B. sends naive local timestamps
// ============================================================

// LocalTime accepts both an RFC3339 timestamp with an offset
// ("2026-08-11T21:47:03-05:00") and a naive one without ("2026-08-12T09:00:00").
//
// This matters because the two examples pinned in docs/dashboard-ui.md use
// *different* formats: generated_at carries an offset, assignment.due does not.
// A naive timestamp is interpreted in the server's local zone, which is correct
// here because G.A.B. and the dashboard run on the same LAN in the same zone —
// if that ever stops being true, this is the one place to fix it.
type LocalTime struct{ time.Time }

// layouts are tried in order. Date-only is accepted because an assignment
// entered by voice as "due Friday" may plausibly land without a time, even
// though the spec asks G.A.B. for hours-precision.
var layouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02",
}

func (lt *LocalTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		lt.Time = time.Time{}
		return nil
	}
	for _, layout := range layouts {
		// ParseInLocation, not Parse: Parse would silently pin a naive
		// timestamp to UTC and shift every countdown by the zone offset.
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			lt.Time = t
			return nil
		}
	}
	return fmt.Errorf("model: cannot parse time %q", s)
}

// MarshalJSON always emits RFC3339 so the browser's Date() parses it
// unambiguously — the naive form is an input concession, not an output one.
func (lt LocalTime) MarshalJSON() ([]byte, error) {
	if lt.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + lt.Format(time.RFC3339) + `"`), nil
}

// ============================================================
// G.A.B. — assignments, tasks, reminders, objectives
// ============================================================

// Assignment matches the response shape pinned in docs/dashboard-ui.md.
// Course is its own field because the monitor renders it as a fixed-width
// column; it must not be baked into Title.
//
// CURRENT REALITY: G.A.B. today stores only a name and a due date — there is
// no course and no done flag behind /api/assignments yet. Both stay in the
// shape because the pinned contract has them and they cost nothing when
// absent: Course empty means the UI drops the column for that row rather than
// rendering a blank one, and Done defaults false. Nothing here requires
// G.A.B. to grow those fields before the dashboard is useful.
type Assignment struct {
	ID     string    `json:"id"`
	Course string    `json:"course"`
	Title  string    `json:"title"`
	Due    LocalTime `json:"due"`
	Done   bool      `json:"done"`
}

// Reminder is an attention source (rule 1: overdue reminders).
//
// UNPINNED SHAPE — docs/dashboard-ui.md pins /api/assignments only, but the
// monitor mockup shows a REMINDERS block and the attention rule is defined in
// terms of reminders. This is the shape the dashboard asks for; if G.A.B.
// exposes something different, change it here and nowhere else.
type Reminder struct {
	ID      string    `json:"id"`
	Text    string    `json:"text"`
	Due     LocalTime `json:"due"`
	Enabled bool      `json:"enabled"`
	Ack     bool      `json:"acknowledged"`
	Repeat  string    `json:"repeat"` // "", "ONCE", "DAILY", "WEEKDAYS", ...

	// Private marks a reminder whose TEXT must not appear on the monitor.
	//
	// The monitor is an always-on panel in a room: anything on it is readable
	// by whoever walks past, for as long as it is up. The phone is held. That
	// asymmetry is the whole privacy surface of this dashboard, and it is why
	// the flag lives on the reminder rather than in a global setting — "pick
	// up prescription" and "take out the trash" do not want the same handling.
	//
	// A private reminder still occupies its row, still counts, and still
	// raises attention when overdue. Only the words are withheld, and only on
	// the monitor. See REDACTED_TEXT in internal/state.
	Private bool `json:"private"`
}

// Overdue implements attention rule 1 verbatim: due time passed, still
// enabled, not yet acknowledged.
func (r Reminder) Overdue(now time.Time) bool {
	return r.Enabled && !r.Ack && !r.Due.IsZero() && r.Due.Before(now)
}

// Task is a daily task. UNPINNED SHAPE — see Reminder.
type Task struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// Objective is a weekly objective with subtasks. UNPINNED SHAPE — see Reminder.
type Objective struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
	Subs  []Task `json:"subs"`
}

// ============================================================
// HOMELAB — GPU, containers, storage
// ============================================================

// GPU is one card as reported by the workstation agent. Fields map 1:1 onto
// nvidia-smi --query-gpu=name,temperature.gpu,fan.speed,memory.used,memory.total
type GPU struct {
	Name        string  `json:"name"`
	TempC       float64 `json:"temp_c"`
	FanPct      float64 `json:"fan_pct"`
	VRAMUsedMB  float64 `json:"vram_used_mb"`
	VRAMTotalMB float64 `json:"vram_total_mb"`
}

// Container is a Proxmox LXC or VM. Running is derived from Status rather than
// reported separately, so the UI never has to know Proxmox's status vocabulary.
type Container struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // proxmox: "running" | "stopped"
	Running bool   `json:"running"`
	Node    string `json:"node"`
	VMID    int    `json:"vmid"`
	Watched bool   `json:"watched"` // in config.AttentionContainers
}

// Disk is one storage pool.
type Disk struct {
	Name       string  `json:"name"`
	UsedPct    float64 `json:"used_pct"`
	UsedBytes  int64   `json:"used_bytes"`
	TotalBytes int64   `json:"total_bytes"`
}

// ============================================================
// PIPELINE — the one editable surface
// ============================================================

// Stage is an application pipeline stage. The four values are fixed by the
// UI (four stage blocks, in this order); this is not a user-defined list.
type Stage string

const (
	StageEmailed   Stage = "emailed"
	StageApplied   Stage = "applied"
	StageInterview Stage = "interview"
	StageDead      Stage = "dead"
)

// Stages is display order: live stages first, dead last, matching the mockup.
var Stages = []Stage{StageInterview, StageApplied, StageEmailed, StageDead}

func (s Stage) Valid() bool {
	for _, known := range Stages {
		if s == known {
			return true
		}
	}
	return false
}

// PipelineItem is one tracked application.
type PipelineItem struct {
	ID        string    `json:"id"`
	Company   string    `json:"company"`
	Role      string    `json:"role"`
	Stage     Stage     `json:"stage"`
	Note      string    `json:"note,omitempty"`
	AddedAt   time.Time `json:"added_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ============================================================
// ATTENTION
// ============================================================

// AttentionSource identifies which of the four rules fired, so the UI can
// route a reason to the right tab badge without string-matching the text.
type AttentionSource string

const (
	SourceReminder  AttentionSource = "reminder"
	SourceContainer AttentionSource = "container"
	SourceGPUTemp   AttentionSource = "gpu_temp"
	SourceDisk      AttentionSource = "disk"
)

// AttentionReason is one row of the NEEDS YOU panel: a headline and a
// smaller detail line, exactly as the mockup renders them.
type AttentionReason struct {
	Source AttentionSource `json:"source"`
	Text   string          `json:"text"`
	Detail string          `json:"detail"`
}

// Attention is the whole rule's output. Active is len(Reasons) > 0 — it is
// sent explicitly so the UI never has to re-derive the rule client-side.
type Attention struct {
	Active  bool              `json:"active"`
	Reasons []AttentionReason `json:"reasons"`
}

// ============================================================
// SOURCE HEALTH
// ============================================================

// SourceStatus wraps every remote source so one dead dependency degrades a
// single lane instead of blanking the panel. The status strip at the bottom
// of the monitor reads these.
type SourceStatus struct {
	OK        bool      `json:"ok"`
	Error     string    `json:"error,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
}
