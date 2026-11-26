# 0007. Preferences on disk and translations checked by the compiler

- Status: Accepted
- Date: 2026-11-25

## Context

The UI offers a light, dark or system theme and is translated (English, Brazilian Portuguese and
Spanish to start). Browser storage is tied to the origin, and the origin changes with every random port,
so `localStorage` would forget the user's choices on each run.

Translations rot when a new string is added in one language only.

## Decision

- Preferences live in `settings.json` under the user's config directory
  (`~/.config/finnick/settings.json`, or `$XDG_CONFIG_HOME`). The server validates them and writes
  `lang` and `data-theme` into `index.html` before sending it, so the first paint already uses them.
- `web/src/i18n/en.ts` is the source of truth. Other locales are typed as `Messages`, so a missing or
  extra key fails `tsc`. Plurals use `Intl.PluralRules` with `_one`/`_other` keys, dates and numbers use
  the selected locale.
- The API returns error codes, not sentences; the UI translates them.

## Consequences

- Adding a locale touches `web/src/i18n/`, `web/src/prefs.tsx` and `internal/settings`. The README
  describes the steps.
- Preferences are per user, not per repository.
