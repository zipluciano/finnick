# Contributing to Finnick

Thanks for helping. Bug reports, translations, docs and code are all welcome.

## Before you start

- **Questions and ideas**: open a [discussion](https://github.com/zipluciano/finnick/discussions).
- **Bugs**: open an issue with the bug template. Never paste real secrets or private keys; use throwaway
  age keys (`age-keygen`) in examples.
- **Security problems**: report privately, as described in [SECURITY.md](SECURITY.md).
- **Larger changes** (new feature, new dependency, change to the security model): open an issue first so
  we agree on the approach before you spend time on it. Check [docs/roadmap.md](docs/roadmap.md).

Please read [docs/architecture.md](docs/architecture.md) and the decision records in
[docs/adr/](docs/adr/). They explain why things are the way they are.

## Development setup

You need Go (version in `go.mod`), Node 22 or newer, and `sops` and `age` for the tests and for trying
things out.

```sh
make build                       # build the UI and bin/finnick
make test                        # go test ./... and the TypeScript check
make dev-api DIR=path/to/repo    # API on :7777, prints an /auth?token=… URL
make dev-web                     # Vite with hot reload on :5173
# then open http://localhost:5173/auth?token=<token from dev-api>
```

For a playground repository with throwaway keys:

```sh
mkdir /tmp/playground && cd /tmp/playground && git init
age-keygen -o key.txt
export SOPS_AGE_KEY_FILE=$PWD/key.txt
cat > .sops.yaml <<EOF
creation_rules:
  - path_regex: ^secrets/.*
    age: $(age-keygen -y key.txt)
EOF
mkdir secrets && printf 'API_URL=http://localhost\nTOKEN=abc\n' > secrets/dev.env
sops encrypt -i secrets/dev.env
```

## Making a change

1. Fork, create a branch from `main`, and keep the pull request focused on one thing.
2. Add or update tests. Go tests that need the real `sops` binary skip when it is missing, so install it
   to run them.
3. Run `make test` and `gofmt`. CI runs the same checks plus `go vet` and a race-enabled test run.
4. Update the docs when behaviour changes: `README.md`, `docs/guide.md` (and `docs/guide.pt-BR.md`),
   `docs/architecture.md` or `docs/roadmap.md`.
5. Write an ADR in `docs/adr/` when the change is hard to reverse, touches security or would make a future
   contributor ask "why?". Accepted ADRs are not rewritten; a new one supersedes them.

### Commits

Use [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `refactor:`,
`test:`, `chore:`), in English, in the imperative. The subject says what changes; the body says why.

### UI and translations

- Every user-facing string goes through `t()` and must exist in `web/src/i18n/en.ts`, `pt-BR.ts` and
  `es.ts`. The TypeScript build fails if a locale is missing a key. If you cannot translate a string,
  say so in the pull request and someone will help.
- Use the tokens in `web/src/design/tokens.css` for colours, spacing and type, and check light and dark.
  Do not add CSS frameworks, UI kits, web fonts or anything loaded from a CDN
  ([ADR 0006](docs/adr/0006-bundled-ui-and-design-system.md)).
- Include screenshots in the pull request for visual changes.

### Security-sensitive code

Changes to `internal/server` or to how values are decrypted and returned get extra review. Keep the
guarantees in [ADR 0005](docs/adr/0005-loopback-server-security.md): loopback only, session cookie, host
check, CSRF header, strict CSP, no clear text on disk. New endpoints need tests for those checks.

### Dependencies

The project keeps dependencies few, because it handles secrets. Explain in the pull request why a new
dependency is needed and why the standard library or existing code is not enough.

## AI-assisted contributions

Using coding assistants is fine. In return, you are responsible for every line you submit: read it, run
it and be ready to explain it.

[`AGENTS.md`](AGENTS.md) holds the instructions shared by all assistants (commands, layout and the rules
reviewers check); `CLAUDE.md` imports it for Claude Code. Keep it short and in sync with the docs: when a
pull request changes a command or a convention, update `AGENTS.md` in the same pull request. Tool-specific
files (`.claude/`, `.cursor/`, `.github/copilot-instructions.md`…) are welcome when they help every
contributor; personal settings such as `CLAUDE.local.md` and `.claude/settings.local.json` stay out of git
([ADR 0008](docs/adr/0008-ai-assistant-instructions-in-the-repository.md)).

## Code of conduct

Everyone taking part is expected to follow the [code of conduct](CODE_OF_CONDUCT.md).

## License

By contributing, you agree that your contributions are licensed under the
[Apache License 2.0](LICENSE), the same as the project.
