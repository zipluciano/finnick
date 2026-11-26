import type { Recipient, Rule, SecretFile, Workspace } from "./api";

export const recipientKey = (r: Recipient) => `${r.type}\u0000${r.id}`;

const typeLabels: Record<string, string> = {
  age: "age",
  pgp: "PGP",
  kms: "AWS KMS",
  gcp_kms: "GCP KMS",
  azure_kv: "Azure Key Vault",
  hc_vault: "Vault",
  hckms: "Huawei KMS",
};

export const typeLabel = (t: string) => typeLabels[t] ?? t;

export function shortId(id: string): string {
  if (id.length <= 20) return id;
  return `${id.slice(0, 10)}…${id.slice(-6)}`;
}

export function recipientLabel(r: Recipient): string {
  return r.name ?? shortId(r.id);
}

export function flat(groups: Recipient[][] | null | undefined): Recipient[] {
  return (groups ?? []).flat();
}

/** Groups files by directory: a central vault shows projects, a single repo shows its secret folders. */
export function groupByDir(files: SecretFile[]): { dir: string; files: SecretFile[] }[] {
  const groups = new Map<string, SecretFile[]>();
  for (const f of files) {
    const i = f.path.lastIndexOf("/");
    const dir = i < 0 ? "" : f.path.slice(0, i);
    groups.set(dir, [...(groups.get(dir) ?? []), f]);
  }
  return [...groups.entries()].map(([dir, files]) => ({ dir, files }));
}

export const basename = (p: string) => p.slice(p.lastIndexOf("/") + 1);
export const rootName = (root: string) => root.split(/[\\/]/).filter(Boolean).pop() ?? root;

export type Member = Recipient & { files: Set<string>; rules: Set<number> };

/** Everyone who appears in .sops.yaml rules or in a file's metadata. */
export function members(ws: Workspace): Member[] {
  const byKey = new Map<string, Member>();
  const get = (r: Recipient) => {
    const k = recipientKey(r);
    let m = byKey.get(k);
    if (!m) {
      m = { ...r, files: new Set(), rules: new Set() };
      byKey.set(k, m);
    }
    if (!m.name && r.name) m.name = r.name;
    return m;
  };
  for (const r of ws.config?.keys ?? []) get(r);
  for (const rule of ws.config?.rules ?? []) for (const r of flat(rule.keyGroups)) get(r).rules.add(rule.index);
  for (const f of ws.files) for (const r of flat(f.keyGroups)) get(r).files.add(f.path);
  return [...byKey.values()].sort((a, b) => recipientLabel(a).localeCompare(recipientLabel(b)));
}

/** The page of one person (or key). */
export const personHref = (r: Recipient) => `#/person/${encodeURIComponent(`${r.type}:${r.id}`)}`;

/** Whether a person is named or used in .sops.yaml, so Finnick can change their access there. */
export function inConfig(ws: Workspace, r: Recipient): boolean {
  const k = recipientKey(r);
  return (
    (ws.config?.keys ?? []).some((x) => recipientKey(x) === k) ||
    (ws.config?.rules ?? []).some((rule) => flat(rule.keyGroups).some((x) => recipientKey(x) === k))
  );
}

/** Rules with several key groups (Shamir) are changed by hand: Finnick would not know which group. */
export const editableRule = (rule: Rule) => (rule.keyGroups?.length ?? 0) <= 1;

/** A name SOPS configs can use as a YAML anchor, as the server checks it. */
export const validName = (name: string) => /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(name);

/** A plausible age public key; the server has the final word. */
export const validAgeKey = (key: string) => /^age1[02-9ac-hj-np-z]{20,}$/.test(key);

export type AccessState = "granted" | "pending" | "revoking" | "none";

/** What a recipient's access to a file is now, and what it becomes after `sops updatekeys`. */
export function access(ws: Workspace, f: SecretFile, r: Recipient): AccessState {
  const k = recipientKey(r);
  const inFile = flat(f.keyGroups).some((x) => recipientKey(x) === k);
  const rule = f.rule >= 0 ? ws.config?.rules?.[f.rule] : undefined;
  const inRule = rule ? flat(rule.keyGroups).some((x) => recipientKey(x) === k) : inFile;
  if (inFile && inRule) return "granted";
  if (inFile) return "revoking";
  if (inRule) return "pending";
  return "none";
}

export function isMe(ws: Workspace, r: Recipient): boolean {
  return r.type === "age" && (ws.identity.recipients ?? []).includes(r.id);
}

/** The SOPS format a path gets from its extension, or null for formats Finnick does not create. */
export function formatForPath(path: string): string | null {
  if (/\.ya?ml$/.test(path)) return "yaml";
  if (/\.json$/.test(path)) return "json";
  if (/\.env$/.test(path)) return "dotenv";
  if (/\.ini$/.test(path)) return "ini";
  return null;
}

/** The path of a new key typed by a person: nested formats use parent.name, as the table shows keys. */
export function newKeyPath(format: string, name: string): string[] {
  return format === "dotenv" ? [name.trim()] : name.trim().split(".");
}

/**
 * The position of the rule SOPS would pick for path among rules, as a preview: Go and JavaScript regular
 * expressions mostly agree, and the server has the final word. -1 means no rule matches, undefined
 * means "cannot tell" (a regex JavaScript does not understand comes first).
 */
export function matchRule(rules: { pathRegex: string }[], path: string): number | undefined {
  for (const [i, rule] of rules.entries()) {
    if (!rule.pathRegex) return i;
    try {
      if (new RegExp(rule.pathRegex).test(path)) return i;
    } catch {
      return undefined;
    }
  }
  return -1;
}

/** The creation rule SOPS would pick for path; null when none matches, undefined when it cannot tell. */
export function ruleForPath(ws: Workspace, path: string): Rule | null | undefined {
  const rules = ws.config?.rules ?? [];
  const i = matchRule(rules, path);
  return i === undefined ? undefined : i < 0 ? null : rules[i];
}

/** The age keys a rule can be given: everyone Finnick knows about, and your own key. */
export function agePeople(ws: Workspace): Recipient[] {
  const people: Recipient[] = members(ws).filter((m) => m.type === "age");
  for (const id of ws.identity.recipients ?? []) {
    if (!people.some((p) => p.id === id)) people.push({ type: "age", id });
  }
  return people;
}

/** Whether two lists name the same recipients. */
export function sameRecipients(a: Recipient[], b: Recipient[]): boolean {
  const ka = new Set(a.map(recipientKey));
  const kb = new Set(b.map(recipientKey));
  return ka.size === kb.size && [...ka].every((k) => kb.has(k));
}
