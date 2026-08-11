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

### Open questions to resolve before/while building
- How G.A.B. stores assignments behind `/api/assignments` — a new first-class type in
  `gab_data.json`, or derived from existing dated reminders? The UI is indifferent.
  **Partly answered:** G.A.B. today stores only a name and a due date, so `course` and
  `done` may simply be absent; the dashboard handles that already.
- **Reminders are specified on this side** — `GET /api/reminders`, shape in
  `README.md`, including a `private` flag. Privacy is handled by surface, not by
  hiding data: the monitor is an always-on panel readable by anyone in the room, so a
  private reminder keeps its row, time and overdue tag there but not its words. The
  NEEDS YOU panel's copy is redacted server-side because that panel is monitor-only.
  G.A.B. still has to grow the endpoint before attention rule 1 can fire.
- **What else G.A.B. exposes besides assignments.** Only `/api/assignments` was ever
  pinned, but the monitor shows DAILY / WEEKLY OBJECTIVES / REMINDERS blocks and attention
  rule 1 is defined in terms of overdue *reminders*. `internal/gab` asks for
  `/api/reminders`, `/api/tasks` and `/api/objectives` as **proposed** shapes and treats a
  404 as "not implemented yet" — the block hides instead of failing. Shapes are in
  `internal/model`, marked `UNPINNED`. Rule 1 can't fire until reminders exist.

## NEXT SESSION — write G.A.B.'s endpoints (not started)

Ryan approved this and it is the next piece of work. **Nothing below has been written
yet.** The dashboard side is finished and merged-ready; this is the G.A.B. half of the
contract that makes attention rule 1 able to fire at all.

### Branching
Ryan wants these reviewed and merged as two separate things:

- **`claude/claude-md-recent-commits-axn2v9`** holds all the dashboard work — the Go
  backend, the UI, tests, config, and this handoff. It is finished and pushed. **Do not
  add G.A.B. endpoint work to it.**
- **The G.A.B. work goes on a NEW branch in the parent repo** (suggested:
  `claude/gab-dashboard-endpoints`). Cut it from the **default branch**, not from the
  dashboard branch — otherwise it drags every dashboard commit along and stops being a
  separate reviewable change.
- The endpoint code itself lives in the **`gab-assistant` submodule**, in
  `gab_server.py`. That is a separate git repo with its own history and remote — commit
  it there, on its own branch in that repo. The parent repo only records *which commit*
  it points at, so the new parent branch gets one follow-up commit:
  `git add gab-assistant && git commit -m "bump gab-assistant submodule"`.

Note while both branches are unmerged: the contract this work implements (response
shapes, the `private` flag, the Privacy section) lives in `CLAUDE.md` and `README.md` on
the **dashboard** branch. If the new branch is cut from the default branch, those files
will not be there — read them from the dashboard branch, or from this section, which
restates everything needed below.

### Getting at the submodule
In a Claude Code web session the submodules are **not cloned** — `git submodule update
--init` fails because only `unified-dashboard-personal` is in the session's GitHub scope.
Attach it first with the `add_repo` tool (`owner: RyanRezaie`, `repo: gab-assistant`,
`access: push`), clone where it tells you, then `register_repo_root` so its own
`CLAUDE.md` loads. Do not skip that last step — G.A.B. has its own conventions file and
Ryan hand-reviews against it.

### What to build
Two **read-only** endpoints on `gab_server.py`, local-only, on **port 8882**. They must
never write. G.A.B. keeps owning the data; the dashboard owns the view.

**`GET /api/assignments`** — shape pinned in `docs/dashboard-ui.md`:
```json
{"generated_at":"2026-08-11T21:47:03-05:00",
 "assignments":[{"id":"a1","course":"PHYS 2425","title":"Lab report","due":"2026-08-12T09:00:00","done":false}]}
```
G.A.B. currently stores only a **name and a due date**, so `course` and `done` may be
omitted — the dashboard drops the course column for rows without one and defaults `done`
to false. Do **not** invent a course-parsing scheme to fill the field; if assignments are
just dated reminders today, derive them from those and leave `course` out.

**`GET /api/reminders`** — this is what attention rule 1 runs on:
```json
{"reminders":[{"id":"r1","text":"take out the trash","due":"2026-08-11T20:00:00",
  "enabled":true,"acknowledged":false,"repeat":"ONCE","private":false}]}
```

Both accept naive local timestamps (no offset) — the dashboard parses them in the
server's local zone, so G.A.B. does not need to emit offsets.

### The `private` flag — the part that matters most
Ryan cares greatly about privacy, and the real exposure is the always-on monitor, which
anyone in the room can read. `private: true` means the dashboard shows the reminder's
row, time and OVERDUE tag but **not its words** on the monitor; the phone still shows
them. The dashboard already implements both halves of this (see `README.md` → Privacy).

G.A.B.'s job is only to **set the flag honestly**. Decide with Ryan how a reminder gets
marked private — a voice phrase ("private reminder: …"), a stored field, or a keyword
list. **Do not guess this**; ask. Defaulting to `false` is safe; defaulting to `true`
would silently blank the panel.

### Conventions to follow inside `gab_server.py`
- `save_data()`'s atomic write with `.bak` backup for any state — but these endpoints
  should not be writing at all.
- Section-banner comments (`# ====...`) dividing major concerns.
- Config constants (HOST, PORT, paths) at the top of the file, not scattered.
- No hardcoded API keys — env vars or a gitignored local config.
- Don't duplicate reminder logic that already exists; read the existing store.

### How to verify
```sh
curl -s localhost:8882/api/assignments | python3 -m json.tool
curl -s localhost:8882/api/reminders   | python3 -m json.tool
# Then point the real dashboard at it — no stub:
GAB_URL=http://localhost:8882 go run ./cmd/dashboard   # → http://localhost:8881
```
Success looks like: the ASSIGNMENTS and REMINDERS blocks populate from real data, the
status strip reads `G.A.B. LINKED`, and an overdue reminder turns the core ring amber
and raises a NEEDS YOU row. A private one must show as `private reminder` on the monitor
and in full on the phone (narrow the window below 1100px).

### Still unanswered — ask, don't assume
1. **G.A.B.'s actual port.** 8882 is this project's reservation, not a port G.A.B. is
   known to listen on. Confirm before hardcoding.
2. **How a reminder gets marked private** (above).
3. **`/api/tasks` and `/api/objectives`** — still only proposed. The dashboard treats a
   404 as "not implemented" and hides that block, so they are genuinely optional. Build
   them only if Ryan wants the DAILY and WEEKLY OBJECTIVES blocks populated.

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
  `gab_data.json`. `GET /api/assignments` and `GET /api/reminders` on `gab_server.py`.
  The dashboard's client (`internal/gab`) is GET-only by construction. See the NEXT
  SESSION section above — those endpoints do not exist yet.
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
