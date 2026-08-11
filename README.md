# Dashboard

The unified semester/homelab dashboard. One glance answers "what's my situation
right now": assignments due, homelab health, application pipeline.

Design is settled in [`docs/dashboard-ui.md`](docs/dashboard-ui.md), with
[`docs/dashboard-ui-mockup.html`](docs/dashboard-ui-mockup.html) as the visual
reference. This file covers how to run it.

## Run it

```sh
# Real data. Needs G.A.B. on GAB_URL; Proxmox and the GPU agent degrade to a
# failed lane if unset rather than taking the panel down.
go run ./cmd/dashboard

# Fixture data — no G.A.B., no Proxmox, no GPU agent needed.
DASHBOARD_STUB=1 go run ./cmd/dashboard
```

Then open **http://localhost:8881**. The startup line says the same thing, but
note that the *bind* address is `0.0.0.0` — that is a wildcard meaning "every
interface", not somewhere a browser can go, and Chrome rejects it outright.
Bound that way, the panel is also reachable at this machine's LAN and
Tailscale addresses, which is the point.

**`DASHBOARD_STUB=1` serves the mockup's fixture data and contacts nothing.**
That is what it is for — UI work without the homelab running — but it means a
stubbed panel looks like `docs/dashboard-ui-mockup.html` no matter what G.A.B.
is doing. If the numbers on screen never change, check that flag first.

Resize the window past 1100px to switch between the two surfaces: the monitor
layout above, the phone layout below.

```sh
go test ./...          # attention rule + pipeline store
docker compose up -d   # deployment shape (see docker-compose.yml)
```

## Configuration

Every value has a default in `internal/config/config.go` and an env override.
No secret has a default.

| Env | Default | What |
|---|---|---|
| `DASHBOARD_HOST` / `DASHBOARD_PORT` | `0.0.0.0:8881` | Bind address |
| `GAB_URL` | `http://127.0.0.1:8882` | G.A.B. — assignments, reminders, tasks, objectives |
| `GPU_AGENT_URL` | — | Workstation agent wrapping `nvidia-smi` |
| `PROXMOX_URL` / `PROXMOX_NODE` | — | Proxmox API |
| `PROXMOX_TOKEN_ID` / `PROXMOX_TOKEN_SECRET` | — | API token; **env only** |
| `PROXMOX_INSECURE` | `true` | Self-signed homelab cert |
| `PIPELINE_PATH` | `./data/pipeline.json` | The only writable state |
| `ATTENTION_CONTAINERS` | `ollama,searxng,open-webui,ntfy,gab` | Watched services; grow as needed |
| `ATTENTION_GPU_TEMP_C` | `80` | Fires at or above |
| `ATTENTION_DISK_PCT` | `85` | Fires at or above |
| `DRIFT_PX`, `DRIFT_PERIOD_MIN`, `DIM_START_HOUR`, `DIM_END_HOUR`, `DIM_OPACITY` | `6`, `17`, `23`, `7`, `0.55` | Burn-in mitigation |
| `DASHBOARD_STUB` | `false` | Fixture data for UI work |

`TZ` matters: G.A.B. sends naive local timestamps and they are parsed in the
server's local zone. A container without `TZ` set runs UTC and every countdown
is wrong by the offset.

## HTTP surface

| Route | |
|---|---|
| `GET /` | The UI — both surfaces, one file, embedded in the binary |
| `GET /api/state` | Everything the UI polls, once every 20s |
| `GET /api/config` | Poll interval, burn-in constants, thresholds |
| `GET /api/pipeline` | Flat list of tracked applications |
| `POST /api/pipeline` | Add one |
| `PATCH /api/pipeline/{id}` | Change stage or fields |
| `DELETE /api/pipeline/{id}` | Remove one |
| `GET /healthz` | Liveness |

The pipeline routes are the only writes in the whole dashboard. Nothing here
writes to G.A.B. — `internal/gab` has no write path to add to by accident.

## How it fits together

```
  ALWAYS-ON SERVER                          MAIN PC (RTX 5070 Ti)
  ┌───────────────────────────┐             ┌──────────────────┐
  │ G.A.B.      :8882 ───────┐│             │ LLM              │
  │ Proxmox API :8006 ──────┐││             │ GPU agent :8883 ─┼──┐
  │                        ┌▼▼▼─────────┐   └──────────────────┘  │
  │                        │ dashboard  │◀─── Tailscale ──────────┘
  │                        │   :8881    │
  │                        └─────┬──────┘
  │                        pipeline.json  ← the only thing it writes
  └───────────────────────────┘
```

