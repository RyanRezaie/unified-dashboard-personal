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

### Open questions to resolve before/while building
- How G.A.B. stores assignments behind `/api/assignments` — a new first-class type in
  `gab_data.json`, or derived from existing dated reminders? The UI is indifferent.
- Which box does this actually run on — same LXC as G.A.B., or its own container?
- Real values for the attention thresholds and the watched-container list. The server
  logs a warning at startup while these are still the placeholders in
  `internal/config/config.go`, and attention rule 2 cannot fire until the watched-container
  list is set.
- **What else G.A.B. exposes besides assignments.** Only `/api/assignments` was ever
  pinned, but the monitor shows DAILY / WEEKLY OBJECTIVES / REMINDERS blocks and attention
  rule 1 is defined in terms of overdue *reminders*. `internal/gab` asks for
  `/api/reminders`, `/api/tasks` and `/api/objectives` as **proposed** shapes and treats a
  404 as "not implemented yet" — the block hides instead of failing. Shapes are in
  `internal/model`, marked `UNPINNED`. Rule 1 can't fire until reminders exist.

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
- **Dashboard ↔ G.A.B.**: dashboard reads assignment/reminder state from G.A.B. — see
  `dashboard`'s own notes above for whether that's a direct file read or an API endpoint
  on G.A.B.'s side (not yet decided as of writing).
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
