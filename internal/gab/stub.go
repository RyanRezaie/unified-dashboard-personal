package gab

import (
	"time"

	"github.com/RyanRezaie/unified-dashboard-personal/internal/model"
)

// ============================================================
// STUB — fixture data for developing the UI without G.A.B. running.
//
// Enabled only by DASHBOARD_STUB=1. It is never a fallback for a failed
// live fetch: a dead source must look dead on the glass, not healthy with
// invented numbers.
//
// The values mirror docs/dashboard-ui-mockup.html so the real UI can be
// eyeballed against the mockup side by side.
// ============================================================

func StubSnapshot(now time.Time) Snapshot {
	day := func(d int, hour, min int) model.LocalTime {
		t := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.Local)
		return model.LocalTime{Time: t.AddDate(0, 0, d)}
	}

	return Snapshot{
		Assignments: []model.Assignment{
			{ID: "a1", Course: "PHYS 2425", Title: "Lab report — projectile motion", Due: day(1, 9, 0)},
			{ID: "a2", Course: "MATH 2415", Title: "Problem set 6 — line integrals", Due: day(3, 23, 59)},
			{ID: "a3", Course: "ELEN 1301", Title: "Circuit simulation writeup", Due: day(4, 23, 59)},
			{ID: "a4", Course: "MATH 3370", Title: "Proof set — induction", Due: day(7, 23, 59)},
			{ID: "a5", Course: "PHYS 2425", Title: "Ch. 9 homework set", Due: day(8, 23, 59)},
			{ID: "a6", Course: "ELEN 1301", Title: "Breadboard practical prep", Due: day(9, 23, 59)},
		},
		Reminders: []model.Reminder{
			// Overdue on purpose: exercises attention rule 1 and the NEEDS YOU panel.
			{ID: "r1", Text: "take out the trash", Due: model.LocalTime{Time: now.Add(-107 * time.Minute)}, Enabled: true, Repeat: "ONCE"},
			{ID: "r2", Text: "PHYS 2425 lab report due", Due: day(1, 9, 0), Enabled: true, Repeat: "ONCE"},
			{ID: "r3", Text: "stand up and stretch", Due: day(1, 14, 0), Enabled: true, Repeat: "WEEKDAYS"},
			// Private: the monitor shows the time and the row, never the words.
			{ID: "r4", Text: "pick up prescription", Due: day(0, 17, 30), Enabled: true, Repeat: "ONCE", Private: true},
		},
		Tasks: []model.Task{
			{ID: "t1", Text: "Reply to advisor email", Done: true},
			{ID: "t2", Text: "Review lab report draft"},
			{ID: "t3", Text: "Rebuild Piper voice cache"},
			{ID: "t4", Text: "Order 0.1µF caps"},
		},
		Objectives: []model.Objective{
			{ID: "o1", Title: "Coursework", Done: 2, Total: 5, Subs: []model.Task{
				{ID: "o1s1", Text: "Signals problem set", Done: true},
				{ID: "o1s2", Text: "Lab report — projectile motion"},
				{ID: "o1s3", Text: "Read ch. 9 before Thursday"},
			}},
			{ID: "o2", Title: "Homelab", Done: 3, Total: 4, Subs: []model.Task{
				{ID: "o2s1", Text: "Bring qdrant back up"},
			}},
			{ID: "o3", Title: "Applications", Done: 1, Total: 3, Subs: []model.Task{
				{ID: "o3s1", Text: "Prep TI interview"},
				{ID: "o3s2", Text: "Follow up with Lockheed"},
			}},
		},
		Have:   map[string]bool{"assignments": true, "reminders": true, "tasks": true, "objectives": true},
		Status: model.SourceStatus{OK: true, FetchedAt: now},
	}
}
