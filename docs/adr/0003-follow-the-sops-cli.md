# 0003. Use the SOPS library and behave like the sops CLI

- Status: Accepted
- Date: 2026-11-25

## Context

Users will keep using the `sops` CLI next to Finnick, in scripts, CI and their editor. If the two
disagree on which `.sops.yaml` applies, which rule matches a file or which key is used, the UI would show
one thing while `sops` does another, which is worse than no UI.

## Decision

- Read and decrypt files with `github.com/getsops/sops/v3` (stores, key sources, AES-GCM), not with a
  reimplementation.
- Copy the CLI's behaviour where the library does not expose it: config lookup from the working directory
  upwards, first matching `creation_rules` entry with `path_regex` relative to the config's directory, age
  keys from `SOPS_AGE_KEY`, `SOPS_AGE_KEY_FILE` or the default `keys.txt`.
- Verify the MAC before showing any value.
- Writes must produce files that `sops` reads and edits without noticing a difference
  ([ADR 0009](0009-writing-sops-files.md)).

## Consequences

- Finnick follows SOPS releases through `go.mod`. A SOPS upgrade that changes behaviour is a change
  here too, with tests.
- Tests that touch the file format run against the real `sops` binary (they skip when it is missing).
- On macOS Finnick also tries `~/.config/sops/age/keys.txt`, where many guides put the key; this is
  the one place where it is more lenient than the CLI, and it never overrides an explicit setting.
