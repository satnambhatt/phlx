# AGENTS.md

Guidance for AI coding assistants (Claude Code, Cursor, Aider, Codex, Continue,
GitHub Copilot Workspace, etc.) working on the Phalanx (`phlx`) codebase.

This file follows the [agents.md](https://agents.md) convention. Treat it as
the authoritative briefing for the repo. If you only read one file before
making changes, read this one.

---

## What this project is

Phalanx is a local CLI that intercepts `npm install`, `yarn add`, and
`pip install` at the shell level, scans every requested package through a
five-stage security pipeline, and either blocks or passes the install
through to the real package manager. Single static Go binary. No daemon by
default. State lives in `~/.phalanx/phalanx.db` (pure-Go SQLite).

The product invariant is: **never silently break the developer's workflow.**
Anything Phalanx cannot evaluate must fail open with a warning, not a hard
stop.

---

## Repository layout

```
phlx/
├── cmd/
│   ├── phalanx/              # main CLI (cobra root)
│   ├── phalanx-npm-hook/     # invoked by the npm shim
│   ├── phalanx-pip-hook/     # invoked by the pip/pip3 shim
│   └── phalanx-yarn-hook/    # invoked by the yarn shim
├── internal/
│   ├── cli/                  # cobra subcommand wiring + self-updater
│   ├── db/                   # SQLite (modernc.org/sqlite, pure Go)
│   ├── scanner/              # 5-stage pipeline orchestrator
│   ├── policy/               # inline policy mirror of policy.rego
│   ├── trivy/                # Trivy invocation (binary or docker)
│   ├── analysers/            # typosquat / freshness / behavioural
│   ├── registry/             # npm + PyPI metadata + tarball download
│   └── hooks/                # shim installer + PATH writer
├── docs/                     # CLI reference, AI tool guidance, upgrades
├── .goreleaser.yaml          # release pipeline — three+ binaries per arch
└── go.mod                    # Go 1.22, pure-Go deps only
```

---

## Build / run / verify

```bash
# Build all three binaries
go build -o bin/phalanx          ./cmd/phalanx
go build -o bin/phalanx-npm-hook ./cmd/phalanx-npm-hook
go build -o bin/phalanx-pip-hook ./cmd/phalanx-pip-hook
go build -o bin/phalanx-yarn-hook ./cmd/phalanx-yarn-hook

# Sanity checks an agent should run before declaring success
go vet ./...
go build ./...
```

There is no unit test suite yet. When adding new packages, add a `_test.go`
file alongside the code; do not introduce a separate `tests/` tree.

---

## Conventions

- **CGO is forbidden.** `CGO_ENABLED=0` is set everywhere. Pick pure-Go
  alternatives (e.g. `modernc.org/sqlite`, not `mattn/go-sqlite3`).
- **Standard library first.** Pull in a dependency only when the stdlib
  equivalent is materially worse. Existing deps:
  `spf13/cobra`, `fatih/color`, `briandowns/spinner`, `modernc.org/sqlite`.
- **Fail open.** Any scan / network / disk error in the hooks must log a
  faint warning and pass through to the real package manager. The user
  should never lose work because Phalanx had a bad day.
- **Three binaries stay in lockstep.** If you add a feature that touches
  the database schema, scanner, or hook protocol, update the npm, yarn, and
  pip hooks together. `.goreleaser.yaml` must build and ship every binary.
- **Output style.** Indent with two spaces, use the existing `color`
  helpers, no emojis in code or commits. The CLI banner and result format
  must stay consistent across `scan`, the hooks, and any new command.
- **Version string.** Goreleaser injects `main.version` via ldflags. Don't
  hardcode versions in `internal/cli/`.

---

## When making changes

1. **Read the existing command first.** New subcommands belong in
   `internal/cli/`. Mirror `scanCmd()` / `updateCmd()` patterns rather
   than inventing a new style.
2. **State changes go through `internal/db`.** Do not write directly to
   `~/.phalanx/phalanx.db` from another package.
3. **Hook parsing is conservative.** When extending `parseArgs` in any hook,
   prefer to skip an argument than to misidentify it as a package — a
   misidentified arg can block a legitimate install.
4. **Self-update is risky.** `internal/cli/update.go` replaces live binaries
   on disk. Changes there must keep the rename-then-fallback-to-copy path
   intact and must continue to update every binary in `hookBinaries`.
5. **Don't introduce a daemon.** The "hooks-only" architecture is a
   deliberate product choice. Daemon work belongs in `docs/upgrades/`.

---

## What not to do

- Do not call out to the network from the main CLI except in `scanner/`,
  `registry/`, and `internal/cli/update.go`.
- Do not add telemetry, analytics, or any "phone home" behaviour. Phalanx is
  offline-first by design.
- Do not add ecosystem support (cargo, gem, etc.) without first writing a
  short design note in `docs/upgrades/`.
- Do not rewrite the scanner pipeline order. Stage 1 (typosquat) must run
  before any download; stages 3 + 4 must remain parallel.
- Do not commit binaries, archives, or anything under `dist/` or `bin/`.

---

## Useful entry points

| Question | Start at |
|---|---|
| How is a package scanned? | `internal/scanner/` orchestrator |
| How is the install gated? | `internal/policy/` |
| How do shims call the hooks? | `internal/hooks/install.go` |
| How does `phlx update` work? | `internal/cli/update.go` |
| What does the CLI surface? | `internal/cli/commands.go` |
| Where is history stored? | `internal/db/db.go` |

See also `SKILLS.md` for task-shaped recipes ("how do I add a new
analyser?", "how do I add an ecosystem?").
