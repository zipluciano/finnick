# Architecture decision records

Each file records one decision: the context, what was decided and what follows from it. They are the
project's memory, for people and for the coding assistants they use (see `AGENTS.md`).

Rules:

- This folder holds only the decisions in force. Every ADR here is accepted and should be followed.
- When a decision changes, update its ADR to describe the new decision, in the same pull request as the
  change. Do not add an ADR that amends or supersedes another one.
- When a decision no longer applies, delete its ADR and fix the links to it. Git history keeps every
  earlier version.
- Number new ADRs in order (`NNNN-short-title.md`) and never reuse a number; copy the shape of an existing one, and open the ADR in the
  same pull request as the change it explains.
- Not every change needs one. Write an ADR when the decision is hard to reverse, affects security, or a
  future contributor would otherwise ask "why is it like this?".

| ADR | Title | Status |
| --- | --- | --- |
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0002](0002-local-tool-no-hosted-service.md) | A local tool, not a hosted service | Accepted |
| [0003](0003-follow-the-sops-cli.md) | Use the SOPS library and behave like the sops CLI | Accepted |
| [0004](0004-one-repository-per-run.md) | One repository per run | Accepted |
| [0005](0005-loopback-server-security.md) | Security model of the loopback server | Accepted |
| [0006](0006-bundled-ui-and-design-system.md) | UI embedded in the binary, with its own design system | Accepted |
| [0007](0007-preferences-and-translations.md) | Preferences on disk and translations checked by the compiler | Accepted |
| [0008](0008-ai-assistant-instructions-in-the-repository.md) | Instructions for AI assistants live in the repository | Accepted |
| [0009](0009-writing-sops-files.md) | Writing SOPS files | Accepted |
| [0010](0010-editing-sops-yaml.md) | Editing .sops.yaml | Accepted |
| [0011](0011-requesting-access.md) | Requesting access: creating keys and proposing changes with git | Accepted |
