# Docker stack upgrade

Optional container stack. Adds three services:

| Service | Image | Port | Purpose |
|---|---|---|---|
| Verdaccio | `verdaccio/verdaccio:6` | 4873 | Private npm registry — mirrors approved packages |
| Trivy | `aquasec/trivy:latest` | — | CVE scanner (alternative to local binary) |
| OPA | `openpolicyagent/opa:latest-static` | 8181 | Policy engine evaluating `policy.rego` |

Reference compose file: [`_reference/docker-compose.yml`](_reference/docker-compose.yml)
Reference Verdaccio config: [`_reference/verdaccio.yaml`](_reference/verdaccio.yaml)

## When to use

- **Verdaccio**: team-shared approved-package cache. After Phalanx clears a
  scan, publish that tarball into Verdaccio and point team `.npmrc` at it.
- **Trivy container**: hosts without Trivy installed, or pinned-version CI.
- **OPA**: centrally managed policy across many devs.

For solo use, the local Trivy binary is faster and the inline policy mirrors
OPA exactly.

## Bring it up

```bash
# copy reference compose to repo root
cp docs/upgrades/_reference/docker-compose.yml ./
cp docs/upgrades/_reference/verdaccio.yaml ./config/

docker compose up -d registry              # always-on registry
docker compose --profile scan up -d opa    # on-demand OPA
```

The Go scanner does not yet check OPA — `internal/policy/policy.go` always
runs inline. To wire it up: add an HTTP call to `localhost:8181/v1/data/phalanx/policy`
at the top of `policy.Evaluate(...)`, fall back to inline on any error.

Verdaccio integration is **not yet wired**.

## Verdaccio integration (not implemented)

The intended flow:

1. Hook scans `lodash@4.17.21` → approved
2. Tarball pushed to `localhost:4873`
3. Team members install via Verdaccio mirror

Required to implement:
- Tarball publish step in `internal/scanner/scanner.go` after `pr.Allow == true`
- `npmPublish` config key
- `.npmrc` writer in `internal/hooks/install.go`

## Resource cost

- Verdaccio: ~50 MB idle
- Trivy: ~0 MB idle (on-demand)
- OPA: ~10 MB idle
- Total: ~60 MB idle, ~350 MB during scan
