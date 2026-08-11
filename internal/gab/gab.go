// Package gab is the read-only client for G.A.B.
//
// Hard constraint from CLAUDE.md and docs/dashboard-ui.md: this package issues
// GET requests and nothing else. G.A.B. owns the assignment/task/reminder data;
// the dashboard owns the view. There is deliberately no write path here to add
// to by accident.
package gab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/RyanRezaie/unified-dashboard-personal/internal/model"
)

// ============================================================
// ENDPOINTS
//
// Only /api/assignments is DECIDED (docs/dashboard-ui.md). The other three
// are needed by the monitor mockup — which shows DAILY, WEEKLY OBJECTIVES and
// REMINDERS blocks, and whose attention rule is defined in terms of overdue
// *reminders* — but no shape for them has been agreed with Ryan yet.
//
// So they are treated as optional: a 404 from any of them degrades that one
// block to empty and is not an error. Assignments failing IS an error.
// ============================================================

const (
	pathAssignments = "/api/assignments" // DECIDED
	// The bare /api/reminders already existed on G.A.B., serving its own
	// HUD, and the two disagree about one field: there, `enabled` is the
	// stored value (it draws an OFF tag from it), while the attention rule
	// here needs "still outstanding" — a dated reminder is disabled by
	// G.A.B.'s scheduler seconds after it fires, long before Ryan has done
	// the thing. `?view=dashboard` selects this side's reading. See the
	// dashboard section of gab_server.py.
	pathReminders  = "/api/reminders?view=dashboard" // PROPOSED — optional
	pathTasks      = "/api/tasks"                    // PROPOSED — optional
	pathObjectives = "/api/objectives"               // PROPOSED — optional
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

// Snapshot is everything the dashboard reads from G.A.B. in one refresh.
type Snapshot struct {
	Assignments []model.Assignment `json:"assignments"`
	Reminders   []model.Reminder   `json:"reminders"`
	Tasks       []model.Task       `json:"tasks"`
	Objectives  []model.Objective  `json:"objectives"`

	// Optional reports which of the PROPOSED endpoints were actually
	// present, so the UI can hide a block instead of rendering an empty one
	// that looks like "nothing due" when it really means "not implemented".
	Have map[string]bool `json:"have"`

	Status model.SourceStatus `json:"status"`
}

// Fetch pulls the whole snapshot. The assignments call decides success; the
// optional calls only add to it.
func (c *Client) Fetch(ctx context.Context) Snapshot {
	snap := Snapshot{
		Have:   map[string]bool{},
		Status: model.SourceStatus{FetchedAt: time.Now()},
	}

	var envelope struct {
		GeneratedAt string             `json:"generated_at"`
		Assignments []model.Assignment `json:"assignments"`
	}
	if err := c.get(ctx, pathAssignments, &envelope); err != nil {
		snap.Status.Error = err.Error()
		return snap
	}
	snap.Assignments = envelope.Assignments
	snap.Status.OK = true
	snap.Have["assignments"] = true

	// Optional blocks. Errors here are swallowed on purpose — see the
	// endpoint comment above. They are visible as Have[x] == false.
	var reminders struct {
		Reminders []model.Reminder `json:"reminders"`
	}
	if err := c.get(ctx, pathReminders, &reminders); err == nil {
		snap.Reminders = reminders.Reminders
		snap.Have["reminders"] = true
	}

	var tasks struct {
		Tasks []model.Task `json:"tasks"`
	}
	if err := c.get(ctx, pathTasks, &tasks); err == nil {
		snap.Tasks = tasks.Tasks
		snap.Have["tasks"] = true
	}

	var objectives struct {
		Objectives []model.Objective `json:"objectives"`
	}
	if err := c.get(ctx, pathObjectives, &objectives); err == nil {
		snap.Objectives = objectives.Objectives
		snap.Have["objectives"] = true
	}

	return snap
}

// get is the only request path in this package, and it is GET-only by
// construction — there is no method parameter to pass something else through.
func (c *Client) get(ctx context.Context, path string, into any) error {
	if c.baseURL == "" {
		return fmt.Errorf("gab: no GAB_URL configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gab: %s unreachable: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gab: %s returned %s", path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("gab: %s bad json: %w", path, err)
	}
	return nil
}
