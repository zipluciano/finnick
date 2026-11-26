import { useState, type FormEvent } from "react";
import { api, ApiError, type ConfigOp, type Workspace } from "../api";
import { Icon } from "../components/Icon";
import { Banner } from "../components/ui";
import { editableRule, members, personHref, validAgeKey, validName } from "../model";
import { usePrefs } from "../prefs";
import { RuleChoices } from "./PersonView";

/** Names a new age key in .sops.yaml and adds it to the chosen rules. */
export function AddPerson({ ws, onDone, onCancel }: { ws: Workspace; onDone: () => Promise<void>; onCancel: () => void }) {
  const { t, tn, errorText } = usePrefs();
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [rules, setRules] = useState<Set<number>>(new Set());
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const cleanKey = key.trim();
  const known = members(ws).find((m) => m.type === "age" && m.id === cleanKey);
  const nameTaken = (ws.config?.keys ?? []).some((k) => k.name === name.trim());
  const ready = validName(name.trim()) && !nameTaken && validAgeKey(cleanKey) && !known?.name && !saving;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!ready || !ws.config) return;
    const ops: ConfigOp[] = [{ op: "addPerson", name: name.trim(), key: cleanKey }];
    for (const rule of rules) ops.push({ op: "grant", rule, key: cleanKey });
    setSaving(true);
    setError(null);
    try {
      await api.config(ws.config.version, ops);
      await onDone();
      location.hash = personHref({ type: "age", id: cleanKey });
    } catch (err) {
      setError(err);
      if (err instanceof ApiError && err.code === "stale") await onDone();
    } finally {
      setSaving(false);
    }
  };

  return (
    <form className="card" onSubmit={submit}>
      <div className="card__header">
        <h3>{t("people.addTitle")}</h3>
      </div>
      <div className="card__body stack">
        {error != null && (
          <Banner tone="danger" icon="alert" title={t("people.addError")}>
            {errorText(error)}
          </Banner>
        )}
        <div className="field-row">
          <label className="field field--key">
            <span className="field__label">{t("people.name")}</span>
            <input
              className="input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="alice"
              spellCheck={false}
              autoComplete="off"
              autoFocus
            />
            <span className="xsmall subtle">
              {name.trim() && !validName(name.trim())
                ? t("errors.invalid_name")
                : nameTaken
                  ? t("errors.name_taken")
                  : tn("people.nameHint", { anchor: <code>&amp;{name.trim() || "name"}</code> })}
            </span>
          </label>
          <label className="field grow">
            <span className="field__label">{t("people.publicKey")}</span>
            <input
              className="input input--mono"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              placeholder="age1…"
              spellCheck={false}
              autoComplete="off"
            />
            <span className="xsmall subtle">
              {cleanKey && !validAgeKey(cleanKey)
                ? t("errors.invalid_recipient")
                : known?.name
                  ? t("people.keyKnown", { name: known.name })
                  : tn("people.publicKeyHint", { command: <code>age-keygen</code> })}
            </span>
          </label>
        </div>

        <div className="stack stack--sm">
          <span className="field__label">{t("people.rules")}</span>
          <RuleChoices
            ws={ws}
            checked={rules}
            onToggle={(i) => {
              const next = new Set(rules);
              if (next.has(i)) next.delete(i);
              else next.add(i);
              setRules(next);
            }}
            disabled={(rule) => !editableRule(rule)}
          />
          <span className="xsmall subtle">{t("people.rulesHint")}</span>
        </div>

        <div className="row">
          <button type="submit" className="btn btn--primary" disabled={!ready}>
            {saving ? <span className="spinner" /> : <Icon name="plus" size={14} />} {t("people.addAction")}
          </button>
          <button type="button" className="btn" onClick={onCancel} disabled={saving}>
            {t("secrets.cancel")}
          </button>
        </div>
      </div>
    </form>
  );
}
