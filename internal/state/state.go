// Package state builds the single snapshot the UI polls, and implements the
// attention rule.
//
// The UI makes one request every 20s and gets everything. Upstreams are
// refreshed on the server's own timer (config.RefreshInterval) so a slow
// Proxmox call never makes a UI poll hang — the poll always reads the last
// good snapshot from memory.
package state

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/RyanRezaie/unified-dashboard-personal/internal/config"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/gab"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/homelab"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/model"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/pipeline"
)

// Snapshot is the whole /api/state response.
type Snapshot struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Attention   model.Attention `json:"attention"`

	Assignments []model.Assignment `json:"assignments"` // sorted soonest-due first
	Reminders   []model.Reminder   `json:"reminders"`
	Tasks       []model.Task       `json:"tasks"`
	Objectives  []model.Objective  `json:"objectives"`
	Have        map[string]bool    `json:"have"`

	Homelab homelab.Snapshot `json:"homelab"`

	Pipeline map[model.Stage][]model.PipelineItem `json:"pipeline"`

	GABStatus model.SourceStatus `json:"gab_status"`
}

// ============================================================
// STORE — holds the last good upstream reads
// ============================================================

type Store struct {
	cfg      config.Config
	gab      *gab.Client
	homelab  *homelab.Client
	pipeline *pipeline.Store

	mu      sync.RWMutex
	gabSnap gab.Snapshot
	labSnap homelab.Snapshot
}

func NewStore(cfg config.Config, g *gab.Client, h *homelab.Client, p *pipeline.Store) *Store {
	return &Store{cfg: cfg, gab: g, homelab: h, pipeline: p}
}

// Refresh pulls both upstreams. G.A.B. and the homelab are independent, so a
// failure in one leaves the other's previous data untouched.
func (s *Store) Refresh(ctx context.Context) {
	if s.cfg.Stub {
		now := time.Now()
		s.mu.Lock()
		s.gabSnap = gab.StubSnapshot(now)
		s.labSnap = homelab.StubSnapshot(now, s.cfg.AttentionContainers)
		s.mu.Unlock()
		return
	}

	gabSnap := s.gab.Fetch(ctx)
	labSnap := s.homelab.Fetch(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()
	// Only replace on success. A transient upstream blip should leave the last
	// known values on the glass (marked stale by the status strip) rather than
	// emptying a lane.
	if gabSnap.Status.OK {
		s.gabSnap = gabSnap
	} else {
		s.gabSnap.Status = gabSnap.Status
	}
	if labSnap.ProxmoxStatus.OK {
		s.labSnap.Containers, s.labSnap.Disks = labSnap.Containers, labSnap.Disks
	}
	s.labSnap.ProxmoxStatus = labSnap.ProxmoxStatus
	if labSnap.GPUStatus.OK {
		s.labSnap.GPUs = labSnap.GPUs
	}
	s.labSnap.GPUStatus = labSnap.GPUStatus
}

// Run refreshes immediately, then on a ticker until ctx is cancelled.
func (s *Store) Run(ctx context.Context) {
	s.refreshOnce(ctx)

	ticker := time.NewTicker(s.cfg.RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshOnce(ctx)
		}
	}
}

func (s *Store) refreshOnce(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, s.cfg.UpstreamTimeout)
	defer cancel()
	s.Refresh(ctx)
}

// ============================================================
// SNAPSHOT
// ============================================================

func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	gabSnap, labSnap := s.gabSnap, s.labSnap
	s.mu.RUnlock()

	now := time.Now()

	assignments := make([]model.Assignment, len(gabSnap.Assignments))
	copy(assignments, gabSnap.Assignments)
	// Sort order is the dashboard's business (docs/dashboard-ui.md): soonest
	// due first, done items last regardless of date.
	sort.SliceStable(assignments, func(i, j int) bool {
		if assignments[i].Done != assignments[j].Done {
			return !assignments[i].Done
		}
		return assignments[i].Due.Before(assignments[j].Due.Time)
	})

	reminders := make([]model.Reminder, len(gabSnap.Reminders))
	copy(reminders, gabSnap.Reminders)
	sort.SliceStable(reminders, func(i, j int) bool {
		return reminders[i].Due.Before(reminders[j].Due.Time)
	})

	return Snapshot{
		GeneratedAt: now,
		Attention:   s.attention(now, gabSnap, labSnap),
		Assignments: assignments,
		Reminders:   reminders,
		Tasks:       gabSnap.Tasks,
		Objectives:  gabSnap.Objectives,
		Have:        gabSnap.Have,
		Homelab:     labSnap,
		Pipeline:    s.pipeline.ByStage(),
		GABStatus:   gabSnap.Status,
	}
}

// ============================================================
// ATTENTION RULE — docs/dashboard-ui.md
//
// ATTENTION if any of:
//   1. a reminder is overdue (passed, enabled, unacknowledged)
//   2. a watched container is not running
//   3. GPU temp >= threshold
//   4. disk >= threshold
//
// Explicitly NOT a source: an approaching assignment deadline. The countdown
// already shows that, and including it would leave the panel amber most of a
// normal week.
// ============================================================

func (s *Store) attention(now time.Time, g gab.Snapshot, l homelab.Snapshot) model.Attention {
	var reasons []model.AttentionReason

	// 1 — overdue reminders
	for _, r := range g.Reminders {
		if !r.Overdue(now) {
			continue
		}
		reasons = append(reasons, model.AttentionReason{
			Source: model.SourceReminder,
			Text:   fmt.Sprintf("%q overdue", r.Text),
			Detail: fmt.Sprintf("DUE %s · %s AGO", r.Due.Format("15:04"), shortDur(now.Sub(r.Due.Time))),
		})
	}

	// 2 — watched containers that are down
	for _, c := range l.Down() {
		reasons = append(reasons, model.AttentionReason{
			Source: model.SourceContainer,
			Text:   fmt.Sprintf("%s is down", c.Name),
			Detail: fmt.Sprintf("%s · NODE %s", upper(c.Status), upper(c.Node)),
		})
	}

	// 3 — GPU temperature
	for _, gpu := range l.GPUs {
		if gpu.TempC < s.cfg.AttentionGPUTempC {
			continue
		}
		reasons = append(reasons, model.AttentionReason{
			Source: model.SourceGPUTemp,
			Text:   fmt.Sprintf("GPU at %.0f °C", gpu.TempC),
			Detail: fmt.Sprintf("THRESHOLD %.0f °C · %s", s.cfg.AttentionGPUTempC, upper(gpu.Name)),
		})
	}

	// 4 — disk usage
	for _, d := range l.Disks {
		if d.UsedPct < s.cfg.AttentionDiskPct {
			continue
		}
		reasons = append(reasons, model.AttentionReason{
			Source: model.SourceDisk,
			Text:   fmt.Sprintf("%s at %.0f%% full", d.Name, d.UsedPct),
			Detail: fmt.Sprintf("THRESHOLD %.0f%%", s.cfg.AttentionDiskPct),
		})
	}

	return model.Attention{Active: len(reasons) > 0, Reasons: reasons}
}

// shortDur renders "1h47m" / "12m" the way the mockup's OVERDUE tag does.
func shortDur(d time.Duration) string {
	d = d.Round(time.Minute)
	if h := int(d.Hours()); h > 0 {
		return fmt.Sprintf("%dh%02dm", h, int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

func upper(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - 32
		}
	}
	return string(out)
}
