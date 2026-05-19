#!/usr/bin/env bash
# scripts/build.sh — reproducible Phalanx builds with post-build sanity checks.
#
# Default: build all four binaries for the host platform inside a pinned Go
# Docker image, drop them in ./dist/<os>_<arch>/, then validate each.
#
# Usage:
#   scripts/build.sh                       # host platform, dockerised
#   scripts/build.sh --all-platforms       # every os/arch goreleaser ships
#   scripts/build.sh --platform linux/arm64
#   scripts/build.sh --no-docker           # use host's go (CI runners)
#   scripts/build.sh --version v1.2.3      # override stamped version
#
# Env overrides:
#   GO_VERSION=1.22                        # Go image tag
#   OUT_DIR=$(pwd)/dist                    # output root
#   VERSION=$(git describe …)              # stamped into binaries

set -euo pipefail

GO_VERSION="${GO_VERSION:-1.22}"
GO_IMAGE="golang:${GO_VERSION}-bookworm"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/dist}"
VERSION="${VERSION:-}"
USE_DOCKER=1
PLATFORMS=()

BINARIES=(phalanx phalanx-npm-hook phalanx-pip-hook phalanx-yarn-hook)

ALL_PLATFORMS=(
  linux/amd64 linux/arm64
  darwin/amd64 darwin/arm64
  windows/amd64
)

# ─── arg parsing ──────────────────────────────────────────────────────────

while [[ $# -gt 0 ]]; do
  case "$1" in
    --all-platforms)  PLATFORMS=("${ALL_PLATFORMS[@]}"); shift ;;
    --platform)       PLATFORMS+=("$2"); shift 2 ;;
    --no-docker)      USE_DOCKER=0; shift ;;
    --version)        VERSION="$2"; shift 2 ;;
    -h|--help)        sed -n '2,/^set -/p' "$0" | sed 's/^# \{0,1\}//;s/^set -.*//'; exit 0 ;;
    *)                echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

if [[ ${#PLATFORMS[@]} -eq 0 ]]; then
  host_os="$(go env GOOS 2>/dev/null || uname -s | tr '[:upper:]' '[:lower:]')"
  host_arch="$(go env GOARCH 2>/dev/null || true)"
  if [[ -z "$host_arch" ]]; then
    case "$(uname -m)" in
      x86_64|amd64) host_arch=amd64 ;;
      aarch64|arm64) host_arch=arm64 ;;
      *) echo "cannot detect host arch — pass --platform explicitly" >&2; exit 2 ;;
    esac
  fi
  PLATFORMS=("$host_os/$host_arch")
fi

if [[ -z "$VERSION" ]]; then
  VERSION="$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)"
fi
COMMIT="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# ─── pretty output ────────────────────────────────────────────────────────

c_dim=$'\033[2m'; c_red=$'\033[31m'; c_grn=$'\033[32m'
c_ylw=$'\033[33m'; c_cyn=$'\033[36m'; c_rst=$'\033[0m'
log()  { printf "%s%s%s\n" "$c_cyn" "$*" "$c_rst"; }
ok()   { printf "  %s✓%s %s\n" "$c_grn" "$c_rst" "$*"; }
warn() { printf "  %s⚠%s %s\n" "$c_ylw" "$c_rst" "$*"; }
die()  { printf "  %s✗%s %s\n" "$c_red" "$c_rst" "$*" >&2; exit 1; }

# ─── build ────────────────────────────────────────────────────────────────

mkdir -p "$OUT_DIR"

build_cmd() {
  # $1 goos, $2 goarch, $3 outdir-on-container, $4 ext
  local goos="$1" goarch="$2" out="$3" ext="$4"
  local ldflags="-s -w -X main.version=${VERSION#v} -X main.commit=${COMMIT} -X main.date=${DATE}"
  local script=""
  for b in "${BINARIES[@]}"; do
    script+="GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 go build -trimpath -ldflags '$ldflags' -o $out/${b}${ext} ./cmd/${b} && "
  done
  script+="echo built"
  echo "$script"
}

