# Phalanx (phlx) — Local Cyber Shield for Open Source Installs

[![GitHub stars](https://img.shields.io/github/stars/satnambhatt/phlx?style=social)](https://github.com/satnambhatt/phlx)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](https://opensource.org/licenses/MIT)

> *The ancient Greek formation where shields interlock. Nothing gets through.*
> *To Note: The code is written using the help of Claude Code.* 

Phalanx intercepts `npm install`, `yarn add`, and `pip install` at the shell
level, scans every package for vulnerabilities before it touches your machine,
and blocks anything dangerous. Developers change nothing about how they work.

```
$ npm install express
  phalanx  scanning express...
  ✓  express@4.18.3 — clean

$ npm install crossenv
  phalanx  scanning crossenv...
  🚫 BLOCKED: crossenv@0.0.2-security
     • [TYPOSQUAT] T001: 'crossenv' is a documented typosquat of 'cross-env'

$ pip install requests
  phalanx  scanning requests...
  ✓  requests==2.31.0 — clean
```

No accounts. No cloud. No config. Runs entirely on your machine.
Written in Go — single static binary, pure-Go SQLite, no runtime needed.

---

## Install

### Prerequisites
| Tool | Why |
|---|---|
| Trivy | Stage 3 CVE scan (optional — fail-open without it) |
| Docker | Trivy fallback when binary not installed (optional) |
| Go 1.22+ | only if building from source |

```bash
brew install trivy
```

---

### Option 1 — Prebuilt binary (recommended)

Pick the latest tag from
[github.com/satnambhatt/phlx/releases](https://github.com/satnambhatt/phlx/releases)
and export it once:

```bash
VERSION=$(curl -s https://api.github.com/repos/satnambhatt/phlx/releases/latest \
  | grep tag_name | cut -d'"' -f4 | sed 's/^v//')
echo "$VERSION"
```

**macOS — Apple Silicon (arm64):**
```bash
curl -sSL "https://github.com/satnambhatt/phlx/releases/download/v${VERSION}/phlx_${VERSION}_macos_arm64.tar.gz" | tar -xz
sudo mv phalanx phalanx-npm-hook phalanx-pip-hook phalanx-yarn-hook /usr/local/bin/
```

**macOS — Intel (x86_64):**
```bash
curl -sSL "https://github.com/satnambhatt/phlx/releases/download/v${VERSION}/phlx_${VERSION}_macos_x86_64.tar.gz" | tar -xz
sudo mv phalanx phalanx-npm-hook phalanx-pip-hook phalanx-yarn-hook /usr/local/bin/
```

**Linux — x86_64:**
```bash
curl -sSL "https://github.com/satnambhatt/phlx/releases/download/v${VERSION}/phlx_${VERSION}_linux_x86_64.tar.gz" | tar -xz
sudo mv phalanx phalanx-npm-hook phalanx-pip-hook phalanx-yarn-hook /usr/local/bin/
```

**Linux — arm64:**
```bash
curl -sSL "https://github.com/satnambhatt/phlx/releases/download/v${VERSION}/phlx_${VERSION}_linux_arm64.tar.gz" | tar -xz
sudo mv phalanx phalanx-npm-hook phalanx-pip-hook phalanx-yarn-hook /usr/local/bin/
```

**Windows — x86_64 (PowerShell):**
```powershell
$VERSION = (Invoke-RestMethod https://api.github.com/repos/satnambhatt/phlx/releases/latest).tag_name -replace '^v',''
Invoke-WebRequest -Uri "https://github.com/satnambhatt/phlx/releases/download/v$VERSION/phlx_${VERSION}_windows_x86_64.zip" -OutFile phlx.zip
Expand-Archive phlx.zip -DestinationPath "$Env:USERPROFILE\bin"
```

**Verify checksum (optional):**
```bash
curl -sSL "https://github.com/satnambhatt/phlx/releases/download/v${VERSION}/checksums.txt" \
  | shasum -a 256 -c --ignore-missing
```

All three binaries must live in the **same directory** — the shim installer
looks for the hooks next to the main `phalanx` binary.

---

### Option 2 — Build from source

```bash
git clone https://github.com/satnambhatt/phlx
cd phlx
go mod tidy
mkdir -p bin
go build -o bin/phalanx           ./cmd/phalanx
go build -o bin/phalanx-npm-hook  ./cmd/phalanx-npm-hook
go build -o bin/phalanx-pip-hook  ./cmd/phalanx-pip-hook
go build -o bin/phalanx-yarn-hook ./cmd/phalanx-yarn-hook
ln -sf phalanx bin/phlx
sudo cp bin/phalanx bin/phalanx-npm-hook bin/phalanx-pip-hook bin/phalanx-yarn-hook /usr/local/bin/
sudo ln -sf /usr/local/bin/phalanx /usr/local/bin/phlx
```

Or via `go install`:
```bash
go install github.com/satnambhatt/phlx/cmd/phalanx@latest
go install github.com/satnambhatt/phlx/cmd/phalanx-npm-hook@latest
go install github.com/satnambhatt/phlx/cmd/phalanx-pip-hook@latest
go install github.com/satnambhatt/phlx/cmd/phalanx-yarn-hook@latest
```

---

### Activate hooks
```bash
phalanx hooks install
source ~/.zshrc        # or ~/.zprofile / ~/.bashrc
```

Verify:
```bash
which npm     # → ~/.phalanx/bin/npm
which yarn    # → ~/.phalanx/bin/yarn
which pip3    # → ~/.phalanx/bin/pip3
```

Every `npm install`, `yarn add`, and `pip install` is now protected.

---

## How it works

Phalanx installs shim scripts at `~/.phalanx/bin/{npm,yarn,pip,pip3}`,
prepended to `$PATH`. Each shim calls a small Go binary that scans, then
either blocks or execs the real package manager.

```
You type: npm install lodash
                │
     ~/.phalanx/bin/npm   ← shim intercepts
                │
   phalanx-npm-hook  ← Go binary
                │
     resolves lodash → 4.17.21
                │
     downloads tarball → Trivy + behavioural in parallel
                │
        ┌───────┴────────┐
      BLOCKED          CLEAN
        │                │
    exit 1           real npm runs
  nothing installs    install proceeds
```

**Fail-open by design.** If Trivy is unavailable or anything goes wrong,
Phalanx warns and steps aside. Never silently breaks your workflow.

---

## CLI reference

Both `phalanx` and `phlx` work — they are the same binary.

| Command | Purpose |
|---|---|
| `phlx scan <pkg>[@<v>]` | Manually scan a package |
| `phlx status` | Scan stats from local SQLite |
| `phlx history -n 20` | Recent install events |
| `phlx watch [path]` | Register a project (see [docs/upgrades](docs/upgrades/README.md)) |
| `phlx allow <pkg>@<v> -r "<reason>"` | Bypass the gate |
| `phlx hooks install [hook...]` / `phlx hooks remove [hook...]` | Manage shell shims (`npm`, `yarn`, `pip`, `pip3`); no args = all |
| `phlx config [key] [value]` | Read or write configuration |
| `phlx update` | Pull and install the latest released version |
| `phlx --version` | Print the build-stamped version |

Full reference with every flag and exit code: **[docs/cli.md](docs/cli.md)**.

```bash
phlx scan lodash                       # manual scan
phlx scan express@4.18.3
phlx scan requests==2.31.0 -e pip
phlx update --check                    # see if a newer release is out
phlx update                            # download + replace binaries
```

All state lives in `~/.phalanx/phalanx.db`. No background process required.

---

## The 5-stage scan pipeline

| Stage | What | Network |
|---|---|---|
| 1. Typosquat | Known squats + Levenshtein + homoglyphs + separator confusion | none |
| 2. Freshness | Compare metadata vs previous version | 1 call |
| 3. Trivy CVE | Vulnerabilities + licence | depends on Trivy |
| 4. Behavioural | 14 static-analysis rules on extracted tarball | none |
| 5. Policy gate | Unified decision (inline mirror of `policy.rego`) | none |

Stages 3 + 4 run in parallel (goroutines). Stage 1 can cause early exit on
CRITICAL typosquat — skips download entirely.

---

## Security policy

Default rules:

| Condition | Action |
|---|---|
| CRITICAL CVE | Block |
| HIGH CVE, no fix | Block |
| HIGH CVE, fix exists | Warn + allow |
| MEDIUM CVE | Warn + allow |
| CRITICAL / HIGH behavioural | Block |
| Known typosquat (T001) | Block |
| GPL / AGPL licence | Block |
| LGPL / MPL licence | Warn |

Externalise the policy via OPA — see [docs/upgrades/opa-policy.md](docs/upgrades/opa-policy.md).

---

## Footprint

| State | RAM |
|---|---|
| Idle (no process) | 0 MB |
| Active scan | ~150 MB peak |
| Binary on disk | ~15 MB (each) |

Scan results cached 24h. Install history persisted. Single SQLite file.

Optional background daemon + Docker stack documented in
[docs/upgrades/](docs/upgrades/README.md).

---

## Cross-compile

```bash
GOOS=linux   GOARCH=amd64 go build -o bin/phalanx-linux-amd64    ./cmd/phalanx
GOOS=linux   GOARCH=arm64 go build -o bin/phalanx-linux-arm64    ./cmd/phalanx
GOOS=darwin  GOARCH=arm64 go build -o bin/phalanx-darwin-arm64   ./cmd/phalanx
GOOS=windows GOARCH=amd64 go build -o bin/phalanx-windows.exe    ./cmd/phalanx
```

`modernc.org/sqlite` is pure Go — no CGO, no platform toolchain.

---

## Smaller binary

```bash
go build -ldflags="-s -w" -trimpath -o bin/phalanx ./cmd/phalanx
upx --best bin/phalanx       # optional, ~5 MB result
```

---

## Remove

```bash
phalanx hooks remove
sudo rm /usr/local/bin/{phalanx,phalanx-npm-hook,phalanx-pip-hook,phalanx-yarn-hook,phlx}
rm -rf ~/.phalanx
```

---

## Project layout

```
phlx/
├── cmd/
│   ├── phalanx/              # main CLI (cobra root)
│   ├── phalanx-npm-hook/     # npm shim binary
│   ├── phalanx-pip-hook/     # pip / pip3 shim binary
│   └── phalanx-yarn-hook/    # yarn shim binary
├── internal/
│   ├── db/                   # SQLite (modernc.org/sqlite, pure Go)
│   ├── scanner/              # 5-stage pipeline orchestrator
│   ├── policy/               # inline policy mirror
│   ├── trivy/                # Trivy invocation (binary or docker)
│   ├── analysers/            # typosquat / freshness / behavioural
│   ├── registry/             # npm + PyPI metadata + tarball download
│   ├── hooks/                # shim installer + PATH writer
│   └── cli/                  # cobra subcommand wiring
└── docs/
    └── upgrades/             # optional daemon, Docker stack, OPA
```

---

## Dependencies

| Module | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI framework |
| `modernc.org/sqlite` | Pure-Go SQLite |
| `github.com/fatih/color` | Terminal colour |
| `github.com/briandowns/spinner` | Spinner |
| stdlib | `archive/tar`, `archive/zip`, `compress/gzip`, `net/http`, `regexp` |

---

## Why Phalanx?

The phalanx was the most effective defensive formation in the ancient world —
shields locked together, no gaps. No individual shield was impenetrable, but
the formation was. Every dependency goes through the formation before it
reaches your code.

*Built with [Trivy](https://trivy.dev). Optional integrations:
[OPA](https://www.openpolicyagent.org), [Verdaccio](https://verdaccio.org)
— see [docs/upgrades](docs/upgrades/README.md).*

---

## Star History

[![Star History Chart](https://api.star-history.com/chart?repos=satnambhatt/phlx&type=date&legend=top-left)](https://www.star-history.com/?repos=satnambhatt%2Fphlx&type=date&legend=top-left)