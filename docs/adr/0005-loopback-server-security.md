# 0005. Security model of the loopback server

- Status: Accepted
- Date: 2026-11-25

## Context

The UI is served over HTTP by a process that holds the user's decryption keys. Anything that can talk to
that port can ask it to decrypt. Threats include other local users and processes, web pages open in the
same browser (CSRF, DNS rebinding) and values left behind in caches or history.

## Decision

- Listen on `127.0.0.1` only, on a random port unless `--port` is given.
- The CLI generates a random token and opens `/auth?token=…`. The server exchanges it for an `HttpOnly`,
  `SameSite=Strict` session cookie and redirects, so the token does not stay in the address bar. Every
  API call needs the cookie.
- Reject requests whose `Host` header is not a loopback name (`127.0.0.1`, `localhost`, `::1`) with
  the port being served (DNS rebinding).
- Calls that reveal values or change state are `POST`/`PUT` and must carry `X-Finnick: 1`, which a
  cross-site form cannot send.
- Strict Content-Security-Policy with no external origins, `Cache-Control: no-store` on every response.
- Decrypt in the Go process and return values only for the file asked for. Never write clear text to
  disk. The UI hides revealed values again after two minutes.

## Consequences

- Opening the UI from another browser or device needs the URL with the token; that is intended.
- Restarting the CLI ends the session (`session_expired` in the UI).
- Any new endpoint must go through the same middleware and get tests for the host check and the CSRF
  header. Changes to this model need a new ADR.
