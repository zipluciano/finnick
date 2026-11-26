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

export type Entry = { key: string; value: string };

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
    call<{ path: string; entries: Entry[] }>("/api/reveal", {
      method: "POST",
      headers: writeHeaders,
      body: JSON.stringify({ path }),
    }),
  settings: () => call<Settings>("/api/settings"),
  saveSettings: (s: Settings) =>
    call<Settings>("/api/settings", { method: "PUT", headers: writeHeaders, body: JSON.stringify(s) }),
};
