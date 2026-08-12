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

## Voice control in Docker — DECIDED, written, untested on hardware

The `gab` image was API-and-HUD-only because "a container has no
microphone." That was a choice, not a law. Voice now runs in the container
via `docker-compose.voice.yml`, an override rather than a profile or a
second service — there can only ever be **one** G.A.B., since two of them
writing `gab_data.json` is a lost write, not redundancy.

What was decided, and why:

- **The host's audio socket, not `/dev/snd`.** `arecord -D plughw:0,0`
  takes the capture device exclusively, so the host loses its own
  microphone, and `plughw` indices follow USB enumeration order. The
  PipeWire PulseAudio socket (`$XDG_RUNTIME_DIR/pulse/native`) moves for
  neither reason. `ARECORD_DEVICE`/`APLAY_DEVICE` become `pulse`, which is
  configuration — **`gab_server.py` is untouched again**, same as the
  endpoints work.
- **A second image, `Dockerfile.voice`, on debian-slim.** Forced: CTranslate2
  and onnxruntime ship manylinux wheels only, and on musl both build from
  source. It installs `faster-whisper`, `piper-tts` and `numpy` — the three
  dependencies G.A.B.'s `CLAUDE.md` already accepts, no others.
- **The container runs as the socket's uid** (`HOST_UID`, default 1000).
  The socket is mode 0700, so a group cannot substitute. Consequence worth
  knowing: the API image runs as 10001 and chowns `/data`, so an existing
  `gab-data` volume needs a one-time `chown` when switching. The override's
  header has the command.
- **The single-writer hazard did not go away, it moved.** The host's
  `systemctl --user stop gab` is now a prerequisite, and it is the real
  reason voice and Docker used to be mutually exclusive — not the mic.

**Not verified against hardware.** No audio device or Docker daemon with
sound in the dev environment. What *is* verified: both entrypoint modes run,
the compose merge resolves correctly (context kept, dockerfile swapped,
`gab-data` preserved, socket and models appended), voice mode degrades to a
working-but-mute assistant when the models are absent rather than spinning
on `arecord`, and the default API-only path is unchanged. The socket path,
the uid match and the ALSA `pulse` plugin all want a real desk.

### Where this goes on k3s

Ryan's next step is k3s across the main PC, a few Raspberry Pis (one of them
holding the mic for the Home Assistant side) and the always-on server. This
design survives that: "the pod that listens runs on the node holding the
microphone" is a `nodeSelector` plus a `hostPath` mount of the same socket.

What will **not** survive is Whisper on a Pi — `base.en` on a Pi CPU is
slower than the speech it transcribes, which is why `GAB_STT_MODEL_SIZE` is
an env var. When that bites, split it rather than shrinking the model: keep
capture and playback on the Pi and POST to `/api/ai/stt`, `/api/ai/ask` and
`/api/ai/tts` on the always-on server. Those three endpoints already exist
and already speak exactly that protocol — the brain is network-shaped
already, only capture and playback are physically pinned.

### Music control in the container — playerctl

`playerctl` is installed in the voice image, and the D-Bus **session** bus is
mounted alongside the audio socket (`$XDG_RUNTIME_DIR/bus`). Installing the
binary alone would have been a no-op: MPRIS players advertise themselves on
that bus, and `MUSIC.available()` caches its answer for the life of the
process, so a missing bus is a permanent "music control is disabled" rather
than something that recovers.

What this means in practice: `gab, pause the music` reaches whatever is
playing **on the host**, because that is where the players are. Music control
therefore follows the desk, not the service — on a headless always-on server
there is no session bus and nothing playing, `MUSIC.available()` caches
False, and every other feature is unaffected. That is the correct outcome,
but it is worth knowing before wondering why it worked on the PC and not on
the server.

## GPU agent — BUILT, unverified against a real card

`cmd/gpu-agent` exists now, so `GPU_AGENT_URL` finally points at something.
It serves `GET /gpu` on **8883** in exactly the shape
`internal/homelab/gpu.go` had already pinned — that contract was written from
the dashboard side first, and the agent was built to it rather than the
reverse.

- **Deliberately not in the compose stack, and deliberately not a container.**
  It has to run where the card is (the main PC), which is the machine this
  dashboard is not allowed to run on. Reaching a GPU from a container needs
  the NVIDIA container toolkit and a device reservation that fails closed
  when the card is absent — real cost for a static binary that runs one
  command. `deploy/gpu-agent.service.example` is a systemd **user** unit;
  nvidia-smi needs no privilege.
- **One exec per request, no cache, no state.** The only client polls every
  20s. Caching, degradation and thresholds are all solved on the dashboard
  side already, and a second implementation would be a second place to fix.
- **It starts without a driver** and returns 503 until there is one. The main
  PC being asleep is the expected case, and the GPU lane degrading alone is
  the behaviour the panel was designed around.
- **`[N/A]` columns parse as 0, not as an error** — plenty of cards never
  report fan speed. The CSV is split from the *right* so a card name
  containing a comma cannot shift every column and report a fan speed as a
  temperature.

Verified against a fake `nvidia-smi` (correct JSON, both failure paths, the
503) and `go vet` / `go test ./...` / `gofmt` are clean. **Not yet run
against a real RTX 5070 Ti** — that needs the PC.

## NEXT SESSION — the pipeline tab has no controls on the monitor

Ryan reported there is no way to add or remove pipeline items. That turned
out to be **two separate things**, one of which is fixed:

- **The add form was unusable on the phone — FIXED.** It opened and vanished
  within a second, because the 1s display tick rebuilds `#p-content` and
  `phoneJobs()` re-emitted the form closed. The jobs branch now skips that
  rebuild while the form is open. Verified in a real browser against both
  builds. Nothing below depends on this; it was a genuine bug and it is out
  of the way now.
- **The monitor surface still has no controls at all** — the design question
  below, still open.

The controls there are **not missing, they are surface-split**. Full CRUD
exists on the **phone** surface only:

- `web/static/index.html:1024` — the `+ TRACK AN APPLICATION` button and the
  add form, rendered by `phoneJobs()`.
- `web/static/index.html:1057` — the click handler on `#p-content`: tapping a
  card advances its stage, `✕` retires it to DEAD, and `✕` again on a dead
  card deletes it after a confirm.
- The **monitor** surface renders `#m-pipeline` (`index.html:810`) as pure
  read-out — no add button, no `✕`, no handler bound to it at all.

So on a MacBook at >1100px, which is the monitor layout, there is nothing to
click. That is consistent with the design ("an always-on monitor, never
touched, no navigation") but it is clearly not what Ryan expected while
sitting at a laptop.

**Decide before building** — this is a design question, not a bug:

1. Leave the monitor read-only and improve discoverability instead (the
   phone layout is the editor; say so on the panel).
2. Give the monitor the same controls. Cheapest in code — the handler is
   already delegated and `pipelineWrite()` is surface-agnostic — but it puts
   click targets on a panel whose whole premise is that nobody touches it,
   and an accidental `✕` on an always-on display is a silent data loss.
3. Split by *input* rather than by width: enable the controls on the monitor
   layout only when the pointer is fine (`@media (pointer: fine)`), so a
   laptop gets them and the wall panel does not.

Option 3 is probably what Ryan actually wants, but it is his call. The
backend needs no work either way — `POST`/`PATCH`/`DELETE /api/pipeline` all
exist and are the only writes in the dashboard.

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
