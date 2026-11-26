import { useState, type FormEvent } from "react";
import { api, type Workspace } from "../api";
import { Icon } from "../components/Icon";
import { Banner, RecipientChip } from "../components/ui";
import { flat, formatForPath, isMe, newKeyPath, recipientKey, ruleForPath } from "../model";
import { usePrefs } from "../prefs";
import { useKeyPlaceholder, ValueField } from "./FileView";

/** Creates an encrypted file, for the recipients of the rule that matches its path. */
export function NewFileView({ ws, onCreated }: { ws: Workspace; onCreated: () => Promise<void> }) {
  const { t, tn, errorText } = usePrefs();
  const firstDir = ws.files[0]?.path.includes("/") ? ws.files[0].path.slice(0, ws.files[0].path.lastIndexOf("/") + 1) : "";
  const [path, setPath] = useState(firstDir || "secrets/");
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const clean = path.trim().replace(/^\.?\/+/, "");
  const format = formatForPath(clean);
  const exists = ws.files.some((f) => f.path === clean);
  const rule = format ? ruleForPath(ws, clean) : undefined;
  const placeholder = useKeyPlaceholder(format);
  const ready = !!format && !exists && rule !== null && name.trim() !== "" && !saving;

  const submit = async (e?: FormEvent) => {
    e?.preventDefault();
    if (!ready || !format) return;
    setSaving(true);
    setError(null);
    try {
      await api.create(clean, [{ op: "add", path: newKeyPath(format, name), value }]);
      await onCreated();
      location.hash = `#/file/${encodeURIComponent(clean)}`;
    } catch (err) {
      setError(err);
    } finally {
      setSaving(false);
    }
  };

  if (!ws.config) {
    return (
      <div className="page">
        <header className="page__header">
          <h1>{t("newFile.title")}</h1>
        </header>
        <Banner tone="warning" icon="alert" title={t("rules.noConfigTitle")}>
          {t("newFile.noConfig")}
        </Banner>
      </div>
    );
  }

  return (
    <div className="page">
      <header className="page__header">
        <div className="stack stack--sm">
          <h1>{t("newFile.title")}</h1>
          <p className="muted">
            {tn("newFile.subtitle", { config: <code>.sops.yaml</code>, command: <code>sops encrypt</code> })}
          </p>
        </div>
      </header>

      <form className="stack stack--lg" onSubmit={submit}>
        {error != null && (
          <Banner tone="danger" icon="alert" title={t("newFile.createError")}>
            {errorText(error)}
          </Banner>
        )}

        <div className="card">
          <div className="card__body stack">
            <label className="field">
              <span className="field__label">{t("newFile.path")}</span>
              <input
                className="input input--mono"
                value={path}
                onChange={(e) => setPath(e.target.value)}
                spellCheck={false}
                autoComplete="off"
                autoFocus
              />
              <span className="xsmall subtle">{tn("newFile.pathHint", { root: <code>{ws.root}</code> })}</span>
            </label>

            {clean && !clean.endsWith("/") && !format && <p className="small muted">{t("newFile.badFormat")}</p>}
            {exists && (
              <p className="small">
                <a href={`#/file/${encodeURIComponent(clean)}`}>{t("newFile.exists")}</a>
              </p>
            )}
            {format && !exists && rule === null && (
              <Banner tone="warning" icon="alert" title={t("newFile.noRule")} />
            )}
            {format && !exists && rule && (
              <div className="stack stack--sm">
                <span className="small muted">
                  {t("file.rule", { n: rule.index + 1 })} <code>{rule.pathRegex || t("access.matchesAll")}</code>
                </span>
                <div className="row row--wrap">
                  {flat(rule.keyGroups).map((r) => (
                    <RecipientChip key={recipientKey(r)} r={r} me={isMe(ws, r)} />
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>

        <div className="card">
          <div className="card__header">
            <h3>{t("newFile.firstKey")}</h3>
          </div>
          <div className="card__body field-row">
            <label className="field field--key">
              <span className="field__label">{t("secrets.key")}</span>
              <input
                className="input input--mono"
                value={name}
                placeholder={placeholder}
                onChange={(e) => setName(e.target.value)}
                spellCheck={false}
                autoComplete="off"
              />
            </label>
            <label className="field grow">
              <span className="field__label">{t("secrets.value")}</span>
              <ValueField
                value={value}
                onChange={setValue}
                label={t("secrets.value")}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) submit();
                }}
              />
            </label>
          </div>
        </div>

        <div className="row">
          <button type="submit" className="btn btn--primary" disabled={!ready}>
            {saving ? <span className="spinner" /> : <Icon name="plus" size={14} />} {t("newFile.create")}
          </button>
          <button type="button" className="btn" onClick={() => history.back()}>
            {t("secrets.cancel")}
          </button>
        </div>
      </form>
    </div>
  );
}
