# 0008. Instructions for AI assistants live in the repository

- Status: Accepted
- Date: 2026-11-25

## Context

Many contributors use coding assistants (Claude Code, Codex, Cursor, Copilot, Gemini and others). Without
project instructions, each assistant guesses the commands and conventions, and pull requests drift from
the rules in this repository: missing translations, colours outside the design tokens, new endpoints
without the security checks.

Large open-source projects such as Bun commit their assistant instructions, so every contributor's tool
starts from the same context. The risk is duplication: instructions that repeat the docs go stale.

## Decision

- `AGENTS.md` at the root is the single set of instructions for assistants. It is the open format read
  by most tools. `CLAUDE.md` only imports it (`@AGENTS.md`), so Claude Code reads the same text.
- `AGENTS.md` stays short: commands, layout and the rules reviewers check. For the reasons behind them it
  links to `CONTRIBUTING.md`, `docs/architecture.md` and these ADRs instead of repeating them.
- Tool-specific files (`.claude/`, `.cursor/`, `.github/copilot-instructions.md`, skills) may be
  committed when they help every contributor, and are reviewed like code.
- Personal settings stay out of git: `CLAUDE.local.md` and `.claude/settings.local.json` are ignored.

## Consequences

- A pull request that changes a command or a convention updates `AGENTS.md` too (the pull request
  template asks for it).
- Instructions are public and reviewed. They must never contain secrets, private keys or internal URLs.
- Contributors remain responsible for what their assistants produce (see `CONTRIBUTING.md`).
