import { useEffect, useState } from "react";
import { api, ApiError, type ConfigOp, type Rule, type SecretFile, type Workspace } from "../api";
import { Icon } from "../components/Icon";
import { SyncAll } from "../components/SyncAll";
import { Avatar, Banner, Command, CopyButton, Empty, useAction, YouBadge } from "../components/ui";
import {
  access,
  editableRule,
  flat,
  inConfig,
  isMe,
  members,
  recipientKey,
  recipientLabel,
  typeLabel,
  validName,
  type Member,
} from "../model";
import { usePrefs } from "../prefs";
import { AccessBadge } from "./FileView";

/** One person (or key): which rules include them, what they can decrypt, and how to take access away. */
export function PersonView({ ws, id, onChanged }: { ws: Workspace; id: string; onChanged: () => Promise<void> }) {
  const { t } = usePrefs();
  const found = members(ws).find((m) => `${m.type}:${m.id}` === id);
  // Files this person had to lose access to in this visit, kept to show the checklist to its end.
  const [revoked, setRevoked] = useState<SecretFile[]>([]);
  // Removing someone drops their name from .sops.yaml; keep calling them by it while access is cleaned up.
  const [name, setName] = useState(found?.name);
  useEffect(() => {
    if (found?.name) setName(found.name);
  }, [found?.name]);
  const person = found && { ...found, name: found.name ?? name };

  if (!person) {
    return (
      <div className="page">
        <Empty
          icon={revoked.length > 0 ? "check" : "users"}
          title={revoked.length > 0 ? t("person.revokedAll", { name: name ?? "" }) : t("person.notFound")}
        >
          {revoked.length > 0 && (
            <p>
              <ChangeValues files={revoked} />
            </p>
          )}
          <p>
            <a className="btn" href="#/people">
              {t("people.title")}
            </a>
          </p>
        </Empty>
      </div>
    );
  }

  const files = ws.files.filter((f) => access(ws, f, person) !== "none");
  const revoking = files.filter((f) => access(ws, f, person) === "revoking");
  const age = person.type === "age";
  const named = !!person.name && (ws.config?.keys ?? []).some((k) => k.id === person.id);

  return (
    <div className="page">
      <header className="page__header">
        <div className="row grow">
          <Avatar r={person} />
          <div className="stack stack--sm grow">
            <div className="row">
              <h1 className="truncate">{recipientLabel(person)}</h1>
              {isMe(ws, person) && <YouBadge />}
              <span className="badge">{typeLabel(person.type)}</span>
            </div>
            <div className="row xsmall muted">
              <span className="mono truncate">{person.id}</span>
              <CopyButton text={person.id} label={t("common.copyPublicKey")} />
            </div>
          </div>
        </div>
      </header>

      <div className="stack stack--lg">
        {ws.config && !age && (
          <Banner icon="info" title={t("person.notAge", { type: typeLabel(person.type) })} />
        )}
        {ws.config && age && !named && inConfig(ws, person) && <NameCard ws={ws} person={person} onChanged={onChanged} />}
        {revoking.length > 0 && (
          <RevokeCard
            person={person}
            files={revoking}
            onRevoked={(f) => setRevoked((prev) => [...prev, f])}
            onChanged={onChanged}
          />
        )}
        <SyncAll ws={ws} onChanged={onChanged} exclude={revoking.map((f) => f.path)} />
        {ws.config && age && <RulesCard ws={ws} person={person} onChanged={onChanged} />}
        <FilesCard ws={ws} person={person} files={files} />
        {ws.config && age && inConfig(ws, person) && <RemoveCard ws={ws} person={person} onChanged={onChanged} />}
      </div>
    </div>
  );
}

/** Sends ops to .sops.yaml; on a conflict the workspace is reloaded so the next try sees the change. */
function useConfigChange(ws: Workspace, onChanged: () => Promise<void>) {
  const action = useAction();
  const apply = (ops: ConfigOp[]) =>
    action.run(async () => {
      try {
        await api.config(ws.config!.version, ops);
      } finally {
        await onChanged();
      }
    });
  return { ...action, apply };
}

function ConfigError({ error }: { error: unknown }) {
  const { t, errorText } = usePrefs();
  if (error == null) return null;
  const stale = error instanceof ApiError && error.code === "stale";
  return (
    <Banner tone="danger" icon="alert" title={t("person.saveError")}>
      {stale ? t("person.stale") : errorText(error)}
    </Banner>
  );
}