run_build() {
  local goos="$1" goarch="$2"
  local ext=""
  [[ "$goos" == "windows" ]] && ext=".exe"
  local platdir="$OUT_DIR/${goos}_${goarch}"
  mkdir -p "$platdir"
  log "Building for ${goos}/${goarch} (version=$VERSION, commit=$COMMIT)"

  local cmd
  cmd="$(build_cmd "$goos" "$goarch" "/work/dist/${goos}_${goarch}" "$ext")"

  if [[ "$USE_DOCKER" == "1" ]]; then
    command -v docker >/dev/null 2>&1 || die "docker not found — re-run with --no-docker"
    docker run --rm \
      -v "$REPO_ROOT:/work" \
      -w /work \
      -e GOFLAGS=-mod=mod \
      "$GO_IMAGE" \
      bash -c "$cmd" >/dev/null
  else
    command -v go >/dev/null 2>&1 || die "go not found and --no-docker set"
    local local_cmd
    local_cmd="$(build_cmd "$goos" "$goarch" "$platdir" "$ext")"
    (cd "$REPO_ROOT" && bash -c "$local_cmd") >/dev/null
  fi
}

# ─── sanity ───────────────────────────────────────────────────────────────

file_size() {
  if stat -c '%s' "$1" >/dev/null 2>&1; then stat -c '%s' "$1"
  else stat -f '%z' "$1"; fi
}

sanity_check() {
  local goos="$1" goarch="$2"
  local ext=""
  [[ "$goos" == "windows" ]] && ext=".exe"
  local platdir="$OUT_DIR/${goos}_${goarch}"
  local host_os host_arch
  host_os="$(go env GOOS 2>/dev/null || uname -s | tr '[:upper:]' '[:lower:]')"
  host_arch="$(go env GOARCH 2>/dev/null || echo "")"

  for b in "${BINARIES[@]}"; do
    local f="$platdir/${b}${ext}"
    [[ -f "$f" ]] || die "missing: $f"
    [[ -x "$f" || "$goos" == "windows" ]] || die "not executable: $f"
    local size
    size="$(file_size "$f")"
    if (( size < 1000000 )); then
      die "$b: ${size}B is suspiciously small (<1MB)"
    fi
    if (( size > 104857600 )); then
      die "$b: ${size}B is suspiciously large (>100MB)"
    fi
    ok "$(printf '%-22s %8s bytes' "$b" "$size")"
  done

  if [[ "$goos/$goarch" == "$host_os/$host_arch" ]]; then
    local main="$platdir/phalanx${ext}"
    local got
    got="$("$main" --version 2>&1 || true)"
    [[ "$got" == *"$VERSION"* || "$got" == *"${VERSION#v}"* ]] \
      || die "--version output did not contain $VERSION: $got"
    ok "phalanx --version → $got"
    "$main" --help >/dev/null 2>&1 || die "phalanx --help failed"
    ok "phalanx --help exits 0"
    if command -v file >/dev/null 2>&1; then
      local desc
      desc="$(file "$main")"
      [[ "$desc" == *"statically linked"* || "$desc" == *"static-pie"* || "$desc" == *"Mach-O"* || "$desc" == *"PE32"* ]] \
        && ok "binary type: ${desc##*: }" \
        || warn "binary type: ${desc##*: }"
    fi
  fi
}

# ─── orchestrate ──────────────────────────────────────────────────────────

log "Phalanx build — $VERSION"
[[ "$USE_DOCKER" == "1" ]] && log "  toolchain: $GO_IMAGE" || log "  toolchain: host go ($(go version))"
log "  out: $OUT_DIR"
log "  platforms: ${PLATFORMS[*]}"
echo

for plat in "${PLATFORMS[@]}"; do
  goos="${plat%/*}"
  goarch="${plat#*/}"
  run_build "$goos" "$goarch"
  sanity_check "$goos" "$goarch"
  echo
done

log "All builds passed sanity checks."
echo "  Artefacts under: $OUT_DIR"
