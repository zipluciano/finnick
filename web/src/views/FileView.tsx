import { useEffect, useMemo, useState, type KeyboardEvent } from "react";
import { api, ApiError, type Edit, type Entry, type Recipient, type Revealed, type SecretFile, type Workspace } from "../api";
import { Icon, type IconName } from "../components/Icon";
import { Banner, Command, CopyButton, Empty, RecipientRow } from "../components/ui";
import { access, flat, isMe, newKeyPath, recipientKey } from "../model";
import { usePrefs } from "../prefs";

const AUTO_HIDE_MS = 2 * 60 * 1000;

export function FileView({ ws, file, onChanged }: { ws: Workspace; file: SecretFile; onChanged: () => void }) {
  const { t, tp, relativeTime } = usePrefs();
  const [tab, setTab] = useState<"secrets" | "access">("secrets");
  const dir = file.path.includes("/") ? file.path.slice(0, file.path.lastIndexOf("/")) : "";
  const name = file.path.slice(dir ? dir.length + 1 : 0);

  return (
    <div className="page">
      <header className="page__header">
        <div className="stack stack--sm grow">
          {dir && (
            <div className="breadcrumb">
              <Icon name="folder" size={14} />
              <span className="truncate">{dir}</span>
            </div>
          )}
          <h1 className="title-mono">{name}</h1>
          <div className="row row--wrap">
            <span className="badge">{file.format}</span>
            <span className="badge">{tp("file.keys", file.keys.length)}</span>
            {file.rule >= 0 ? (
              <a className="badge" href="#/rules" title={t("file.ruleTitle")}>
                {t("file.rule", { n: file.rule + 1 })}
              </a>
            ) : (
              ws.config && <span className="badge badge--warning">{t("file.noRule")}</span>
            )}
            {file.canDecrypt ? (
              <span className="badge badge--success">
                <Icon name="unlock" size={12} /> {t("file.canDecrypt")}
              </span>
            ) : (
              <span className="badge">
                <Icon name="lock" size={12} /> {t("file.locked")}
              </span>
            )}
            <span className="xsmall subtle" title={file.lastModified}>
              {t("file.modified", { when: relativeTime(file.lastModified), version: file.sopsVersion })}
            </span>
          </div>
        </div>
      </header>

      {file.drift && <DriftBanner file={file} onChanged={onChanged} />}

      <div className="tabs" role="tablist">
        <button type="button" role="tab" className="tab" aria-selected={tab === "secrets"} onClick={() => setTab("secrets")}>
          <Icon name="key" size={14} /> {t("tabs.secrets")}
        </button>
        <button type="button" role="tab" className="tab" aria-selected={tab === "access"} onClick={() => setTab("access")}>
          <Icon name="users" size={14} /> {t("tabs.access")}
          {file.drift && <span className="dot dot--warning" />}
        </button>
      </div>

      {tab === "secrets" ? <SecretsTab file={file} onChanged={onChanged} /> : <AccessTab ws={ws} file={file} onChanged={onChanged} />}
    </div>
  );
}

/** Runs an action once at a time and keeps its error. */
function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const run = async (action: () => Promise<unknown>) => {
    setBusy(true);
    setError(null);
    try {
      await action();
      return true;
    } catch (e) {
      setError(e);
      return false;
    } finally {
      setBusy(false);
    }
  };
  return { busy, error, run };
}

