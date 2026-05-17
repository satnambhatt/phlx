# Future upgrades

Phalanx core (Go) runs hooks-only: every `npm install` and `pip install`
passes through a shim that calls a small Go binary which scans inline and
writes history to `~/.phalanx/phalanx.db`. Two binaries:
`phalanx-npm-hook` and `phalanx-pip-hook`.

The features below were prototyped in the original Node version but moved
out of the default install path. Reference implementations live in
`_reference/`. Porting them to Go is left as future work.

| Upgrade | Adds | When to use |
|---|---|---|
| [Daemon](daemon.md) | Long-running background process, live file watcher, HTTP API on `localhost:7777` | Teams that want continuous scanning of `package.json` / `requirements.txt` without re-running installs |
| [Docker stack](docker.md) | Verdaccio (private npm registry), Trivy container, OPA server | Air-gapped or team-shared approved-package mirror |
| [OPA policy](opa-policy.md) | Externalised security policy in Rego, hot-reloadable | Centralised policy management across many machines |

Each upgrade is additive. None are required for normal use.

Reference scaffolds in `_reference/` are Go files marked `//go:build ignore`
so they don't compile by default. Copy them into:

- `_reference/daemon.go` → `cmd/phalanx-daemon/main.go`
- `_reference/watcher.go` → `internal/watcher/watcher.go`

and add `github.com/fsnotify/fsnotify` to `go.mod` to start wiring the upgrade.
