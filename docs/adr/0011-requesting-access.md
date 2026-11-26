# 0011. Requesting access: creating keys and proposing changes with git

- Status: Accepted
- Date: 2026-11-26

## Context

Joining a repository that uses SOPS takes several steps a newcomer gets wrong easily: creating an age key in
the place SOPS looks for it, sending only the public part, and getting it into `.sops.yaml`, which they
cannot do alone because the files must then be re-encrypted by someone who can decrypt them. Two parts of
this touch sensitive things: a private key is written to disk, and a change is pushed to a shared remote.

## Decision

- **Create the key like age-keygen, where SOPS reads it.** Finnick writes a new X25519 key to
  `SOPS_AGE_KEY_FILE` or SOPS' default file, in age-keygen's format, with mode `0600` in a `0700` folder.
  It never replaces a file that exists and refuses when `SOPS_AGE_KEY` or `SOPS_AGE_KEY_CMD` provide a key.
  The private key is never returned by the API, shown or logged; the page asks for a backup.
- **Propose, do not commit locally.** The request is a branch on `origin`, built on its default branch
  (`origin/HEAD`, fetched first). The `.sops.yaml` change is made with the same editor and checks as every
  other change ([ADR 0010](0010-editing-sops-yaml.md)), on the file as it is on that branch. The commit is
  created with `git hash-object`, a temporary index and `git commit-tree`, then pushed by hash: the
  working tree, the index, the current branch and local refs are never touched, so work in progress
  (including local edits to `.sops.yaml`) cannot leak into the request.
- **Preview first.** The page shows the exact diff and the branch name before anything is pushed; pushing
  is a separate click.
- **Only for your own key.** The server proposes keys that come from the local identity, never one typed in.
- **Use the user's git, unattended.** Commands run with the user's git configuration and credentials
  (author, signing, SSH agent, credential helpers) and with prompts disabled, so a missing credential is an
  error shown in the page rather than a hung request. A branch name that is taken gets a number.
- **Hosts are only links.** For GitHub, GitLab and Bitbucket remotes the page links to the "new pull/merge
  request" form; Finnick does not call their APIs or need a token. Elsewhere it names the branch to open
  one from.

## Consequences

- The review of the pull request is where the key is checked to really belong to the person: the commit
  message says so, and the change shows up in the People page as soon as it is merged.
- Merging still does not give access: someone who can decrypt runs Update keys, as with any added person.
- Git is needed only for this flow; everything else keeps working without it.
- Tests run the git commands against a local bare remote and check that the clone is left as it was.