function DriftBanner({ file, onChanged }: { file: SecretFile; onChanged: () => void }) {
  const { t, tp, errorText } = usePrefs();
  const { busy, error, run } = useAction();
  const { missing, extra } = file.drift!;
  const names = (rs: Recipient[]) => rs.map((r) => r.name ?? r.type).join(", ");
  const update = () => run(() => api.updateKeys(file.path)).then((ok) => ok && onChanged());
  return (
    <div className="page__section">
      <Banner tone="warning" icon="alert" title={t("drift.title")}>
        <span>
          {missing.length > 0 && tp("drift.missing", missing.length, { names: names(missing) })}{" "}
          {extra.length > 0 && tp("drift.extra", extra.length, { names: names(extra) })}
        </span>
        {file.canDecrypt ? (
          <div className="row row--wrap">
            <button type="button" className="btn btn--primary" onClick={update} disabled={busy}>
              {busy ? <span className="spinner" /> : <Icon name="refresh" size={14} />} {t("drift.update")}
            </button>
            <span className="small">{t("drift.orTerminal")}</span>
          </div>
        ) : (
          <span>{t("drift.needAccess")}</span>
        )}
        <Command>{`sops updatekeys -y ${file.path}`}</Command>
        {error != null && (
          <strong>
            {t("drift.updateError")}: {errorText(error)}
          </strong>
        )}
        {extra.length > 0 && <span>{t("drift.revokeWarning")}</span>}
      </Banner>
    </div>
  );
}

type RowMode = { kind: "edit"; key: string } | { kind: "delete"; key: string } | { kind: "add" } | null;

function SecretsTab({ file, onChanged }: { file: SecretFile; onChanged: () => void }) {
  const { t, errorText } = usePrefs();
  const [data, setData] = useState<Revealed | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<{ title: string; cause: unknown } | null>(null);
  const [filter, setFilter] = useState("");
  const [mode, setMode] = useState<RowMode>(null);

  // Values hide after a while without activity; editing counts as activity.
  useEffect(() => {
    if (!data) return;
    const timer = setTimeout(() => {
      setData(null);
      setMode(null);
    }, AUTO_HIDE_MS);
    return () => clearTimeout(timer);
  }, [data, mode]);

  // Starting another action dismisses the message about the last one.
  useEffect(() => {
    if (mode) setError(null);
  }, [mode]);

  const reveal = async () => {
    setLoading(true);
    setError(null);
    try {
      setData(await api.reveal(file.path));
    } catch (e) {
      setError({ title: t("secrets.decryptError"), cause: e });
    } finally {
      setLoading(false);
    }
  };

  const hide = () => {
    setData(null);
    setMode(null);
  };

  const save = async (edits: Edit[]) => {
    if (!data) return;
    setSaving(true);
    setError(null);
    try {
      setData(await api.edit(file.path, data.version, edits));
      setMode(null);
      onChanged();
    } catch (e) {
      setError({ title: t("secrets.saveError"), cause: e });
      if (e instanceof ApiError && e.code === "stale") {
        // Someone (or git) changed the file: show what is there now, so the change can be redone on top.
        setMode(null);
        api.reveal(file.path).then(setData, () => {});
      }
    } finally {
      setSaving(false);
    }
  };

  const canEdit = !!data?.editable && file.canDecrypt;
  const rows = useMemo(() => {
    const all: { key: string; entry?: Entry }[] = data ? data.entries.map((e) => ({ key: e.key, entry: e })) : file.keys.map((key) => ({ key }));
    const q = filter.trim().toLowerCase();
    return all.filter((r) => r.key.toLowerCase().includes(q));
  }, [data, file.keys, filter]);
  const total = data ? data.entries.length : file.keys.length;

  return (
    <div className="stack">
      {!file.canDecrypt && (
        <Banner tone="info" icon="lock" title={t("secrets.notRecipientTitle")}>
          {t("secrets.notRecipientBody")}
        </Banner>
      )}
      {error && (
        <Banner tone="danger" icon="alert" title={error.title}>
          {errorText(error.cause)}
        </Banner>
      )}

      <div className="card">
        <div className="card__header">
          <div className="input-with-icon">
            <Icon name="search" size={14} />
            <input
              className="input"
              placeholder={t("secrets.filter")}
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              aria-label={t("secrets.filter")}
            />
          </div>
          <div className="row">
            {canEdit && (
              <button type="button" className="btn" onClick={() => setMode({ kind: "add" })} disabled={mode?.kind === "add"}>
                <Icon name="plus" size={14} /> {t("secrets.add")}
              </button>
            )}
            {data ? (
              <button type="button" className="btn" onClick={hide}>
                <Icon name="eyeOff" size={14} /> {t("secrets.hide")}
              </button>
            ) : (
              <button type="button" className="btn btn--primary" onClick={reveal} disabled={!file.canDecrypt || loading}>
                {loading ? <span className="spinner" /> : <Icon name="eye" size={14} />} {t("secrets.reveal")}
              </button>
            )}
          </div>
        </div>
        {rows.length === 0 && mode?.kind !== "add" ? (
          <Empty icon="search" title={total ? t("secrets.noMatch") : t("secrets.noKeys")} />
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th className="table__key">{t("secrets.key")}</th>
                <th>{t("secrets.value")}</th>
                <th className="table__actions" />
              </tr>
            </thead>
            <tbody>
              {rows.map(({ key, entry }) => {
                if (entry && mode?.kind === "edit" && mode.key === key) {
                  return <EditRow key={key} entry={entry} saving={saving} onSave={save} onCancel={() => setMode(null)} />;
                }
                const deleting = entry && mode?.kind === "delete" && mode.key === key;
                return (
                  <tr key={key}>
                    <td className="mono">{key}</td>
                    <td>
                      {deleting ? (
                        <strong>{t("secrets.confirmDelete")}</strong>
                      ) : entry === undefined ? (
                        <span className="secret-value secret-value--masked">••••••••••</span>
                      ) : (
                        <span className="secret-value">
                          {entry.value === "" ? <span className="subtle">{t("secrets.empty")}</span> : entry.value}
                        </span>
                      )}
                    </td>
                    <td className="table__actions">
                      {deleting ? (
                        <div className="row">
                          <button type="button" className="btn btn--sm btn--danger" disabled={saving} onClick={() => save([{ op: "delete", path: entry.path }])}>
                            {saving ? <span className="spinner" /> : <Icon name="trash" size={13} />} {t("secrets.delete")}
                          </button>
                          <button type="button" className="btn btn--sm" disabled={saving} onClick={() => setMode(null)}>
                            {t("secrets.cancel")}
                          </button>
                        </div>
                      ) : (
                        entry && (
                          <div className="row">
                            <CopyButton text={entry.value} label={t("secrets.copy", { key })} />
                            {canEdit && (
                              <>
                                <IconButton icon="pencil" label={t("secrets.editKey", { key })} onClick={() => setMode({ kind: "edit", key })} />
                                <IconButton icon="trash" label={t("secrets.deleteKey", { key })} onClick={() => setMode({ kind: "delete", key })} />
                              </>
                            )}
                          </div>
                        )
                      )}
                    </td>
                  </tr>
                );
              })}
              {mode?.kind === "add" && <AddRow format={file.format} saving={saving} onSave={save} onCancel={() => setMode(null)} />}
            </tbody>
          </table>
        )}
      </div>
      {data && (
        <p className="xsmall subtle">
          {t("secrets.autoHide")} {data.editable ? t("secrets.editHint") : t("secrets.readOnly")}
        </p>
      )}
    </div>
  );
}