function NameCard({ ws, person, onChanged }: { ws: Workspace; person: Member; onChanged: () => Promise<void> }) {
  const { t } = usePrefs();
  const [name, setName] = useState("");
  const { busy, error, apply } = useConfigChange(ws, onChanged);
  const taken = (ws.config?.keys ?? []).some((k) => k.name === name.trim());
  const ready = validName(name.trim()) && !taken && !busy;
  return (
    <form
      className="card"
      onSubmit={(e) => {
        e.preventDefault();
        if (ready) apply([{ op: "addPerson", name: name.trim(), key: person.id }]);
      }}
    >
      <div className="card__header">
        <h3>{t("person.nameTitle")}</h3>
      </div>
      <div className="card__body stack">
        <p className="small muted">{t("person.nameBody")}</p>
        <ConfigError error={error} />
        <div className="row row--wrap">
          <input
            className="input field--key"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="alice"
            aria-label={t("people.name")}
            spellCheck={false}
            autoComplete="off"
          />
          <button type="submit" className="btn btn--primary" disabled={!ready}>
            {busy && <span className="spinner" />} {t("person.saveName")}
          </button>
          {name.trim() && !validName(name.trim()) && <span className="xsmall subtle">{t("errors.invalid_name")}</span>}
          {taken && <span className="xsmall subtle">{t("errors.name_taken")}</span>}
        </div>
      </div>
    </form>
  );
}

/** Checkboxes for the creation rules, with what each one covers. */
export function RuleChoices({
  ws,
  checked,
  onToggle,
  disabled,
}: {
  ws: Workspace;
  checked: Set<number>;
  onToggle: (index: number) => void;
  disabled: (rule: Rule) => boolean;
}) {
  const { t, tp } = usePrefs();
  const rules = ws.config?.rules ?? [];
  if (rules.length === 0) return <p className="small muted">{t("rules.emptyTitle")}</p>;
  return (
    <div className="choices">
      {rules.map((rule) => {
        const off = disabled(rule);
        const files = ws.files.filter((f) => f.rule === rule.index).length;
        return (
          <label key={rule.index} className={off ? "choice choice--disabled" : "choice"}>
            <input type="checkbox" checked={checked.has(rule.index)} disabled={off} onChange={() => onToggle(rule.index)} />
            <span className="badge">#{rule.index + 1}</span>
            <code className="truncate grow">{rule.pathRegex || t("rules.noRegex")}</code>
            <span className="xsmall subtle">{off ? t("person.shamirRule") : tp("rules.files", files)}</span>
          </label>
        );
      })}
    </div>
  );
}

