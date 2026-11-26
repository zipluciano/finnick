<h1 align="center">
  <img src="docs/assets/finnick-header.svg#gh-light-mode-only" alt="Finnick" width="480">
  <img src="docs/assets/finnick-header-dark.svg#gh-dark-mode-only" alt="Finnick" width="480">
</h1>

A local UI for secrets kept with [SOPS](https://github.com/getsops/sops). No server, no account, no lock-in.
Your secrets stay in git, encrypted with age (or KMS, PGP, Vault…), and Finnick gives you a friendly
interface over them: browse your secret files, reveal and copy values, see who can decrypt what,
and spot files that are out of sync with `.sops.yaml`.

It runs inside a repository: the one with your code, a `.sops.yaml` and your encrypted files (for example
`secrets/dev.env`, `secrets/prod.env`). Any layout and any SOPS format work.

New to SOPS or age? The [step-by-step guide](docs/guide.md) ([Português](docs/guide.pt-BR.md)) goes from
creating your key to CI, Docker and Finnick.

> Status: early. Read-only for now (browse, reveal, access overview). Editing, people management and git sync
> are next: see [docs/roadmap.md](docs/roadmap.md).

## Install

macOS, with [Homebrew](https://brew.sh):

```sh
brew install --cask zipluciano/tap/finnick
```

Linux:

```sh
VERSION=0.1.0 ARCH=amd64   # or arm64
curl -fsSLO "https://github.com/zipluciano/finnick/releases/download/v${VERSION}/finnick_${VERSION}_linux_${ARCH}.tar.gz"
tar xzf "finnick_${VERSION}_linux_${ARCH}.tar.gz" finnick
sudo install -m 755 finnick /usr/local/bin/finnick
```

Windows: download `finnick_<version>_windows_amd64.zip` (or `arm64`) from
[Releases](https://github.com/zipluciano/finnick/releases) and put `finnick.exe` on your `PATH`.

Every release lists `checksums.txt` to verify the download. The macOS binaries are not signed by Apple yet;
the Homebrew cask removes the quarantine flag so they run. `go install` is not supported, because the UI
is built with Node and embedded at release time: use a release or [build from source](#build-from-source).

## Quick start

```sh
cd my-app      # anywhere inside the repository
finnick ui     # opens http://127.0.0.1:<port> in your browser
finnick ls     # same information as a table
```

Finnick finds `.sops.yaml` the same way `sops` does (current directory, then parents) and opens the
repository it belongs to. Without a `.sops.yaml` it falls back to the git root and only lists encrypted
files. `--dir` opens another directory.

Like Drizzle Studio, it fits in `package.json`:

```json
{ "scripts": { "secrets": "finnick ui" } }
```

It uses the same keys as `sops`: `SOPS_AGE_KEY`, `SOPS_AGE_KEY_FILE`, or the default `keys.txt`. On macOS it
also picks up `~/.config/sops/age/keys.txt` when nothing else is configured.

## What you get

- **Secrets**: every SOPS file in the repository (dotenv, YAML, JSON, INI, binary), grouped by folder. Key names without
  decrypting (SOPS stores them in clear), values on demand, MAC verified, hidden again after 2 minutes.
- **Access**: who can decrypt each file, read from the file itself, with names taken from YAML anchors in
  `.sops.yaml` (`- &alice age1…` shows as *alice*).
- **Drift**: files whose recipients differ from their creation rule (someone was added to or removed from
  `.sops.yaml` but `sops updatekeys` was not run), with the command to fix it.
- **People & keys**: a matrix of every recipient against every file.
- **Rules**: `creation_rules` in order, which files each one matches, and files no rule covers.
- **Light, dark or system theme** and **English, Português (Brasil) and Español**, switchable from the
  sidebar. Choices are saved to `~/.config/finnick/settings.json` (the UI runs on a random port, so
  browser storage would forget them).

## Security model

Finnick runs on your machine with your keys, so it is built to be unusable by anything else:

- listens on `127.0.0.1` only, on a random port by default;
- the browser gets in through a one-time token in the URL the CLI opens; it becomes an `HttpOnly`,
  `SameSite=Strict` session cookie;
- the `Host` header must be the loopback address (blocks DNS rebinding);
- calls that reveal values are `POST` with a custom header (CSRF);
- strict Content-Security-Policy, `no-store` on every response, no external requests: the design system,
  icons and fonts are bundled in the binary.

Decryption happens in the Go process using SOPS' own library. Nothing is written to disk in clear text.

## Build from source

Requires Go 1.26+ and Node 22+.

```sh
make build      # builds web/ and embeds it into bin/finnick
make test
make snapshot   # release archives for every platform in dist/ (needs GoReleaser)
```

UI development with hot reload:

```sh
make dev-api DIR=path/to/repo    # API on :7777, prints an /auth?token=… URL
make dev-web                     # Vite on :5173
# open http://localhost:5173/auth?token=<token from dev-api>
```

Project layout:

```
cmd/finnick/           CLI entry point (ui, ls, version)
internal/workspace/    .sops.yaml parsing, file discovery, drift, decryption (uses getsops/sops/v3)
internal/server/       loopback HTTP server and JSON API
web/                   React + TypeScript UI, built with Vite and embedded via go:embed
web/src/design/        the design system: tokens.css, base.css, components.css
web/src/i18n/          translations (en.ts is the source of truth; other locales are type-checked against it)
docs/                  guide (en, pt-BR), architecture and roadmap
docs/adr/              architecture decision records
```

## Translations

Add a locale by copying `web/src/i18n/es.ts`, translating every value, and registering it in
`web/src/prefs.tsx` (`LOCALES`, `dictionaries`, `browserLocale`) and `internal/settings` (`Locales`). The
TypeScript build fails if a key is missing. Keys ending in `_one`/`_other` are plural forms.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Design decisions are recorded in [docs/adr/](docs/adr/).
Security issues: [SECURITY.md](SECURITY.md).

## License

[Apache License 2.0](LICENSE). Finnick is an independent project, not affiliated with the SOPS
project or the CNCF.
