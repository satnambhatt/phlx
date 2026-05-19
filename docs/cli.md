# CLI reference

Both `phalanx` and `phlx` invoke the same binary. Every command listed
here works under either name.

```
phlx <command> [flags]
```

| Command | One-liner |
|---|---|
| [`scan`](#scan) | Scan a package without installing it |
| [`status`](#status) | Show scan stats from local SQLite |
| [`history`](#history) | List recent install events |
| [`watch`](#watch) | Register a project path for tracking |
| [`allow`](#allow) | Bypass the security gate for a specific package |
| [`hooks`](#hooks) | Install or remove the npm / yarn / pip shims |
| [`config`](#config) | Read or write configuration values |
| [`update`](#update) | Pull and install the latest released version |
| `version`, `--version` | Print the version goreleaser stamped at build time |
| `help` | `phlx help <command>` shows usage for any subcommand |

State lives in `~/.phalanx/phalanx.db`. No background process is required.

---

## `scan`

Run the full 5-stage pipeline against a package without installing it.
Records the result in history exactly as if the install had gone through
a shim.

```bash
phlx scan lodash                    # scans lodash@latest from npm
phlx scan lodash@4.17.21
phlx scan requests==2.31.0 -e pip
phlx scan @types/node@20.10.0
```

| Flag | Default | Description |
|---|---|---|
| `-e`, `--ecosystem` | `npm` | `npm` or `pip` |

Exit code is `0` for clean or warned, `1` for blocked.

---

## `status`

```bash
phlx status
```

Prints the lifetime scan counters from the local database — total scans,
allowed, warned, blocked, and the number of registered projects. Useful
for confirming the database is healthy.

---

## `history`

```bash
phlx history             # last 20 events
phlx history -n 100
```

| Flag | Default | Description |
|---|---|---|
| `-n`, `--limit` | `20` | Number of entries to display |

Columns: package, version, ecosystem, action (`allowed` / `warned` /
`blocked` / `bypassed`), timestamp.

---

## `watch`

```bash
phlx watch                # current directory
phlx watch ~/code/my-app
```

Registers a project path in the database. Live file watching needs the
optional daemon upgrade — see [`upgrades/daemon.md`](upgrades/daemon.md).
Without the daemon, `watch` is a registration-only no-op so the path
appears in `phlx status`.

---

## `allow`

```bash
phlx allow crossenv@0.0.2-security -r "internal mirror, vetted"
phlx allow requests==2.31.0 -e pip -r "approved by sec team"
```

| Flag | Default | Description |
|---|---|---|
| `-e`, `--ecosystem` | `npm` | `npm` or `pip` |
| `-r`, `--reason` | `manual bypass` | Stored alongside the allow entry |

Adds an entry to the allow list keyed `allow:<eco>:<pkg>:<version>`. Use
`*` as the version to allow every version (after running `phlx allow
<pkg>@\*` once). The hooks read this list and record the action as
`bypassed` in history.

---

## `hooks`

```bash
phlx hooks install
phlx hooks remove
```

`install` writes shim scripts into `~/.phalanx/bin/` for every supported
package manager that is on `$PATH`, and prepends that directory to
`$PATH` in the first shell rc it finds (`.zshrc`, `.zprofile`,
`.bashrc`, `.bash_profile`, `.profile`). Shims installed:

| Shim | Real binary | Hook binary |
|---|---|---|
| `~/.phalanx/bin/npm` | `npm` | `phalanx-npm-hook` |
| `~/.phalanx/bin/yarn` | `yarn` | `phalanx-yarn-hook` |
| `~/.phalanx/bin/pip` | `pip` | `phalanx-pip-hook` |
| `~/.phalanx/bin/pip3` | `pip3` | `phalanx-pip-hook` |

Package managers not present on `$PATH` at install time are skipped with
a `⚠` and a "skipping" note. Re-run `phlx hooks install` after
installing a new package manager.

`remove` deletes the shim files and strips the PATH line from every
shell rc that has it.

---

## `config`

```bash
phlx config                          # list everything (allow: keys hidden)
phlx config trivy.path                # read one key
phlx config trivy.path /usr/bin/trivy # set one key
```

Configuration values are stored in the same SQLite database. Keys with
the `allow:` prefix are managed via `phlx allow` and hidden from the
listing for clarity.

---

## `update`

```bash
phlx update                # check + install
phlx update --check        # check only, don't install
phlx update --force        # reinstall the latest even if already current
```

| Flag | Default | Description |
|---|---|---|
| `--check` | `false` | Print current + latest, then exit. No download. |
| `--force` | `false` | Skip the same-version short-circuit. |

The updater:

1. Calls the GitHub Releases API for `satnambhatt/phlx`.
2. Picks the archive matching the running OS / arch (`linux_x86_64`,
   `macos_arm64`, `windows_x86_64`, etc.) — same naming as the manual
   install snippets.
3. Downloads to a temp directory, extracts, and atomically replaces
   `phalanx` and every shipped hook binary
   (`phalanx-npm-hook`, `phalanx-pip-hook`, `phalanx-yarn-hook`) next
   to the running executable.
4. Falls back to copy-then-rename when the temp dir is on a different
   filesystem from the install dir.

The updater needs write access to the install directory. If you
installed under `/usr/local/bin`, run `sudo phlx update`. If you used
`go install`, the binary lives under `$GOBIN` (typically
`$HOME/go/bin`) and `sudo` is not needed.

Existing shims under `~/.phalanx/bin/` point at the absolute hook paths
recorded at `hooks install` time, so an in-place update of the hook
binaries takes effect immediately — no need to re-run
`phlx hooks install` after a successful update.

---

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success / clean / warned |
| `1` | Blocked, scan error, update failure |

The hooks always return the real package manager's exit code on the
pass-through path. A non-zero exit from `npm`, `yarn`, or `pip` is
forwarded unchanged.
