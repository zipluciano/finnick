# 0006. UI embedded in the binary, with its own design system

- Status: Accepted
- Date: 2026-11-25

## Context

The UI must work offline, behind corporate proxies and under a CSP with no external origins (see
[0005](0005-loopback-server-security.md)). Loading a design system, fonts or icons from a CDN would break
all three and leak usage to a third party. Large component libraries also bring many dependencies into a
tool that handles secrets.

## Decision

- The UI is React and TypeScript, built with Vite, and embedded in the Go binary with `go:embed`. One
  file to install, no runtime download.
- Styling is a small design system in `web/src/design/`: `tokens.css` (colour, spacing and type tokens,
  light and dark through `light-dark()`), `base.css` and `components.css`. No CSS framework, no UI kit,
  no web fonts: system font stacks and inline SVG icons.
- Runtime dependencies stay limited to React. Adding one needs a reason in the pull request.

## Consequences

- `make build` needs Node to build the UI before compiling Go. Release binaries ship the UI built.
- Components are written here, so they look consistent but take more work than importing a kit.
- New colours and sizes go into `tokens.css`, not into components, so both themes stay correct.
