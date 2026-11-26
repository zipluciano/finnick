# Security policy

Finnick runs with access to your decryption keys, so security reports are taken seriously.

## Reporting a vulnerability

Please **do not open a public issue**. Report privately through GitHub:
[Security → Report a vulnerability](https://github.com/zipluciano/finnick/security/advisories/new).

Include the version (`finnick version`), your OS and browser, and the steps to reproduce. Use
throwaway keys and fake secrets in any example.

You can expect an acknowledgement within a week. Once a fix is released, the advisory is published with
credit to the reporter, unless you prefer otherwise.

## Supported versions

Only the latest release gets security fixes while the project is below 1.0.

## Scope

In scope, for example:

- another local user, process or web page reading values or calling the API without the session token;
- bypasses of the host check, the CSRF header or the Content-Security-Policy;
- clear text written to disk, logs, caches or browser history;
- values shown for a file whose MAC does not verify.

Out of scope, because they are limits of SOPS and git that the docs describe
([docs/architecture.md](docs/architecture.md#known-limits-by-design-of-sops--git)):

- someone removed from `.sops.yaml` still reading old versions from git history;
- anyone with write access to the repository replacing an encrypted file;
- an attacker who already runs code as your user or has your private key.

The design is recorded in [ADR 0005](docs/adr/0005-loopback-server-security.md).
