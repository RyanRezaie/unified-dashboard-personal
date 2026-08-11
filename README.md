# Dashboard

The unified semester/homelab dashboard. One glance answers "what's my situation
right now": assignments due, homelab health, application pipeline.

Design is settled in [`docs/dashboard-ui.md`](docs/dashboard-ui.md), with
[`docs/dashboard-ui-mockup.html`](docs/dashboard-ui-mockup.html) as the visual
reference. This file covers how to run it.

## Run it

```sh
# With fixture data — no G.A.B., no Proxmox, no GPU agent needed.
DASHBOARD_STUB=1 go run ./cmd/dashboard
# → http://localhost:8881
```

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
| `GAB_URL` | `http://127.0.0.1:8882` | G.A.B., for `GET /api/assignments` |
| `GPU_AGENT_URL` | — | Workstation agent wrapping `nvidia-smi` |
| `PROXMOX_URL` / `PROXMOX_NODE` | — | Proxmox API |
| `PROXMOX_TOKEN_ID` / `PROXMOX_TOKEN_SECRET` | — | API token; **env only** |
| `PROXMOX_INSECURE` | `true` | Self-signed homelab cert |
| `PIPELINE_PATH` | `./data/pipeline.json` | The only writable state |
| `ATTENTION_CONTAINERS` | *(empty)* | **Unconfirmed** — see below |
| `ATTENTION_GPU_TEMP_C` | `80` | Confirmed |
| `ATTENTION_DISK_PCT` | `85` | Confirmed |
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

## Still open

Carried from `docs/dashboard-ui.md`, plus one found while building:

1. **The watched-container list.** GPU temp (80 °C) and disk (85%) are
   confirmed. This is the last placeholder: the server warns at startup while
   `ATTENTION_CONTAINERS` is empty, and rule 2 can never fire until it is set.
2. **G.A.B.'s port.** 8882 is this project's reservation, not a port G.A.B. is
   known to be listening on.
3. **G.A.B. endpoints beyond assignments** — `/api/tasks` and
   `/api/objectives` are still proposed shapes; a 404 from either hides that
   block rather than failing. See the reminders contract below, which is now
   settled on this side.

## What G.A.B. needs to serve

`GET /api/assignments` is pinned in `docs/dashboard-ui.md`. Since G.A.B.
currently stores only a name and a due date, `course` and `done` may be
omitted — the UI drops the course column for rows without one rather than
rendering an empty gutter.

`GET /api/reminders` is what attention rule 1 runs on. Until it exists, that
rule can never fire:

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

`private` is the flag described under Privacy above. Anything G.A.B. would not
want read aloud by a panel on the wall should set it. Both endpoints are
read-only; `internal/gab` has no write path.
