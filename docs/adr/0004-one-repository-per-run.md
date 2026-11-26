# 0004. One repository per run

- Status: Accepted
- Date: 2026-11-25

## Context

The first prototype was built for a central vault: one git repository holding the secrets of many
projects, in a fixed `projects/<name>/<env>` layout. A CLI tool over SOPS can make that layout
unnecessary. Keeping secrets in the repository of the code that uses them means:

- a change that needs a new variable is one pull request, reviewed with the code;
- cloning the project is enough to run it, given a key in `.sops.yaml`;
- CI reads the secrets from the checkout it already has.

The cost is that adding or removing a person touches every repository. With a handful of repositories
that is acceptable, and SOPS users who prefer a vault can still point Finnick at it.

## Decision

`finnick ui` opens the repository it runs in and nothing else. It looks for `.sops.yaml` like `sops`
does, falls back to the git root, and refuses to start outside a repository. There is no required folder
structure: files are found by their SOPS metadata and grouped by directory. `--dir` opens another
directory.

## Consequences

- It works like Drizzle Studio: a `package.json` script (`"secrets": "finnick ui"`) is all a project
  needs.
- No project registry, no global list of repositories, no cross-repository views.
- A repository with several `.sops.yaml` files is shown through the one closest to the starting
  directory. Supporting several configs in one repository is on the roadmap.
