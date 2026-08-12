# Blueprint — letting G.A.B. drive the dashboard

**Status: not built. This is a sizing document**, written so the decision can
be made before any of it exists. Nothing here is committed to.

The question is "how much work to let Gab interact with the new UI." It is
really three questions with very different answers, so they are separated
below and each is costed on its own.

---

## The one fact that shapes all of it

Today the dependency runs in exactly one direction:

```
dashboard  ──GET──▶  G.A.B.        (internal/gab, GET-only by construction)
dashboard  ──GET──▶  GPU agent
dashboard  ──GET──▶  Proxmox
```

G.A.B. does not know the dashboard exists. That is why the dashboard can be
restarted, redeployed or switched off without G.A.B. noticing, and why
`internal/gab` treats a 404 as "not implemented yet" rather than an error.

Every option below adds an arrow pointing the other way. That is the real cost
— not the line count.

**What the dashboard actually owns is one thing: the pipeline.** Assignments,
tasks, objectives and reminders are all mirrors of G.A.B.'s own store, and
`CLAUDE.md`'s non-goals are explicit that editing those stays G.A.B.'s job.
So "Gab interacting with the UI" mostly means "Gab writing to the pipeline",
which is the one piece of state G.A.B. currently has no idea about.

---

## Phase 1 — Voice writes to the pipeline

**"Gab, I applied to Stripe." → `POST /api/pipeline`**

This is the phase with real value, and it is the cheapest of the three.

### Dashboard side: no code at all

`POST`, `PATCH` and `DELETE /api/pipeline` already exist (`internal/api/api.go:57-59`)
and are already the only writes in the whole dashboard. `GET /api/pipeline`
already lists items so a spoken company name can be matched to an id.
`pipeline.Store` already does atomic writes with a `.bak`. Nothing needs
adding, and nothing needs relaxing — there is no auth to work around, because
by design there is none (Tailscale ACLs are the boundary).

### G.A.B. side: ~90 lines in `gab_server.py`

The intent classifier is the extension point and it is already shaped for
this. Five edits, all additive:

| Where | What | Size |
|---|---|---|
| Config constants | `DASHBOARD_URL`, defaulting to `http://localhost:8881` | ~4 lines |
| `TASK_INTENT_SYSTEM_PROMPT` | 3 new intents: `add_application`, `advance_application`, `drop_application` | ~5 lines |
| `TASK_INTENT_CUE_PHRASES` | `"applied to"`, `"heard back from"`, `"interview with"`, `"track an application"` | ~5 lines |
| New helper | `dashboard_pipeline_write()` — stdlib `urllib`, same shape as `preload_ollama_model()` | ~30 lines |
| Intent dispatch | the three new branches, plus spoken confirmation | ~45 lines |

Plus two lines in `docker-entrypoint.py` to make `DASHBOARD_URL` an env var
(`GAB_DASHBOARD_URL`), matching how the other three service URLs are already
overridden, and one line in `docker-compose.yml`.

No new pip dependency — `urllib` is what every other outbound call in that
file already uses.

### The two things that need care

**The classifier fail-safe is a hard constraint.** `gab-assistant/CLAUDE.md`
requires that any JSON parse failure returns `{"intent": "none"}` and falls
through to a normal answer, never a silent mutation. Three new intents mean
three new ways to get that wrong. The prefilter (`looks_like_task_command`)
must also only ever be able to produce `"none"`.

