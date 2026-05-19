# Using Phalanx with AI coding tools

AI coding assistants (Claude Code, Cursor, Aider, Continue, GitHub Copilot
Workspace, Codex, etc.) install packages by shelling out to `npm`,
`yarn`, or `pip` exactly the same way a human does. Phalanx hooks into
the package manager — not the editor — so **every AI tool that runs a
shell command gets gated automatically once `phlx hooks install` is
done**. No tool-specific plugin or wrapper is required.

This page covers:

- One-time setup that makes any AI tool safe by default
- Files in this repo that orient AI tools at the codebase
- Per-tool notes for the common cases

---

## One-time setup

1. **Install Phalanx** (see [README](../README.md#install)).
2. Install the shims:
   ```bash
   phlx hooks install
   source ~/.zshrc          # or ~/.bashrc / ~/.zprofile
   ```
3. Confirm the AI tool's shell sees the shims:
   ```bash
   which npm    # → ~/.phalanx/bin/npm
   which yarn   # → ~/.phalanx/bin/yarn
   which pip3   # → ~/.phalanx/bin/pip3
   ```
4. (Optional) Keep Phalanx fresh:
   ```bash
   phlx update --check
   ```

That's the whole integration. When the assistant runs `npm install foo`,
the shim runs `phalanx-npm-hook` first, which scans `foo` and either
blocks the install or hands the original command to the real `npm`.

---

## Files in this repo for AI tools

| File | Purpose |
|---|---|
| [`AGENTS.md`](../AGENTS.md) | Repo-wide briefing — invariants, layout, what not to do |
| [`SKILLS.md`](../SKILLS.md) | Task-shaped recipes (add a command, add an ecosystem, cut a release) |
| [`docs/cli.md`](cli.md) | Full CLI reference — point tools here for command syntax |
| `README.md` | Human-facing intro; AI tools should prefer `AGENTS.md` |

If your AI tool reads a single file by convention (Cursor reads
`.cursorrules`, Aider reads `CONVENTIONS.md`, Continue reads
`.continue/`), have it source `AGENTS.md` from there. A one-line include
is enough — keep the content in `AGENTS.md` so every tool sees the same
guidance.

---

## Per-tool notes

### Claude Code

Claude Code runs inside the developer's shell, so the shims activate
automatically. Worth knowing:

- Add the repo with `claude .` from anywhere inside the working tree —
  Claude will read `CLAUDE.md` (if present), `AGENTS.md`, and `README.md`
  during the session.
- For background sessions or GitHub Actions sessions in this repo, install
  Phalanx in the session's `SessionStart` hook so any installs the agent
  runs are gated:
  ```bash
  # .claude/session-start.sh
  command -v phlx >/dev/null || curl -sSL https://github.com/satnambhatt/phlx/releases/latest/download/phlx_linux_x86_64.tar.gz | tar -xz -C /usr/local/bin
  phlx hooks install
  ```
- See the [session-start-hook docs](https://code.claude.com/docs/en/claude-code-on-the-web)
  for setup details.

### Cursor

- Cursor uses your shell's `PATH`, so the shim works as-is.
- Point Cursor at `AGENTS.md` by adding to `.cursorrules`:
  ```
  Read AGENTS.md before making any changes. Follow SKILLS.md when the
  task matches one of the listed recipes.
  ```

### Aider

- Aider invokes `git` and `pip` directly — both will hit the shim.
- Add `AGENTS.md` and `SKILLS.md` to the chat with `/add AGENTS.md
  SKILLS.md` so Aider keeps them in context.

### GitHub Copilot Workspace / Codespaces

- Pre-install Phalanx in the devcontainer:
  ```jsonc
  // .devcontainer/devcontainer.json
  {
    "postCreateCommand": "curl -sSL https://github.com/satnambhatt/phlx/releases/latest/download/phlx_linux_x86_64.tar.gz | tar -xz -C /usr/local/bin && phlx hooks install"
  }
  ```
- Reload the shell once after first creation so `PATH` picks up
  `~/.phalanx/bin`.

### Continue / Cody / generic LLM agents

- Anything that runs `bash -c "npm install …"` is already covered.
- If the tool sandboxes `PATH`, ensure `~/.phalanx/bin` is prepended in
  the sandbox's environment, not just the parent shell's.

---

## When Phalanx blocks an AI's install

The assistant sees a non-zero exit code and the deny output on stderr.
Most agents will either:

- Surface the block verbatim — let the developer decide whether to
  `phlx allow <pkg>@<version> -r "<reason>"` and retry, **or**
- Retry with a different package — which is usually the right outcome,
  because the block tagged the original choice as a typosquat or
  vulnerable version.

Do **not** configure the AI tool to auto-bypass blocks. The allow list
is intentionally a manual step.

---

## Verifying end-to-end

Pick a known typosquat to confirm the wiring works inside the AI tool's
shell:

```bash
phlx scan crossenv          # expect: BLOCKED
# Then ask the assistant to run:
npm install crossenv        # expect: BLOCKED, exit 1, real npm never runs
```

If the second command goes through, the shim is not on `PATH` for the
assistant's shell. Re-run `phlx hooks install` and source the shell rc
in the assistant's environment.
