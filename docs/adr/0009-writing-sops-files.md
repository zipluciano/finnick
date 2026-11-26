# 0009. Writing SOPS files

- Status: Accepted
- Date: 2026-11-26

## Context

Finnick changes encrypted files that people also change with `sops`, in their editor, and through git
(pull, merge, rebase). A write that loses someone else's change, leaves a half-written file, produces
something `sops` rejects, or puts clear text on disk is worse than not having the feature.

## Decision

- **Like `sops edit`.** An edit decrypts the file with the SOPS library, verifies its MAC, changes the
  tree, and encrypts it again with the same data key, a new `lastmodified` and a new MAC. Comments, key
  order and the file's encryption settings (`unencrypted_suffix`, `encrypted_regex`, …) are kept. YAML
  and JSON indentation follow the `stores` section of `.sops.yaml`, as in the CLI.
- **No lost updates.** Revealing a file returns a version (the SHA-256 of its bytes). An edit carries that
  version and is refused with `stale` if the file changed since; the UI then reloads the values so the
  change can be made again on top.
- **All or nothing.** The edits of one request are applied in memory and written once. If any of them is
  invalid, the file is not touched.
- **Atomic replace.** The new content is written to a temporary file in the same directory, synced and
  renamed over the original, keeping its permissions. Only encrypted content is ever written.
- **Only where it belongs.** Writes go through the same path checks as reads and also refuse
  `.sops.yaml`, anything under `.git`, symlinks, and directories that lead outside the repository.
- **Values keep their type.** A new value that parses as the old one's type (number, boolean) stays that
  type; anything else is a string. New keys are strings.
- **What is left to `sops edit`.** Binary files, YAML files with several documents, and changes to whole
  maps or lists (adding list items, replacing a map with a value).

## Consequences

- Every save rewrites `sops.mac` and `sops.lastmodified`, like `sops edit`, so concurrent edits of the
  same file still conflict in git. The per-key merge on the roadmap addresses that.
- Tests check written files with the real `sops` binary.
- Changes to how files are written update this ADR.