function IconButton({ icon, label, onClick }: { icon: IconName; label: string; onClick: () => void }) {
  return (
    <button type="button" className="btn btn--ghost btn--icon" title={label} aria-label={label} onClick={onClick}>
      <Icon name={icon} size={14} />
    </button>
  );
}

/** Escape cancels; Cmd/Ctrl+Enter saves (plain Enter adds a line to the value). */
function editKeys(onSave: () => void, onCancel: () => void) {
  return (e: KeyboardEvent<HTMLElement>) => {
    if (e.key === "Escape") onCancel();
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey || e.currentTarget.tagName === "INPUT")) {
      e.preventDefault();
      onSave();
    }
  };
}

export function ValueField({ value, onChange, onKeyDown, label, autoFocus }: {
  value: string;
  onChange: (v: string) => void;
  onKeyDown: (e: KeyboardEvent<HTMLElement>) => void;
  label: string;
  autoFocus?: boolean;
}) {
  return (
    <textarea
      className="input"
      rows={Math.min(8, value.split("\n").length)}
      value={value}
      spellCheck={false}
      autoComplete="off"
      aria-label={label}
      autoFocus={autoFocus}
      onChange={(e) => onChange(e.target.value)}
      onKeyDown={onKeyDown}
    />
  );
}

function SaveButtons({ saving, onSave, onCancel }: { saving: boolean; onSave: () => void; onCancel: () => void }) {
  const { t } = usePrefs();
  return (
    <div className="row">
      <button type="button" className="btn btn--sm btn--primary" disabled={saving} onClick={onSave}>
        {saving ? <span className="spinner" /> : <Icon name="check" size={13} />} {t("secrets.save")}
      </button>
      <button type="button" className="btn btn--sm" disabled={saving} onClick={onCancel}>
        {t("secrets.cancel")}
      </button>
    </div>
  );
}

