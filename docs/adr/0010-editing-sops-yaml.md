# 0010. Editing .sops.yaml

- Status: Accepted
- Date: 2026-11-26

## Context

`.sops.yaml` is written by hand. Teams keep comments in it, blank lines between groups, YAML anchors that
give keys names (`- &alice age1…`), and their own choice of list style (`age: [*a, *b]`, one key per line,
or a comma-separated string). It decides who can decrypt every file created or updated after it, so a
change that misreads it, or that loses a recipient, is a security problem; a change that reformats the
whole file makes every review noisy.

Re-encoding the file with a YAML library keeps the data but not the text: yaml.v3 drops blank lines and
reflows folded strings, and editing the goccy/go-yaml AST loses comments next to the items it touches.

## Decision

- **Text edits, guided by a parse.** Finnick parses the file with yaml.v3 only to find where things are,
  then changes the lines that hold them: it appends an item after the last one of a list, removes an item
  with the comment lines right above it, or rewrites a one-line value. Everything else stays byte for byte,
  including line endings.
- **Keep each list's style.** A block list gets a new line, a flow list (`[*a, *b]`) gets a new item, a
  comma-separated string gets the key appended to it. A lone alias (`age: *a`) becomes a flow list.
- **Rules move as blocks.** Adding a rule inserts it at the chosen position; moving or deleting one takes
  its comment lines along, and moving it back restores the file. A repository without `.sops.yaml` gets
  one, at its root, when its first rule is added. A `path_regex` must compile as RE2, like in SOPS.
- **Names are anchors.** Adding a person adds `- &name age1…` to the top-level `keys` list, creating it above
  `creation_rules` if needed, and rules refer to it as `*name`. Removing a person takes the key out of every
  rule and drops its entry from `keys`.
- **Read back before writing.** After the edits, the new text is parsed again and must mean exactly what the
  operations asked for: same rules and regexes, the recipients of each rule changed only as requested, and
  the same names. If anything else differs, or the text no longer parses, nothing is written and the UI
  asks for the change to be made by hand. A rule or key group is never left without recipients.
- **Preview, then save once.** Rule changes are previewed in the UI before anything is written: which files
  each rule will cover and which will need their keys updated. They are saved together, as one change.
- **Same safety as file writes** ([ADR 0009](0009-writing-sops-files.md)): every change carries the version
  of the file it was based on and is refused if the file changed since; changes are all or nothing and
  replace the file atomically; symlinks are refused.
- **Only what Finnick can show.** It changes age recipients, in flat rules or rules with a single key group.
  Rules with several key groups (Shamir), other key types, and files written in flow style are left to be
  edited by hand.
- **Removing someone is not done until the files are.** Changing `.sops.yaml` gives or takes away nothing by
  itself. The person's page lists every file they can still decrypt and, for each, updates its keys and
  rotates its data key; it also reminds that values they could read must be changed where they are issued.

## Consequences

- Reviews of `.sops.yaml` changes made in Finnick show only the lines that changed.
- Unusual layouts make Finnick refuse a change instead of guessing; the error says so.
- Tests compare edited files with the exact text expected, and a test adds and removes people and checks the
  result with the real `sops` binary.
- Changes to how `.sops.yaml` is edited update this ADR.
