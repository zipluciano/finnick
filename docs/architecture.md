# Architecture

This document describes the current design. The reasons behind each choice are in the
[decision records](adr/README.md).

## Principles

1. **The repository is the source of truth.** Finnick never keeps its own database. Everything it shows
   is derived from `.sops.yaml`, the encrypted files and git.
2. **No server.** It is a local tool, like `git` or `sops`. A hosted service would hold keys and become a
   Vault/Infisical clone with all of their security obligations; that is out of scope.
3. **Same behavior as the sops CLI.** Config lookup, rule matching (first match wins, path relative to the
   config directory), key lookup and MAC verification follow SOPS, using its Go library where the API allows.
4. **Make the crypto model visible, not hidden.** Access is cryptographic and per file. The UI shows when
   `.sops.yaml` and a file disagree, and states plainly that removing a recipient does not revoke history.
5. **One repository at a time.** Secrets live next to the code that uses them, so a change to code and
   to its configuration is one commit and one review. Finnick opens the repository it runs in, with
   no required folder structure; files are grouped by directory.

## Components

```
            browser (React UI, embedded)
                   │  same-origin JSON, session cookie
                   ▼
   internal/server ─ loopback HTTP, auth token, host check, CSRF header, CSP
                   │
                   ▼
 internal/workspace ─ Open (find .sops.yaml) · Scan (find SOPS files) · Reveal (decrypt) · Edit · Create · UpdateKeys · Rotate · EditConfig
                   │
                   ▼
   getsops/sops/v3 ─ stores (dotenv/yaml/json/ini/binary), key sources (age, KMS, …), AES-GCM
```

The CLI (`cmd/finnick`) and the UI share the same `workspace` package, so every UI feature can also
be a command.

## Data model

- **Workspace**: root directory (where `.sops.yaml` lives, or the given dir) and the parsed config.
- **Rule**: one `creation_rules` entry, its regex and key groups. Recipients get a `name` from YAML anchors.
- **File**: path, format, SOPS metadata (recipients per key group, last modified), key names, the rule that
  matches it, and **drift** (recipients missing from or extra in the file compared with the rule).
- **Identity**: public keys derived from the local age private keys, used to tell "you can decrypt".

## Known limits (by design of SOPS + git)

- Permissions are per file, not per secret.
- No read audit: decryption happens offline.
- Write access to the repo lets someone overwrite a file even without being able to read it. Branch
  protection and review are the control, not Finnick.
- Removing someone from a rule only protects future versions; secrets they could read must be rotated.

## Writes

Editing uses the SOPS library to decrypt the tree, apply the changes, and encrypt it again with the existing
data key, producing the same output as `sops edit` ([ADR 0009](adr/0009-writing-sops-files.md)). Each edit
carries the version (SHA-256) of the file it was based on and is refused if the file changed since; the new
content replaces the file atomically.

New files take their recipients from the matching creation rule. **Update keys** re-encrypts a file's data key
for the recipients its rule lists now (`sops updatekeys`), and **rotate** replaces the data key (`sops rotate`).

Adding and removing people changes `.sops.yaml` as text, guided by a yaml.v3 parse, so comments, blank lines
and anchors stay; the result is read back and must mean exactly what was asked before it replaces the file
([ADR 0010](adr/0010-editing-sops-yaml.md)). The UI then updates keys and rotates the affected files.

### Git sync and the merge problem

Every SOPS edit rewrites `sops.mac` and `sops.lastmodified`, so two people editing different keys of the
same file always conflict in git. Finnick can resolve this: decrypt base, ours and theirs, run a
three-way merge per key on the clear-text trees, and re-encrypt. Only real conflicts (same key changed on
both sides) are shown to the user. This can also be offered as a git merge driver for CLI users.
