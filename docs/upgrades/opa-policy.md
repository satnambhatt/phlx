# OPA policy upgrade

Phalanx's scanner has an inline policy (`Evaluate(...)` in
`internal/policy/policy.go`) that mirrors `_reference/policy.rego` in spirit.
Both produce `{ allow, deny[], warn[] }`.

The inline copy is the default — and currently the only path. Switch to real
OPA when you want:

- Hot-reloadable policy without restarting hooks
- A single Rego file shared across many machines
- An audit trail of policy evaluations

## Enable OPA (Go port — not yet implemented)

1. Run OPA:
   ```bash
   docker run -p 8181:8181 -v $(pwd)/policies:/policies \
     openpolicyagent/opa:latest-static run --server --addr=0.0.0.0:8181 /policies
   ```
2. Modify `internal/policy/policy.go` — add an HTTP call to
   `POST http://localhost:8181/v1/data/phalanx/policy` at the top of
   `Evaluate(...)`. Fall back to inline on any error.
3. Edit your local `policy.rego`; OPA reloads automatically.

## Keep the two in sync

`Evaluate(...)` is the source of truth when OPA is down. If you diverge:

- Add the rule to `policy.rego` first
- Mirror in `Evaluate(...)` in `internal/policy/policy.go`
- Add a test fixture that asserts both produce identical `{ deny, warn }`

## Finding prefixes (contract)

OPA Rego and inline must emit strings prefixed exactly:

| Prefix | Source |
|---|---|
| `[CVE]` | Trivy vulnerability |
| `[BEHAVIOUR]` | Behavioural analyser |
| `[FRESHNESS]` | Freshness analyser |
| `[TYPOSQUAT]` | Typosquat analyser |
| `[LICENCE]` | Trivy licence scan |

The CLI colours by prefix, so changes break terminal output.
