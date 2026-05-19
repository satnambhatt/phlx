# Releases

How Phalanx gets from a commit on `main` to binaries that users install.

There are two pieces:

1. **`scripts/build.sh`** — reproducible local builds inside a pinned Go
   Docker image, with post-build sanity checks. Used for verification
   before tagging and for emergency manual binary uploads.
2. **`.goreleaser.yaml` + `.github/workflows/release.yml`** — the
   canonical pipeline. A `v*` tag triggers GoReleaser in GitHub Actions,
   which builds every os/arch combination, archives them, generates
   checksums, and publishes the GitHub Release. `phlx update` reads the
   resulting Release.

Most releases only touch piece 2 — tag and push. Piece 1 exists so we
can prove the binaries are good before we ship them.

---

## Versioning

SemVer (`vMAJOR.MINOR.PATCH`):

| Bump | When |
|---|---|
| MAJOR | Hook protocol or DB schema breaks; users must re-run `phlx hooks install` after upgrade |
| MINOR | New CLI command, new ecosystem, new analyser — backwards-compatible |
| PATCH | Bug fix or doc-only change |

The version is **never** hard-coded in source. GoReleaser injects it via
`-ldflags "-X main.version=<tag>"`; `scripts/build.sh` does the same when
building locally.

---

## Pre-release checks

Before tagging, run these from the repo root:

```bash
go vet ./...                            # no findings
go build ./...                          # compiles everywhere

scripts/build.sh                        # host platform inside Docker
scripts/build.sh --all-platforms        # full cross-compile matrix
```

`scripts/build.sh` runs the following sanity checks per produced binary:

- File exists and is executable (or `.exe` on Windows)
- Size between 1 MB and 100 MB (catches truncated / accidentally-vendored builds)
- For the host-platform `phalanx`: `--version` contains the stamped
  version, `--help` exits 0, `file(1)` reports a statically-linked
  binary (CGO is disabled across the board)

If any check fails the script exits non-zero — do not tag.

---

## Cutting a release

```bash
# 1. Confirm main is green
git checkout main && git pull

# 2. Verify locally
scripts/build.sh --all-platforms

# 3. Pick the next tag (this example: 1.2.0)
git tag -a v1.2.0 -m "v1.2.0"

# 4. Push the tag — GitHub Actions takes it from here
git push origin v1.2.0
```

The `release` workflow (`.github/workflows/release.yml`):

1. Checks out the tagged commit
2. Sets up Go 1.22
3. Runs `goreleaser release --clean`, which:
   - Builds `phalanx`, `phalanx-npm-hook`, `phalanx-pip-hook`,
     `phalanx-yarn-hook` for every os/arch declared in `.goreleaser.yaml`
   - Stamps `main.version`, `main.commit`, `main.date` via ldflags
   - Archives binaries (`tar.gz` everywhere except Windows → `zip`)
   - Generates `checksums.txt` (SHA-256)
   - Creates the GitHub Release with the header + install snippet from
     `.goreleaser.yaml`'s `release.header` block

Watch the run finish in the Actions tab. On success the release shows up
at `https://github.com/satnambhatt/phlx/releases/tag/v1.2.0` with all
archives attached.

---

## Updating binaries (in-flight users)

Existing installs upgrade themselves:

```bash
phlx update --check     # confirm latest matches the tag you just pushed
phlx update             # actually replace the binaries
```

The self-updater pulls the same archives GoReleaser uploaded and atomically
replaces `phalanx` + every binary listed in `hookBinaries`
(`internal/cli/update.go`). Existing shims under `~/.phalanx/bin/` keep
working without re-running `phlx hooks install`.

---

## Post-release verification

From a clean machine (or container):

```bash
# Install the prior tag
VERSION=1.1.0 ./README-install-snippet.sh

# Upgrade and confirm
phlx --version          # → phalanx version 1.1.0
phlx update --check     # → update available: 1.1.0 → 1.2.0
phlx update             # writes new binaries
phlx --version          # → phalanx version 1.2.0

# Functional smoke test
phlx scan crossenv      # known typosquat — must still BLOCK
```

If `phlx update` fails to find the archive, the most common cause is a
mismatch between `archiveName()` in `internal/cli/update.go` and the
`name_template` in `.goreleaser.yaml`. They must produce identical
filenames — change one without the other and self-update breaks.

---

## Manual binary upload (emergency)

Only when GoReleaser is broken and you need to ship anyway:

```bash
scripts/build.sh --all-platforms --version v1.2.1

# Each platform's binaries land under dist/<os>_<arch>/
# Archive them to match the GoReleaser naming convention:
cd dist/linux_amd64
tar -czf ../phlx_1.2.1_linux_x86_64.tar.gz phalanx phalanx-npm-hook phalanx-pip-hook phalanx-yarn-hook
# repeat per platform; macOS uses "macos" instead of "darwin", amd64 → x86_64

# Create the release on GitHub manually and attach the archives + a
# checksums.txt produced via:  sha256sum *.tar.gz *.zip > checksums.txt
```

The naming must exactly match what `archiveName()` expects or
`phlx update` will 404. Reference:

```
phlx_<version>_<os>_<arch>.<ext>

os    : macos | linux | windows
arch  : x86_64 (amd64) | arm64
ext   : tar.gz everywhere except windows → zip
```

After a manual release, follow the **Post-release verification** steps
above before you announce.

---

## Hotfix process

Bug found on the latest tag, `main` has unrelated work in flight:

```bash
git checkout -b hotfix/v1.2.1 v1.2.0
# fix the bug, commit
scripts/build.sh                  # verify locally
git tag -a v1.2.1 -m "v1.2.1"
git push origin v1.2.1            # release workflow fires
git checkout main
git merge hotfix/v1.2.1           # carry the fix forward
git push origin main
```

Do not delete the hotfix branch until the next tag from `main` has
shipped — it's the canonical record of the fix.

---

## What lives where

| Concern | File |
|---|---|
| Build matrix, archive names, release notes header | `.goreleaser.yaml` |
| CI that runs GoReleaser | `.github/workflows/release.yml` |
| Local reproducible build + sanity | `scripts/build.sh` |
| Self-updater (filename convention) | `internal/cli/update.go` |
| List of every binary the release ships | `cmd/phalanx-*-hook/`, `internal/cli/update.go` `hookBinaries`, `.goreleaser.yaml` `archives.ids` |
