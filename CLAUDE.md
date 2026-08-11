# CLAUDE.md — Dashboard (parent repo)

## What this repo is
The unified semester/homelab dashboard, with related projects included as git
submodules. This file is for orientation across the whole tree — each submodule has its
own `CLAUDE.md` with real detail; don't duplicate that here, link to it.

## The dashboard app itself
A single-pane status view for Ryan's semester + homelab, LAN/Tailscale only, no login,
showing:
- Assignments due (read-only from G.A.B.'s data store)
- Homelab health (GPU temp/VRAM, container status)
- Application pipeline status (manual kanban: emailed → applied → interview → dead)

Design goal: answer "what's my situation right now" in under 5 seconds from any of Ryan's
three screens (iPhone 15+/GrapheneOS Pixel 8a, iPad A16, MacBook Pro). This is a glance
tool, not a place to do work.

### Stack decisions (fill in as decided — leave TODOs, don't assume)
- Backend language: **DECIDED — Go.** The dashboard is mostly a fan-out aggregator over
  Proxmox and the workstation GPU agent, and the GPU agent will most likely be Go too.
  Parity with G.A.B. only ever mattered for data G.A.B. owns, which this reads over HTTP.
  Standard library only — no third-party dependencies. See `README.md` to run it.
- Frontend: vanilla JS/HTML, matching `gab.html`'s style, unless Ryan asks for a framework.
  Lives in `web/static/`, embedded into the binary, so the binary is the whole deployment.
- Deployment: Docker container on Proxmox, exposed via Tailscale (`tailscale serve`),
  same pattern as Open WebUI
- Data sources:
  - G.A.B.: **DECIDED** — a new read-only `GET /api/assignments` endpoint on
    `gab_server.py`, local-only, never a direct read of `gab_data.json`. It must never
    write. Response shape is pinned in `docs/dashboard-ui.md`; how G.A.B. stores
    assignments behind it is still open. Don't duplicate reminder logic.
  - Proxmox: Proxmox API (token auth) for container/VM status
  - GPU: `nvidia-smi --query-gpu=... --format=csv` on the workstation, exposed via a
    small Tailscale-reachable agent — this dashboard itself should NOT run on the
    workstation, it belongs on the NAS/router box or a Proxmox LXC
  - Application pipeline: no external integration exists (Handshake has no public API);
    a static JSON file or small SQLite table edited via the UI is enough — don't
    over-engineer this piece

### Topology — DECIDED
Two machines:
- **Main PC** (RTX 5070 Ti) — runs the LLM and the GPU agent. Not always on.
- **Always-on server** — runs this dashboard, G.A.B., and everything else.

So the GPU agent is the one upstream that crosses machines, and the only one expected to
be legitimately offline day to day. It degrades the GPU lane alone, never the panel.

### Attention thresholds — DECIDED
GPU ≥ 80 °C, disk ≥ 85%, and the watched containers are `ollama`, `searxng`,
`open-webui`, `ntfy`, `gab` — expected to grow, which is why it is an env var and not a
code change. The dashboard's own container is deliberately not watched: if it is down
there is no panel to report it on.

### Ports — DECIDED
8080 is SearXNG and 8880 is ntfy. This stack lives in **8881-8889**:
`8881` dashboard · `8882` G.A.B. · `8883` GPU agent.

### Non-goals
- No auth system beyond Tailscale ACLs — don't build a login page
- No duplicate notifications — G.A.B. already pushes to ntfy/Gotify; this dashboard reads
  state, it doesn't re-notify
- No editing of tasks/reminders here — that stays G.A.B.'s job via voice; this is read-only
  plus the manual pipeline tracker

### Conventions to match G.A.B.'s codebase
- Atomic writes with `.bak` backup for any local JSON state (see `save_data()` in
  `gab_server.py`)
- Section-banner comments (`# ====...`) dividing major concerns
- Config constants (HOST, PORT, paths) declared at the top of the file, not scattered
- No hardcoded API keys — env vars or a gitignored local config file

### UI — decided, see `docs/dashboard-ui.md`
The UI design is settled. `docs/dashboard-ui-mockup.html` is the visual reference
(self-contained, open it in a browser); `docs/dashboard-ui.md` is the spec. Summary:
- **Two surfaces, one HTML file**, split by a width breakpoint — same shape `gab.html`
  uses for its TV case. An always-on monitor layout (1920×1080, never touched, no
  navigation, everything visible at once) and a phone layout (390×844, 64px left rail,
  four tabs). Nothing new visually — all tokens, type and idioms come from `gab.html`.
- **Attention rule**: overdue reminders + homelab faults. Deliberately NOT approaching
  deadlines — the countdown already shows those, and including them would leave the
  panel amber most of a normal week.
- **Refresh**: 20s data poll (matching `gab.html`'s `loadState`), 1s client-side tick
  for the countdown. Cheap — `gab_server.py` is a `ThreadingHTTPServer` already serving
  the HUD's 400ms status poll.
- **Burn-in mitigation is in scope** — slow layout drift + scheduled dim, as top-of-file
  constants. The panel shows this UI ~99% of the time.

### Open questions — mostly answered now, see the next section
- How G.A.B. stores assignments behind `/api/assignments` — **answered: derived from
  existing dated reminders.** No new type in `gab_data.json`. `course` is absent and
  is not parsed out of the reminder text; `done` comes from the daily task G.A.B.
  already creates from a dated reminder. The dashboard handles both already.
- **Reminders are specified on this side** — shape in `README.md`, including a
  `private` flag. Privacy is handled by surface, not by hiding data: the monitor is an
  always-on panel readable by anyone in the room, so a private reminder keeps its row,
  time and overdue tag there but not its words. The NEEDS YOU panel's copy is redacted
  server-side because that panel is monitor-only. **G.A.B. serves this now** at
  `/api/reminders?view=dashboard` (why the parameter: next section) — attention rule 1
  fires. What is still open is how a reminder gets *marked* private.
- **What else G.A.B. exposes besides assignments.** `/api/tasks` and
  `/api/objectives` are built now, so all four blocks populate. `internal/gab`
  still treats a 404 as "not implemented yet" rather than failing, which is what
  keeps the panel alive against an older G.A.B. Shapes are in `internal/model`,
  still marked `UNPINNED` — they are this side's proposal, and G.A.B. matched it.

## G.A.B.'s endpoints — WRITTEN, awaiting review

The G.A.B. half of the contract exists now. It lives in the **`gab-assistant`
submodule** on branch `claude/dashboard-endpoints` (pushed); this parent branch
carries the submodule pointer bump, this note, and the small dashboard-side
changes called out below. Four read-only endpoints, on G.A.B.'s server:

```
GET /api/assignments
GET /api/reminders?view=dashboard
GET /api/tasks
GET /api/objectives
```

Nothing new is stored on G.A.B.'s side. The full write-up is in the submodule's
`README.md` → "Dashboard endpoints"; what matters here is what got decided:

- **Assignments are dated reminders.** G.A.B. has no assignment type and did not
  grow one — a deadline there *is* a dated reminder, which is what a syllabus
  scan already creates. `course` is omitted rather than parsed out of the text,
  exactly as this file asked. Past deadlines drop off once their day is over,
  and a `private` dated reminder is left out of the list entirely (that shape
  has nowhere to hide text; it still appears, redacted, in the reminders list).
- **`?view=dashboard` — the one deviation from the pinned path.** `/api/reminders`
  already existed on G.A.B., serving its own HUD, and the two consumers disagree
  about `enabled`: the HUD draws an OFF tag from the stored flag, while attention
  rule 1 needs "still outstanding". G.A.B.'s scheduler disables a `once` reminder
  within twenty seconds of its deadline, so the stored flag alone would leave
  rule 1 unable to fire in practice — the exact thing this work was for. The
  HUD's meaning keeps the bare path; the dashboard's sits behind the parameter.
  **The dashboard side changed one line for this** — `pathReminders` in
  `internal/gab/gab.go`. That is the only dashboard change in this work.
- **`acknowledged` and `done` come from the daily task list.** G.A.B. has no
  acknowledge action, but `REMINDER_TASK_IMPORT` already puts a dated reminder
  on the daily list when its day arrives, and "Gab, mark the lab report as done"
  ticks it off. That tick is the only "I dealt with it" signal in the store.
- **`private` is set two ways, both confirmed by Ryan.** A spoken marker
  ("private reminder: …", "privately", "keep it private") that strips itself
  back out of the reminder text and gets confirmed out loud, and a
  HIDE/UNHIDE button on every row of G.A.B.'s HUD in EDIT mode, which is also
  the undo. Matching is lexical and whole-word — a bare "private" is
  deliberately not a marker. Default stays `false`.
- **The flag follows the reminder into the task list.** G.A.B. copies a dated
  reminder's text onto the daily list when its day arrives, so that task is
  marked private too and `model.Task` grew a `Private` field to carry it.
  `taskRow` redacts on the monitor exactly like `reminderRow`. Without that,
  a private reminder's words came back on the panel as an ordinary task.
- **`/api/tasks` and `/api/objectives` exist now**, so the DAILY and WEEKLY
  OBJECTIVES blocks populate instead of hiding. They are the same lists
  `/api/state` serves, reshaped — `priority` and `linked` dropped, objectives
  carrying `done`/`total`.

**G.A.B. moved to 8882.** It served 8420 for its whole history; Ryan asked for
the move, so `config.GABBaseURL` is correct as written now and its `CONFIRM:`
comment is answered. One consequence code cannot handle: G.A.B.'s Spotify
redirect URI is derived from its port but is *also* registered at
developer.spotify.com, so the 8882 URI has to be added there or that login
starts failing. Nothing on this side is affected.

**Verified end to end**, not against a stub, and with no `GAB_URL` override —
the default config points at the real G.A.B. now. All four blocks populate,
`gab_status.ok` is true, an overdue reminder raises attention, and a private
one comes through as "a private reminder is overdue" with the time intact.
`go vet`, `go test ./...` and `gofmt` are clean.

Both surfaces were also driven in a real browser (Playwright + the
pre-installed Chromium): the monitor never prints a private reminder's or a
private task's words, the phone prints both, and G.A.B.'s own HUD toggle marks
a row without also switching the reminder off on the way past.

### Merge order

The submodule pointer recorded here points at the branch commit in
`gab-assistant`. If that PR is squashed or rebased on merge, the SHA changes and
the pointer needs re-bumping to the merged commit before this branch goes in.

## Layout
```
dashboard/              ← this repo; the actual dashboard app (see its own build notes above)
├── gab-assistant/                ← submodule: G.A.B., the voice assistant (existing project,
│                          not started fresh here). See gab/CLAUDE.md.
├── notes-rag/           ← submodule: RAG-backed notes search/chat. See notes-rag/CLAUDE.md.
└── homelab-scheduler/    ← submodule, added later once Ryan starts it himself.
                            See homelab-scheduler/CLAUDE.md — deferred, do not build
                            from that file without explicit instruction.
```

## Working across submodules
- Each submodule is its own repo with its own history, remote, and `CLAUDE.md`. Changes
  inside `gab/` or `notes-rag/` are committed in that submodule, not in the parent.
- The parent repo only tracks *which commit* of each submodule it points at — after
  committing inside a submodule, the parent needs a follow-up commit recording the new
  submodule pointer (`git add gab && git commit -m "bump gab submodule"`).
- Freshly cloning this repo requires `git submodule update --init --recursive`, or the
  submodule directories will be empty and Claude Code won't see files inside them.

## Who's building this
Ryan hand-reviews every line across all of these — no unexplained blocks, no vibe coding.
He treats AI as a tutor/precision tool, not a code generator. This applies the same way
whether working in the parent dashboard code or inside a submodule.

## Cross-cutting integration points
- **Dashboard ↔ G.A.B.**: DECIDED — read-only HTTP, never a direct read of
  `gab_data.json`. `GET /api/assignments` and `GET /api/reminders?view=dashboard` on
  `gab_server.py`. The dashboard's client (`internal/gab`) is GET-only by construction.
  Both endpoints are written and verified end to end — see the section above for the
  shapes, the reason for the query parameter, and the three questions still open.
- **Dashboard ↔ notes-rag**: not yet integrated; the dashboard's current scope is
  assignments/homelab health/application pipeline, not notes search. If that changes,
  update this section.
- **G.A.B. ↔ notes-rag**: this is the `gab-voice-rag` extension work — lives as changes
  inside `gab/` itself (extending `gab_server.py`), not a separate submodule. See that
  project's `CLAUDE.md` for the integration design (G.A.B. calls out to notes-rag's API,
  doesn't embed its retrieval logic).

## Deployment
All pieces run as separate Docker containers on Proxmox, exposed via Tailscale
(`tailscale serve`), consistent with the rest of the homelab. This parent repo is a
source-control convenience, not a deployment unit — each submodule/service still gets
built and deployed independently.
