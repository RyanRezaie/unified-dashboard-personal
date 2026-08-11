# CLAUDE.md — Dashboard (parent repo)

## What this repo is
The unified semester/homelab dashboard, with related projects included as git
submodules. This file is for orientation across the whole tree — each submodule has its
own `CLAUDE.md` with real detail; don't duplicate that here, link to it.

## Layout
```
dashboard/              ← this repo; the actual dashboard app (see its own build notes above)
├── gab/                ← submodule: G.A.B., the voice assistant (existing project,
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
