import type { Recipient, SecretFile, Workspace } from "./api";

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
  for (const rule of ws.config?.rules ?? []) for (const r of flat(rule.keyGroups)) get(r).rules.add(rule.index);
  for (const f of ws.files) for (const r of flat(f.keyGroups)) get(r).files.add(f.path);
  return [...byKey.values()].sort((a, b) => recipientLabel(a).localeCompare(recipientLabel(b)));
}

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