The GPU agent is the only upstream that crosses machines, and the only one
expected to be offline routinely — whenever the main PC is off. It degrades
the GPU lane alone.

Upstreams refresh on the server's own 10s timer; the UI's 20s poll only reads
memory, so a slow Proxmox call can never stall a poll. Each source carries its
own status, so one dead dependency degrades one lane instead of blanking the
panel — and a failed source shows as failed rather than falling back to stubs.

## Privacy

The dashboard makes **no outbound connections**. It talks to G.A.B., Proxmox
and the GPU agent, all on the LAN/Tailscale, and to nothing else. No
telemetry, no analytics, no CDN — the fonts are embedded and served from this
origin, so the panel works with the internet unplugged. There are no
third-party Go dependencies, so nothing else ships in the binary either.

The real exposure here is not the network, it's the glass. **The monitor is an
always-on panel in a room**: whatever is on it is readable by anyone who walks
past, for as long as it is up. The phone is held. So reminders carry a
`private` flag:

| | Monitor | Phone |
|---|---|---|
| Row, time, OVERDUE tag | shown | shown |
| The words | `private reminder` | shown |
| NEEDS YOU panel | "a private reminder is overdue" | n/a |

A private reminder still counts, still occupies its row, and still raises
attention when overdue — only the words are withheld. The redaction for the
NEEDS YOU panel happens **server-side** (`REDACTED_TEXT` in `internal/state`),
because that panel is monitor-only: those words never reach the browser at
all. Reminder rows are redacted client-side, since the phone renders the same
list and does need the text.

What this does **not** protect against: someone with physical access to the
panel opening devtools, or anyone already on your tailnet. It is a
shoulder-surfing defence, not an access-control system — there is no login
here by design.

Logging is deliberately thin: only non-GET requests are logged, method and
path only. No request bodies, no reminder text, no assignment titles.

## What G.A.B. serves

All four endpoints exist and are verified end to end. **G.A.B. listens on
8882** — it served 8420 for its whole history and was moved to this project's
reserved port, so `GAB_URL`'s default is right as written and needs no
override. All four are read-only, and `internal/gab` has no write path to add
to by accident.

| Path | Feeds |
|---|---|
| `GET /api/assignments` | the ASSIGNMENTS block and the NEXT DUE countdown |
| `GET /api/reminders?view=dashboard` | the REMINDERS block and attention rule 1 |
| `GET /api/tasks` | DAILY |
| `GET /api/objectives` | WEEKLY OBJECTIVES |

```json
{
  "reminders": [
    {
      "id": "r1",
      "text": "take out the trash",
      "due": "2026-08-11T20:00:00",
      "enabled": true,
      "acknowledged": false,
      "repeat": "ONCE",
      "private": false
    }
  ]
}
```

Three things about that shape are G.A.B.'s side of the contract rather than
this side's, and are worth knowing when reading the panel:

- **`?view=dashboard` is load-bearing.** The bare `/api/reminders` predates
  this project and serves G.A.B.'s own HUD, where `enabled` is the stored flag
  (it draws an OFF tag from it). Here `enabled` has to mean "still
  outstanding", because G.A.B.'s scheduler disables a dated reminder within
  twenty seconds of its deadline — long before you have done the thing. Without
  the parameter, attention rule 1 could essentially never fire.
- **`acknowledged` is a ticked-off daily task.** G.A.B. has no acknowledge
  action, but a dated reminder lands on its daily task list when its day
  arrives, and "Gab, mark the lab report as done" ticks it. Same signal drives
  `done` on an assignment.
- **`private` is set by voice or from G.A.B.'s HUD** ("private reminder: …",
  or the HIDE button on a row in EDIT mode). It is the flag described under
  Privacy above. It rides along onto a task too, since the reminder import
  copies the text — otherwise the words would come back on the monitor as an
  ordinary DAILY row.

`course` may be absent on an assignment: G.A.B. stores a name and a due date,
and does not guess a course out of the text. The UI drops the column for rows
without one rather than rendering an empty gutter.

## Still open

- **Nothing is blocking.** Both questions that used to sit here — G.A.B.'s real
  port, and whether it would serve tasks and objectives — are answered above.
- The shapes for reminders, tasks and objectives are still marked `UNPINNED` in
  `internal/model`: only `/api/assignments` was ever pinned in
  `docs/dashboard-ui.md`. They are this side's proposal that G.A.B. matched, so
  a 404 from any of the three still degrades that one block to hidden rather
  than failing the panel — which is what keeps this running against an older
  G.A.B.
