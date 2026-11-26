export type Recipient = {
  type: string; // age, pgp, kms, gcp_kms, azure_kv, hc_vault, hckms
  id: string;
  name?: string;
};

export type Rule = {
  index: number;
  pathRegex: string;
  keyGroups: Recipient[][] | null;
};

export type Config = { path: string; rules: Rule[] | null };

export type Identity = { source?: string; recipients: string[] | null; error?: string };

export type Drift = { missing: Recipient[]; extra: Recipient[] };

export type SecretFile = {
  path: string;
  format: string;
  lastModified: string;
  sopsVersion: string;
  keyGroups: Recipient[][] | null;
  keys: string[];
  rule: number;
  drift?: Drift;
  canDecrypt: boolean;
};

export type Workspace = {
  version: string;
  root: string;
  config: Config | null;
  identity: Identity;
  files: SecretFile[];
};

/** One value of a decrypted file. `path` addresses it in edits: map keys are strings, list positions numbers. */
export type Entry = { key: string; path: (string | number)[]; value: string; type: string };

/** A decrypted file. `version` must be sent back with edits, so a write never overwrites unseen changes. */
export type Revealed = { path: string; entries: Entry[]; version: string; editable: boolean };

export type Edit =
  | { op: "set"; path: (string | number)[]; value: string }
  | { op: "add"; path: string[]; value: string }
  | { op: "delete"; path: (string | number)[] }
  | { op: "rename"; path: (string | number)[]; to: string };

export type Theme = "system" | "light" | "dark";
export type Settings = { theme: Theme; locale: string };

/** An error from the API. `code` identifies errors the UI knows how to translate. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly code?: string,
  ) {
    super(message);
  }
}

const writeHeaders = { "Content-Type": "application/json", "X-Finnick": "1" };

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { credentials: "same-origin", ...init });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(body.error ?? `${res.status} ${res.statusText}`, body.code);
  return body as T;
}

export const api = {
  workspace: () => call<Workspace>("/api/workspace"),
  reveal: (path: string) =>
    call<Revealed>("/api/reveal", { method: "POST", headers: writeHeaders, body: JSON.stringify({ path }) }),
  create: (path: string, edits: Edit[]) =>
    call<Revealed>("/api/create", { method: "POST", headers: writeHeaders, body: JSON.stringify({ path, edits }) }),
  edit: (path: string, version: string, edits: Edit[]) =>
    call<Revealed>("/api/edit", { method: "POST", headers: writeHeaders, body: JSON.stringify({ path, version, edits }) }),
  settings: () => call<Settings>("/api/settings"),
  saveSettings: (s: Settings) =>
    call<Settings>("/api/settings", { method: "PUT", headers: writeHeaders, body: JSON.stringify(s) }),
};
