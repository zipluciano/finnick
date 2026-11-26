# AGENTS.md

Instructions for coding assistants working on Finnick. People should read
[CONTRIBUTING.md](CONTRIBUTING.md); the reasons behind these rules are in [docs/adr/](docs/adr/).

## What this is

A local web UI for repositories that keep secrets with SOPS. One Go binary (`finnick ui`) serves an
embedded React UI on `127.0.0.1` and opens the repository it runs in. It reads and edits SOPS files; what
comes next is in `docs/roadmap.md`.

## Commands

```sh
make build        # npm build of web/ + go build → bin/finnick
make test         # go test ./... + tsc --noEmit
go test -race ./...
gofmt -l cmd internal web/embed.go   # must print nothing
make snapshot                        # GoReleaser archives in dist/, nothing published
make dev-api DIR=path/to/repo        # API on :7777 (prints an /auth?token=… URL)
make dev-web                         # Vite on :5173, proxies /api to :7777
```

Workspace tests run the real `sops` binary and skip without it. Use throwaway age keys
(`age-keygen`) in tests and examples; never read, print or commit real secrets or private keys.

## Layout

```
cmd/finnick/          CLI: ui, ls, version
internal/workspace/   .sops.yaml parsing, file discovery, drift, decryption and writes (getsops/sops/v3)
internal/server/      loopback HTTP server, auth, JSON API
internal/settings/    theme and locale in ~/.config/finnick/settings.json
web/src/              React + TypeScript UI (App.tsx, api.ts, views/, components/)
web/src/design/       design system: tokens.css, base.css, components.css
web/src/i18n/         en.ts (source of truth), pt-BR.ts, es.ts
docs/                 guide (en, pt-BR), architecture, roadmap, adr/
```

## Rules

- **Behave like the sops CLI**: config lookup, first matching rule, key lookup, MAC check. Use the SOPS
  library, do not reimplement it ([ADR 0003](docs/adr/0003-follow-the-sops-cli.md)).
- **Server security** ([ADR 0005](docs/adr/0005-loopback-server-security.md)): every route goes through
  the existing middleware (host check, session cookie, `no-store`, CSP). Anything that reveals values or
  changes state is `POST`/`PUT` and requires the `X-Finnick: 1` header. New endpoints get tests for
  these checks. Never write clear text to disk or logs.
- **Writes** ([ADR 0009](docs/adr/0009-writing-sops-files.md)): go through `resolveWrite`, check the
  file's version, keep the data key unless the operation is a rotation, and replace files atomically.
  Test written files with the real `sops` binary.
- **Errors**: the API returns error codes (`not_recipient`, `integrity`, …), never translated sentences.
  The UI translates them under `errors.*`.
- **Translations**: every user-facing string uses `t()`/`tp()`/`tn()` and is added to `en.ts`,
  `pt-BR.ts` and `es.ts` in the same change. Plurals use `_one`/`_other` keys.
- **Styling**: use the tokens in `tokens.css`; check light and dark. No CSS frameworks, UI kits, web
  fonts or CDNs ([ADR 0006](docs/adr/0006-bundled-ui-and-design-system.md)).
- **Dependencies**: keep them few; justify any new one in the pull request.
- **Scope**: one repository per run, no central vault or project registry
  ([ADR 0004](docs/adr/0004-one-repository-per-run.md)).
- **Docs**: update README, `docs/guide.md` and `docs/guide.pt-BR.md` when behaviour changes. Write a new
  ADR for hard-to-reverse or security-relevant decisions. When a decision changes, update its ADR (or
  delete it if the decision no longer applies) instead of adding one that supersedes it.
- **Commits**: Conventional Commits in English, imperative subject, body explaining why.
- **Pull requests**: always set an assignee (`gh pr create --assignee <login>`); CI fails without one.
- Keep this file short and current: update it when a command or convention changes.
