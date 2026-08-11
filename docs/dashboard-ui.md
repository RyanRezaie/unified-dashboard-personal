# Dashboard UI — design decisions

Companion to `dashboard-ui-mockup.html` in this folder (open it in a browser; it's
self-contained, fonts included). That file is the visual reference. This file is the
part a backend session needs.

Everything here was decided with Ryan on 2026-08-11. Anything still genuinely open is
in the last section, marked as such — don't resolve those by guessing.

## The core decision: two surfaces, one file

This is **not** one responsive layout. It's two surfaces with different jobs that happen
to ship in one HTML file, split by a width breakpoint — the same shape `gab.html` already
uses for its 2200px TV case.

| | Monitor | Phone |
|---|---|---|
| Context | Always-on panel, ~99% showing this UI | Held, operated |
| Interaction | **Never touched** | Tapped, navigated |
| Job | Everything visible at once | One concern at a time |
| Navigation | None — no tabs, no scroll, nothing below the fold | 64px left rail, 4 destinations |

The monitor gets no navigation *because* nobody operates it. The phone is free to show
less *because* the monitor already carries the always-everything job.

Drawn at **1920×1080** for the monitor and **390×844** for the phone.

## Design system

There is no new visual language here. Everything is lifted from `gab.html`:

- **Tokens** — `--cyan #2de8f7`, `--amber #ff9d3d`, `--link #b48cff` (purple),
  `--text #d7f7ff`, `--text-dim #5c8a96`, `--bg-0 #050b14`, `--bg-1 #0a1628`
- **Type** — Orbitron (display/labels/numerals, wide letter-spacing) + Share Tech Mono (body)
- **Semantic colour** — cyan = normal/system, amber = needs attention, purple = grouped
  (weekly objectives, pipeline). This mapping is already load-bearing in `gab.html`;
  keep it.
- **Idioms** — section labels with the glowing square bullet and underline rule; rows with
  a 3px left accent stripe; ring capsules using the `1fr auto 1fr` centring grid.

Two rules worth stating because they're easy to break:

- **Exactly one element on screen carries a filled background**, and it's the one waiting
  on a person (the NEEDS YOU panel). Same rule `gab.html` uses for its pending-import
  panel. It's absent entirely when nothing is wrong, so its presence *is* the signal.
- **The countdown lives in the monitor's header, not as a centred hero.** As a hero it
  was unbeatable for one question and wasted ~700px of height. In the header it's still
  the largest thing on the glass and the three lanes get the full height.

## Attention rule

Drives three things: the core ring's amber state, the phone rail's badge dot, and whether
the NEEDS YOU panel exists at all. **The dashboard is in ATTENTION state if any of:**

1. **A reminder is overdue** — due time passed, still enabled, not yet acknowledged.
2. **A watched container is not running.**
3. **GPU temp at or above threshold.**
4. **Disk at or above threshold.**

Not in the rule, deliberately: an approaching assignment deadline. The countdown already
shows that, and making it an attention source would leave the panel amber most of a normal
week — which is how a dashboard learns to cry wolf.

Thresholds belong at the top of the file as named constants, per the repo convention:

```
ATTENTION_CONTAINERS   = [...]   # NEEDS REAL VALUES — which are load-bearing?
ATTENTION_GPU_TEMP_C   = 80      # PLACEHOLDER
ATTENTION_DISK_PCT     = 90      # PLACEHOLDER
```

Do not ship the placeholders as if they were chosen. Ask.

## Refresh

**20-second poll for data, client-side tick for the countdown.**

- `gab.html` already polls `loadState` every 20s, with the comment *"pick up changes made
  from another device"* — that is this exact requirement, already solved.
- Worst-case staleness after marking an assignment done by voice: **20s**.
- The countdown and clock tick client-side every 1s at zero server cost. Do not conflate
  the two intervals — one is display, one is data.
- Cost is a non-issue: `gab_server.py` runs `ThreadingHTTPServer` and already serves the
  HUD's **400ms** `/api/ai/status` poll. A 20s poll is noise next to that.

## Burn-in

The panel is on this UI ~99% of the time (otherwise Proxmox or a terminal), so this is
worth building in rather than retrofitting. Both as top-of-file constants:

- **Slow layout drift** — shift the whole `.display` by a few pixels on a long cycle.
- **Scheduled dim** — reduce brightness after a configured hour.

## What the UI needs from the backend

### `GET /api/assignments` (new, on `gab_server.py`)

Decided: a **dedicated endpoint**, not a direct read of `gab_data.json`. Local-only.
**Read-only — this endpoint must never write.** G.A.B. keeps owning the data; the
dashboard owns the view. This mirrors the existing constraint in the parent `CLAUDE.md`
that the dashboard doesn't edit tasks or reminders.

The UI needs this shape. How G.A.B. *stores* assignments behind it is a backend call —
a new first-class type in `gab_data.json`, or derived from existing dated reminders —
and the UI is indifferent:

```json
{
  "generated_at": "2026-08-11T21:47:03-05:00",
  "assignments": [
    {
      "id": "a1",
      "course": "PHYS 2425",
      "title": "Lab report — projectile motion",
      "due": "2026-08-12T09:00:00",
      "done": false
    }
  ]
}
```

Notes that come from the mockups:

- `course` renders as its own fixed-width column, so it needs to be a **separate field**,
  not baked into the title string.
- `due` must carry a **time**, not just a date — the countdown is hours-precise and the
  monitor header shows `TOMORROW 09:00`.
- Sort order is the dashboard's business; return them in any order.

### Other sources (unchanged from the parent `CLAUDE.md`)

- **Homelab** — Proxmox API (token auth) for container/VM state; `nvidia-smi` on the
  workstation via a small Tailscale-reachable agent. The dashboard does **not** run on
  the workstation.
- **Pipeline** — no external integration exists. A local JSON file or small SQLite table
  edited through the UI. This is the **only** interactive part of the whole dashboard,
  which is why it's the only phone tab with a verb in its header.

## Still open — do not guess these

1. **How G.A.B. stores assignments** behind `/api/assignments`. Response shape above is
   pinned; storage is not.
2. **Which box the dashboard runs on** — its own LXC or alongside G.A.B. Listed as open
   in the parent `CLAUDE.md` and still unanswered.
3. **Real threshold values** for the attention rule, and the watched-container list.
4. **Backend language** — still `TODO` in the parent `CLAUDE.md`.