function EditRow({ entry, saving, onSave, onCancel }: {
  entry: Entry;
  saving: boolean;
  onSave: (edits: Edit[]) => void;
  onCancel: () => void;
}) {
  const { t } = usePrefs();
  const last = entry.path[entry.path.length - 1];
  const renamable = typeof last === "string";
  const parent = renamable ? entry.key.slice(0, entry.key.length - last.length) : "";
  const [name, setName] = useState(renamable ? last : "");
  const [value, setValue] = useState(entry.value);

  const submit = () => {
    const edits: Edit[] = [];
    if (value !== entry.value) edits.push({ op: "set", path: entry.path, value });
    if (renamable && name.trim() !== last) edits.push({ op: "rename", path: entry.path, to: name.trim() });
    if (edits.length) onSave(edits);
    else onCancel();
  };
  const keys = editKeys(submit, onCancel);

  return (
    <tr>
      <td className="table__editing">
        {renamable ? (
          <div className="row">
            {parent && <span className="mono subtle">{parent}</span>}
            <input className="input input--mono grow" value={name} onChange={(e) => setName(e.target.value)} onKeyDown={keys} aria-label={t("secrets.key")} spellCheck={false} autoComplete="off" />
          </div>
        ) : (
          <span className="mono">{entry.key}</span>
        )}
      </td>
      <td className="table__editing">
        <div className="stack stack--sm">
          <ValueField value={value} onChange={setValue} onKeyDown={keys} label={t("secrets.value")} autoFocus />
          {entry.type !== "string" && <span className="xsmall subtle">{t("secrets.typeHint", { type: entry.type })}</span>}
          <SaveButtons saving={saving} onSave={submit} onCancel={onCancel} />
        </div>
      </td>
      <td className="table__editing" />
    </tr>
  );
}

export function useKeyPlaceholder(format: string | null) {
  const { t } = usePrefs();
  return format === "dotenv" ? "NAME" : format === "ini" ? t("secrets.newKeyIni") : t("secrets.newKeyNested");
}

function AddRow({ format, saving, onSave, onCancel }: {
  format: string;
  saving: boolean;
  onSave: (edits: Edit[]) => void;
  onCancel: () => void;
}) {
  const { t } = usePrefs();
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const path = newKeyPath(format, name);
  const placeholder = useKeyPlaceholder(format);
  const submit = () => {
    if (name.trim()) onSave([{ op: "add", path, value }]);
  };
  const keys = editKeys(submit, onCancel);

  return (
    <tr>
      <td className="table__editing">
        <input className="input input--mono" value={name} placeholder={placeholder} onChange={(e) => setName(e.target.value)} onKeyDown={keys} aria-label={t("secrets.key")} spellCheck={false} autoComplete="off" autoFocus />
      </td>
      <td className="table__editing">
        <div className="stack stack--sm">
          <ValueField value={value} onChange={setValue} onKeyDown={keys} label={t("secrets.value")} />
          <SaveButtons saving={saving} onSave={submit} onCancel={onCancel} />
        </div>
      </td>
      <td className="table__editing" />
    </tr>
  );
}