function RulesCard({ ws, person, onChanged }: { ws: Workspace; person: Member; onChanged: () => Promise<void> }) {
  const { t, tn } = usePrefs();
  const rules = ws.config?.rules ?? [];
  const current = new Set(
    rules.filter((r) => flat(r.keyGroups).some((x) => recipientKey(x) === recipientKey(person))).map((r) => r.index),
  );
  const [draft, setDraft] = useState<Set<number> | null>(null);
  const { busy, error, apply } = useConfigChange(ws, onChanged);
  const chosen = draft ?? current;
  const ops: ConfigOp[] = [];
  for (const r of rules) {
    if (chosen.has(r.index) && !current.has(r.index)) ops.push({ op: "grant", rule: r.index, key: person.id });
    if (!chosen.has(r.index) && current.has(r.index)) ops.push({ op: "revoke", rule: r.index, key: person.id });
  }

  return (
    <div className="card">
      <div className="card__header">
        <h3>{t("person.rulesTitle")}</h3>
      </div>
      <div className="card__body stack">
        <p className="small muted">{tn("person.rulesBody", { config: <code>.sops.yaml</code> })}</p>
        <ConfigError error={error} />
        <RuleChoices
          ws={ws}
          checked={chosen}
          disabled={(rule) => !editableRule(rule) || busy}
          onToggle={(i) => {
            const next = new Set(chosen);
            if (next.has(i)) next.delete(i);
            else next.add(i);
            setDraft(next);
          }}
        />
        {ops.length > 0 && (
          <div className="row">
            <button
              type="button"
              className="btn btn--primary"
              disabled={busy}
              onClick={() => apply(ops).then((ok) => ok && setDraft(null))}
            >
              {busy && <span className="spinner" />} {t("person.saveRules")}
            </button>
            <button type="button" className="btn" onClick={() => setDraft(null)} disabled={busy}>
              {t("secrets.cancel")}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

function FilesCard({ ws, person, files }: { ws: Workspace; person: Member; files: SecretFile[] }) {
  const { t } = usePrefs();
  return (
    <div className="card">
      <div className="card__header">
        <h3>{t("person.filesTitle")}</h3>
      </div>
      <div className="card__body">
        {files.length === 0 ? (
          <p className="small muted">{t("person.noFiles")}</p>
        ) : (
          <div className="steps">
            {files.map((f) => (
              <div key={f.path} className="step">
                <a className="mono grow truncate" href={`#/file/${encodeURIComponent(f.path)}`}>
                  {f.path}
                </a>
                <AccessBadge state={access(ws, f, person)} />
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

/** The rest of taking access away: each file is updated (they leave it) and rotated (they knew its key). */
function RevokeCard({
  person,
  files,
  onRevoked,
  onChanged,
}: {
  person: Member;
  files: SecretFile[];
  onRevoked: (f: SecretFile) => void;
  onChanged: () => Promise<void>;
}) {
  const { t, tp, errorText } = usePrefs();
  const [working, setWorking] = useState<string | null>(null);
  const [failed, setFailed] = useState<Record<string, unknown>>({});
  const mine = files.filter((f) => f.canDecrypt);
  const locked = files.filter((f) => !f.canDecrypt);

  const revoke = async (list: SecretFile[]) => {
    const errors: Record<string, unknown> = {};
    for (const f of list) {
      setWorking(f.path);
      try {
        await api.updateKeys(f.path);
        await api.rotate(f.path);
        onRevoked(f);
      } catch (e) {
        errors[f.path] = e;
      }
    }
    setWorking(null);
    setFailed(errors);
    await onChanged();
  };

  return (
    <Banner tone="warning" icon="alert" title={tp("person.revokeTitle", files.length, { name: recipientLabel(person) })}>
      <span>{t("person.revokeBody", { name: recipientLabel(person) })}</span>
      {mine.length > 1 && (
        <div>
          <button type="button" className="btn btn--primary" onClick={() => revoke(mine)} disabled={working !== null}>
            {working !== null ? <span className="spinner" /> : <Icon name="refresh" size={14} />}{" "}
            {tp("person.revokeAll", mine.length)}
          </button>
        </div>
      )}
      <div className="steps">
        {files.map((f) => (
          <div key={f.path} className="step">
            <a className="mono grow truncate" href={`#/file/${encodeURIComponent(f.path)}`}>
              {f.path}
            </a>
            {f.canDecrypt ? (
              <button type="button" className="btn btn--sm" onClick={() => revoke([f])} disabled={working !== null}>
                {working === f.path ? <span className="spinner" /> : <Icon name="refresh" size={13} />}{" "}
                {t("person.revokeFile")}
              </button>
            ) : (
              <span className="xsmall muted">{t("person.revokeLocked")}</span>
            )}
            {failed[f.path] != null && <span className="xsmall">{errorText(failed[f.path])}</span>}
          </div>
        ))}
      </div>
      {locked.length > 0 && (
        <Command>{locked.map((f) => `sops updatekeys -y ${f.path} && sops rotate -i ${f.path}`).join(" && ")}</Command>
      )}
      <ChangeValues files={files} />
    </Banner>
  );
}

/** Rotation protects the files; values someone could read are only safe once changed at their source. */
function ChangeValues({ files }: { files: SecretFile[] }) {
  const { tn } = usePrefs();
  const keys = [...new Set(files.flatMap((f) => f.keys))];
  if (keys.length === 0) return null;
  const shown = keys.slice(0, 12);
  return (
    <span className="small">
      {tn("person.changeValues", {
        keys: (
          <>
            {shown.map((k, i) => (
              <span key={k}>
                {i > 0 && ", "}
                <code>{k}</code>
              </span>
            ))}
            {keys.length > shown.length && " …"}
          </>
        ),
      })}
    </span>
  );
}

function RemoveCard({ ws, person, onChanged }: { ws: Workspace; person: Member; onChanged: () => Promise<void> }) {
  const { t, tn } = usePrefs();
  const [confirming, setConfirming] = useState(false);
  const { busy, error, apply } = useConfigChange(ws, onChanged);
  const shamir = (ws.config?.rules ?? []).some(
    (r) => !editableRule(r) && flat(r.keyGroups).some((x) => recipientKey(x) === recipientKey(person)),
  );
  return (
    <div className="card">
      <div className="card__header">
        <h3>{t("person.removeTitle")}</h3>
        <button type="button" className="btn btn--danger" onClick={() => setConfirming(true)} disabled={confirming || shamir}>
          <Icon name="trash" size={14} /> {t("person.remove", { name: recipientLabel(person) })}
        </button>
      </div>
      <div className="card__body stack stack--sm">
        <p className="small muted">{tn("person.removeBody", { config: <code>.sops.yaml</code> })}</p>
        {shamir && <p className="small muted">{t("errors.key_groups")}</p>}
        <ConfigError error={error} />
        {confirming && (
          <div className="row row--wrap">
            <strong className="small">{t("person.confirmRemove")}</strong>
            <button
              type="button"
              className="btn btn--sm btn--danger"
              disabled={busy}
              onClick={() => apply([{ op: "removePerson", key: person.id }]).then(() => setConfirming(false))}
            >
              {busy ? <span className="spinner" /> : <Icon name="trash" size={13} />} {t("person.removeAction")}
            </button>
            <button type="button" className="btn btn--sm" onClick={() => setConfirming(false)} disabled={busy}>
              {t("secrets.cancel")}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
