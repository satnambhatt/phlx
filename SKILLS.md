# SKILLS.md

Task-shaped recipes for AI coding assistants working on Phalanx. Each skill
is a self-contained playbook: when to use it, what files to touch, and
which invariants to respect. Read `AGENTS.md` first for repo-wide
conventions.

---

## Skill: Add a new CLI subcommand

**When**: the user wants `phlx <verb>` to do something new (e.g.
`phlx export`, `phlx policy lint`).

**Steps**:
1. Add a function `myCmd() *cobra.Command` in `internal/cli/commands.go`
   (or a new file in `internal/cli/` for substantial commands like
   `update.go`).
2. Wire it into the slice in `Root(version)` so it appears in `--help`.
3. Call `banner()` at the top of `RunE` for any command that prints
   user-facing output.
4. Use existing `color` helpers — never raw ANSI escapes.
5. Database access goes through `internal/db`; never open the SQLite file
   directly from a new command.

**Don't**: introduce a `urfave/cli` style or invent a new flag parser —
cobra is the standard everywhere.

---

## Skill: Add a new package ecosystem (cargo, gem, composer, ...)

**When**: the user wants Phalanx to gate a package manager other than
npm / yarn / pip.

**Steps**:
1. Add resolver + tarball downloader in `internal/registry/` mirroring
   `ResolveNpmVersion` / `ResolvePypiVersion`.
2. Create `cmd/phalanx-<eco>-hook/main.go` — a ~25-line `main` that
   builds a `hookcore.Config` and calls `hookcore.Run(cfg)`. Clone one
   of the existing hooks; everything ecosystem-shaped (subcommand
   names, regex, skip prefixes, flags-with-value) is plain config.
3. If the ecosystem needs behaviour the existing `hookcore.Parser`
   can't express, extend `internal/hookcore/hookcore.go` rather than
   inlining logic in the cmd.
4. Add the binary to:
   - `.goreleaser.yaml` builds list **and** the `archives.ids` list
   - `internal/cli/update.go` `hookBinaries` slice
   - `internal/hooks/install.go` `Install()` and `Remove()` shim lists
5. Wire ecosystem-specific scan rules in `internal/scanner/` and
   `internal/policy/` if the existing rules don't apply cleanly.
6. Add the ecosystem to `docs/cli.md` and to the CLI reference table in
   `README.md`.

**Invariant**: every new hook must fail open. `hookcore.Run` already
implements the fail-open behaviour — keep ecosystem-specific code out
of the cmd file so this stays true by construction.

---

## Skill: Add a new analyser to the scan pipeline

**When**: detecting a new class of badness (e.g. install-script abuse,
postinstall network calls).

**Steps**:
1. Add the analyser as a new file in `internal/analysers/`. It must
   expose a function that takes the extracted tarball path and returns
   structured findings.
2. Wire it into the orchestrator in `internal/scanner/`. Decide whether
   it joins the parallel goroutines (stages 3 + 4) or runs serially.
3. Add a deny / warn rule in `internal/policy/` keyed off the new
   finding type. Use the existing `[TAG]` prefix convention so the CLI
   colourises the message (`[BEHAVIOUR]`, `[CVE]`, etc.).
4. Document the rule in the README's "5-stage scan pipeline" table.

**Don't**: change the order of existing stages. Typosquat-first / parallel
CVE+behaviour is a deliberate latency optimisation.

---

## Skill: Modify the self-updater

**When**: the user wants different update behaviour (channels, pinned
versions, signature verification).

**Steps**:
1. Touch `internal/cli/update.go` only.
2. Keep `archiveName()` aligned with the `name_template` in
   `.goreleaser.yaml`. If you change either, change both.
3. Keep `hookBinaries` complete — every binary the project ships must be
   replaced atomically.
4. Preserve `replaceBinary`'s rename-with-copy-fallback. `/tmp` is often
   a different filesystem from `/usr/local/bin`, so the rename can fail
   with `EXDEV`.
5. The updater must be a no-op when already on the latest version unless
   `--force` is set.

**Verify**: build, then run `./bin/phalanx update --check`. The command
should print current + latest version and exit cleanly without writing
anything.

---

## Skill: Investigate a "Phalanx blocked my install" report

**When**: a user reports a false positive.

**Steps**:
1. Reproduce with `phlx scan <pkg>@<version>` (no install needed).
2. Look at the deny messages — each is tagged with `[TYPOSQUAT]`,
   `[CVE]`, `[BEHAVIOUR]`, `[FRESHNESS]`, or `[LICENCE]`. That tag
   points at the analyser in `internal/analysers/` or the policy rule
   in `internal/policy/`.
3. Run `phlx history -n 20` to confirm the block was recorded.
4. If the verdict is genuinely wrong, fix the analyser / rule. If the
   user just needs to ship, point them at
   `phlx allow <pkg>@<version> -r "<reason>"`.

**Don't**: weaken the default policy to fix one user's edge case. Prefer
to make the analyser more precise.

---

## Skill: Wire Phalanx into an AI coding tool

**When**: a user wants their AI assistant to use Phalanx-gated installs.

**Steps**:
1. Confirm Phalanx is installed: `phlx --version`.
2. Confirm hooks are active: `which npm`, `which yarn`, `which pip3` —
   each should resolve under `~/.phalanx/bin/`.
3. Point the AI tool at `docs/ai-coding-tools.md` for the per-tool
   configuration snippets.
4. Remind the user that the AI assistant inherits the developer's shell,
   so its `npm install` calls will route through the shim automatically.
   No tool-specific integration is required.

---

## Skill: Cut a release

**When**: shipping a new version.

**Full runbook**: [`docs/releases.md`](docs/releases.md) — read it before
the first release. The summary below is for cases where you have the
context and just need the steps.

**Steps**:
1. `go vet ./... && go build ./...` clean.
2. `scripts/build.sh --all-platforms` — builds inside the pinned Go
   Docker image and runs post-build sanity checks (size, static link,
   `--version` stamping, `--help` exit code). Do not tag if any check
   fails.
3. Update CHANGELOG if one exists. Otherwise rely on goreleaser's
   generated changelog from commit messages (`feat:` / `fix:` prefixes
   land in the right group).
4. `git tag -a vX.Y.Z -m vX.Y.Z && git push origin vX.Y.Z`.
5. The `release` GitHub workflow runs goreleaser, which builds every
   binary for every os/arch listed in `.goreleaser.yaml`, packages
   them, generates checksums, and creates the GitHub Release.
6. Once published, `phlx update` will pick it up — verify by running
   `phlx update --check` against a binary on the previous tag.

**Invariants**:
- `archiveName()` in `internal/cli/update.go` and the `name_template`
  in `.goreleaser.yaml` must produce identical filenames. Change one,
  change both.
- Every shipped binary (main + each hook) must appear in
  `.goreleaser.yaml`'s `archives.ids` AND in `hookBinaries` in
  `internal/cli/update.go`, or the self-updater will leave stale
  binaries behind.

**Don't**: hand-edit a release on GitHub. The release notes header and
install snippet live in `.goreleaser.yaml`. For genuine emergencies see
the "Manual binary upload" section of `docs/releases.md`.