**Matching a spoken company to an existing item is fuzzy, and the failure is
destructive.** "Gab, Stripe got back to me" has to resolve to one row.
`gab_server.py` already has phrase-matching helpers used for `toggle_task`
and `remove_task`, so the machinery exists — but a mismatch on
`drop_application` retires the wrong company. Recommendation: **speak the
match back before acting** on anything destructive ("Moving Stripe to dead —
say yes"), and let `add_application` proceed without confirmation since a
wrong add is trivially undone.

### Failure behaviour

The dashboard being down must not break the voice assistant. The write is one
call at command time, not a poll, so a `try/except` around it that speaks
"I couldn't reach the dashboard" is the whole story — G.A.B. keeps working,
same as it does when Ollama is unreachable.

The panel updates itself within 20s via the existing poll. No push needed.

**Estimate: half a day**, including driving it end to end.

---

## Phase 2 — Voice questions about dashboard state

**"Gab, how's the homelab?" → `GET /api/state`**

Cheap, but worth being honest about how much it actually buys. G.A.B. already
owns assignments, tasks, objectives and reminders, so it can already answer
questions about all of those without the dashboard. The genuinely new
information is **homelab health** (GPU temp, VRAM, containers down, disk) and
**pipeline contents** — the two things the dashboard aggregates and G.A.B.
does not have.

- `dashboard_state()` GET helper — ~15 lines
- A spoken summary path, or feeding the relevant slice of `/api/state` into
  the LLM context for an open-ended answer — ~40 lines
- Cue phrases and one or two intents — ~10 lines

The GPU lane is expected to be offline whenever the main PC is asleep, so the
spoken answer needs to distinguish "GPU is fine" from "I can't see the GPU" —
`gpu_status.ok` already carries exactly that, so it is a wording problem, not
a data problem.

**Estimate: half a day.** Do it after Phase 1, reusing the same HTTP helper.

---

## Phase 3 — Live control of the panel

**"Gab, show me the jobs tab."**

This is the expensive one and the one I would not build.

The dashboard polls every 20s and has no push channel. Voice-driven
*navigation* is the only thing that genuinely needs one — data changes from
Phase 1 already appear within one poll.

What it would take:

- `GET /api/events` SSE handler in `internal/api` — client registry, fan-out,
  heartbeat, cleanup on disconnect. ~80 lines of Go, and it is the first piece
  of long-lived per-client state in a codebase that currently has none.
- An `EventSource` subscriber in `index.html` alongside the existing poll,
  with reconnect. ~20 lines.
- A way for G.A.B. to publish to it — another endpoint, and now the dashboard
  accepts commands rather than just data.

**And it argues with the design.** `docs/dashboard-ui.md` specifies the
monitor as "an always-on monitor, never touched, no navigation, everything
visible at once". A voice command that changes what is on screen makes the
panel modal — someone walking past no longer knows they are seeing the whole
picture. The phone surface has tabs precisely because it cannot show
everything; the monitor does not have that problem.

**Estimate: a full day**, for a capability that works against the panel's
premise. Recommend deferring until there is a concrete want for it.

---

## Interaction with the open monitor-controls question

`CLAUDE.md`'s NEXT SESSION note leaves a decision open: the monitor surface
has no pipeline controls, and the three candidate answers were (1) leave it
read-only, (2) give it the same controls as the phone, (3) enable them only
when the pointer is fine.

**Phase 1 is a fourth answer.** "Gab, I applied to Stripe" adds a pipeline
item without putting a single click target on an always-on display — which
was the entire objection to option 2. It does not replace option 3 (a laptop
at >1100px still wants a mouse), but it removes the pressure to solve the
problem by adding buttons to a panel whose premise is that nobody touches it.

Worth deciding these two together rather than separately.

---

## Recommendation

Build **Phase 1**. It is half a day, needs no dashboard code, delivers the one
thing the voice assistant genuinely cannot do today, and doubles as an answer
to the monitor-controls question.

Take **Phase 2** if Phase 1 lands well — it shares the HTTP helper and is
mostly wording.

Leave **Phase 3** alone until something concrete demands it.

The one thing to decide before starting: Phase 1 makes G.A.B. depend on the
dashboard, where today nothing does. That is acceptable because the dependency
is a single best-effort call at command time rather than a startup
requirement — but it should be a deliberate choice, and it means
`gab-assistant/CLAUDE.md` gains a line saying the dashboard is now an
optional downstream.
