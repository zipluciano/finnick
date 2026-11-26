# 0002. A local tool, not a hosted service

- Status: Accepted
- Date: 2026-11-25

## Context

SOPS keeps secrets encrypted in git and decrypts them offline with the user's own key. People who choose
it usually do so to avoid a secrets server (Vault, Infisical, Doppler). What they miss is a friendly way to
browse and edit those files.

A hosted UI would need the users' private keys, or would have to decrypt on their behalf. It would become
one more secrets service, with the availability, compliance and breach obligations that SOPS users
avoided.

## Decision

Finnick is a single binary that runs on the user's machine, like `git` or `sops`. It reads the
repository and the user's keys, serves a web UI on the loopback interface and has no database of its own.
Everything it shows comes from `.sops.yaml`, the encrypted files and git.

## Consequences

- No accounts, no telemetry, no network calls besides those SOPS itself makes for KMS keys.
- Nothing to back up or migrate: deleting Finnick loses nothing.
- Features that need a central server (read audit, instant revocation) are out of scope. The docs say so
  plainly instead of pretending otherwise.
- Every UI feature can also be a CLI command, because both use the same `internal/workspace` package.
