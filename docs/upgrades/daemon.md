# Daemon upgrade

A long-running background process that hosts an HTTP API on `localhost:7777`
and runs a file watcher. Currently **not implemented in the Go core** —
Phalanx hooks scan inline and exit. This is the design for adding it back.

Reference scaffolds: [`_reference/daemon.go`](_reference/daemon.go) and
[`_reference/watcher.go`](_reference/watcher.go). Both carry `//go:build ignore`
so they don't compile by default — copy into `cmd/phalanx-daemon/` and
`internal/watcher/` respectively to wire them up.

## Why add it?

- **Live watch**: scan packages the moment `package.json` changes, before
  anyone runs `npm install`.
- **HTTP API**: integrate with CI dashboards or IDE extensions.
- **Faster repeat scans**: skip Go process startup cost when many scans hit
  per minute (currently ~5 ms per invocation — usually not worth it).

The default hooks already cache results 24 h, so for solo dev use the daemon
adds little.

## Endpoints (proposed)

```
POST   /scan      { pkg, version, ecosystem }   → scan result
GET    /status                                   → health + stats
GET    /history                                  → recent installs
GET    /blocked                                  → all-time blocked
POST   /watch     { path }                       → register project
DELETE /watch     { path }                       → unregister
POST   /allow     { pkg, version, ecosystem }    → bypass exception
GET    /config                                   → key-value pairs
```

## Go port outline

1. Create `cmd/phalanx-daemon/main.go` — `net/http` server, listens on
   `127.0.0.1:7777`, env override `PHALANX_PORT`.
2. Add `internal/watcher/` using `github.com/fsnotify/fsnotify` — watch
   `package.json`, `requirements.txt`, `Pipfile`, `pyproject.toml`.
3. In hooks (`cmd/phalanx-{npm,pip}-hook/main.go`), try `POST localhost:7777/scan`
   first with a short timeout. On `ECONNREFUSED` fall back to inline
   `scanner.Scan(...)` — same code path already in use today.
4. Add CLI commands `phalanx start` / `phalanx stop` to spawn/kill the
   daemon. Spawn via `exec.Command` with `os.ProcAttr.Setsid` on POSIX.
5. Store PID in `~/.phalanx/daemon.pid` for the stop command.

## Auto-start

- **macOS**: write `~/Library/LaunchAgents/dev.phalanx.daemon.plist`
- **Linux**: write `~/.config/systemd/user/phalanx.service`

Neither is implemented yet.

## Resource cost

- Idle: ~30 MB RSS (Go runtime + watcher)
- Active scan: ~150 MB peak
- Network: localhost only
