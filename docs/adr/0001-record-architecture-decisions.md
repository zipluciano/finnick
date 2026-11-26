# 0001. Record architecture decisions

- Status: Accepted
- Date: 2026-11-25

## Context

Finnick handles decryption keys and secrets, so many of its choices exist for security reasons that
are not obvious from the code. Contributors come and go, and a decision that is only in someone's head, a
chat or a private assistant configuration is lost.

## Decision

Record significant decisions as architecture decision records in `docs/adr/`, in Markdown, numbered in
order. The folder describes the decisions in force: when a decision changes its ADR is updated, and
when it no longer applies its ADR is deleted. Git history keeps the earlier versions.

## Consequences

- Reviewers can ask for an ADR when a pull request makes a hard-to-reverse or security-relevant choice.
- `docs/architecture.md` describes the current design; the ADRs explain why it is that way.
