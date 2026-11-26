# Roadmap

## v0.1: read-only (done)

- [x] Find `.sops.yaml` like sops; work without one
- [x] Discover SOPS files in any format; key names without decrypting
- [x] Rule matching, recipient names from YAML anchors, drift detection
- [x] Reveal values (MAC verified), copy, auto-hide
- [x] People & keys matrix, rules view
- [x] Loopback server with token/cookie auth, host check, CSRF header, CSP
- [x] Bundled design system (tokens, base, components), light and dark
- [x] Opens the repository it runs in (`.sops.yaml` lookup like sops, git root fallback)
- [x] Light/dark/system theme and i18n (en, pt-BR, es), saved in `~/.config/finnick/settings.json`

## v0.2: editing

- [ ] Edit, add, rename and delete keys in a file (re-encrypt with the existing data key)
- [ ] Create a file for a new environment/project; recipients come from the matching rule
- [ ] Run `updatekeys` for one file or all drifted files
- [ ] Rotate the data key (`sops rotate`)

## v0.3: people and rules

- [ ] Add a person: paste an age public key, give it a name (anchor), choose rules
- [ ] Remove a person, with a guided rotation checklist for what they could read
- [ ] Create/reorder rules with a live preview of which files each rule matches
- [ ] "Request access" flow: a new member generates a key in the UI and opens a PR adding it

## v0.4: git

- [ ] Status, pull, commit and push from the UI
- [ ] Per-key three-way merge for SOPS files (also as a git merge driver)
- [ ] History per key: who changed `DATABASE_URL` and when (values only for those who can decrypt)

## Later

- [ ] KMS recipients (GCP, AWS, Azure) in the people view, with cloud auth status
- [ ] Push secrets to GCP Secret Manager / AWS Secrets Manager / GitHub Actions secrets
- [ ] Multiple `.sops.yaml` files in one repo (per-directory configs)
- [ ] Shamir key groups in the access view
- [ ] Desktop app wrapper (Wails) for people who prefer not to use the terminal
- [ ] Release binaries (goreleaser), Homebrew tap, `go install`