function AccessTab({ ws, file, onChanged }: { ws: Workspace; file: SecretFile; onChanged: () => void }) {
  const { t, tn } = usePrefs();
  const rule = file.rule >= 0 ? ws.config?.rules?.[file.rule] : undefined;
  const seen = new Set<string>();
  const all: Recipient[] = [];
  for (const r of [...flat(file.keyGroups), ...flat(rule?.keyGroups)]) {
    if (!seen.has(recipientKey(r))) {
      seen.add(recipientKey(r));
      all.push(r);
    }
  }
  const groups = file.keyGroups?.length ?? 0;

  return (
    <div className="stack">
      {groups > 1 && <Banner title={t("access.shamirTitle", { count: groups })}>{t("access.shamirBody")}</Banner>}
      <div className="card">
        <div className="card__header">
          <h3>{t("access.whoCanDecrypt")}</h3>
          {rule && (
            <span className="small muted">
              {t("file.rule", { n: rule.index + 1 })} <code>{rule.pathRegex || t("access.matchesAll")}</code>
            </span>
          )}
        </div>
        <table className="table">
          <thead>
            <tr>
              <th>{t("access.recipient")}</th>
              <th>{t("access.type")}</th>
              <th>{t("access.key")}</th>
              <th className="table__actions">{t("access.status")}</th>
            </tr>
          </thead>
          <tbody>
            {all.map((r) => (
              <RecipientRow key={recipientKey(r)} r={r} me={isMe(ws, r)} status={<AccessBadge state={access(ws, file, r)} />} />
            ))}
          </tbody>
        </table>
      </div>
      <p className="small muted">
        {tn("access.footnote", { config: <code>.sops.yaml</code>, command: <code>sops updatekeys</code> })}
      </p>
      <RotateCard file={file} onChanged={onChanged} />
    </div>
  );
}

function RotateCard({ file, onChanged }: { file: SecretFile; onChanged: () => void }) {
  const { t, tn, errorText } = usePrefs();
  const { busy, error, run } = useAction();
  const [confirming, setConfirming] = useState(false);
  const [done, setDone] = useState(false);
  const rotate = () =>
    run(() => api.rotate(file.path)).then((ok) => {
      setConfirming(false);
      setDone(ok);
      if (ok) onChanged();
    });

  return (
    <div className="card">
      <div className="card__header">
        <h3>{t("rotate.title")}</h3>
        <button
          type="button"
          className="btn"
          onClick={() => setConfirming(true)}
          disabled={!file.canDecrypt || confirming}
        >
          <Icon name="refresh" size={14} /> {t("rotate.action")}
        </button>
      </div>
      <div className="card__body stack stack--sm">
        <p className="small muted">{tn("rotate.body", { command: <code>sops rotate</code> })}</p>
        {confirming && (
          <div className="row row--wrap">
            <strong className="small">{t("rotate.confirm")}</strong>
            <button type="button" className="btn btn--sm btn--danger" onClick={rotate} disabled={busy}>
              {busy ? <span className="spinner" /> : <Icon name="refresh" size={13} />} {t("rotate.confirmAction")}
            </button>
            <button type="button" className="btn btn--sm" onClick={() => setConfirming(false)} disabled={busy}>
              {t("secrets.cancel")}
            </button>
          </div>
        )}
        {!file.canDecrypt && <p className="small muted">{t("rotate.needAccess")}</p>}
        {done && (
          <p className="small">
            <Icon name="check" size={13} /> {t("rotate.done")}
          </p>
        )}
        {error != null && (
          <Banner tone="danger" icon="alert" title={t("rotate.error")}>
            {errorText(error)}
          </Banner>
        )}
      </div>
    </div>
  );
}

export function AccessBadge({ state }: { state: ReturnType<typeof access> }) {
  const { t } = usePrefs();
  switch (state) {
    case "granted":
      return <span className="badge badge--success">{t("access.granted")}</span>;
    case "pending":
      return <span className="badge badge--warning">{t("access.pending")}</span>;
    case "revoking":
      return <span className="badge badge--danger">{t("access.revoking")}</span>;
    default:
      return null;
  }
}
