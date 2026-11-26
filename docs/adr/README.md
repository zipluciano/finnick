# Architecture decision records

Each file records one decision: the context, what was decided and what follows from it. They are the
project's memory, for people and for the coding assistants they use (see `AGENTS.md`).

Rules:

- An accepted ADR is not rewritten. To change a decision, add a new ADR that supersedes it and set the old
  one's status to `Superseded by NNNN`. Typos and broken links can be fixed in place.
- Number them in order (`NNNN-short-title.md`), copy the shape of an existing one, and open the ADR in the
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
