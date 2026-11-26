import { useState, type FormEvent } from "react";
import { api, ApiError, type ConfigOp, type Recipient, type SecretFile, type Workspace } from "../api";
import { Icon } from "../components/Icon";
import { SyncAll } from "../components/SyncAll";
import { Banner, Empty, RecipientChip } from "../components/ui";
import {
  agePeople,
  flat,
  isMe,
  matchRule,
  recipientKey,
  recipientLabel,
  sameRecipients,
} from "../model";
import { usePrefs } from "../prefs";

/** A rule being edited: changes stay here, with their ops, until they are saved together. */
type DraftRule = {
  id: number; // stable while editing
  origin: number | null; // position in .sops.yaml, null for a new rule
  pathRegex: string;
  keyGroups: Recipient[][];
};

type Draft = { rules: DraftRule[]; ops: ConfigOp[]; next: number };

function startDraft(ws: Workspace): Draft {
  const rules = (ws.config?.rules ?? []).map((r, i) => ({
    id: i,
    origin: i,
    pathRegex: r.pathRegex,
    keyGroups: r.keyGroups ?? [],
  }));
  return { rules, ops: [], next: rules.length };
}

/** Applies op to the draft exactly as the server will apply it to .sops.yaml. */
function apply(draft: Draft, op: ConfigOp, named: Recipient[]): Draft {
  const rules = draft.rules.map((r) => ({ ...r, keyGroups: r.keyGroups.map((g) => [...g]) }));
  let next = draft.next;
  const recipient = (id: string) => named.find((r) => r.type === "age" && r.id === id) ?? { type: "age", id };
  switch (op.op) {
    case "addRule":
      rules.splice(op.to, 0, { id: next++, origin: null, pathRegex: op.pathRegex, keyGroups: [op.keys.map(recipient)] });
      break;
    case "setRegex":
      rules[op.rule].pathRegex = op.pathRegex;
      break;
    case "moveRule": {
      const [r] = rules.splice(op.rule, 1);
      rules.splice(op.to, 0, r);
      break;
    }
    case "deleteRule":
      rules.splice(op.rule, 1);
      break;
    case "grant": {
      const r = rules[op.rule];
      if (r.keyGroups.length === 0) r.keyGroups.push([]);
      if (!flat(r.keyGroups).some((x) => x.type === "age" && x.id === op.key)) r.keyGroups[0].push(recipient(op.key));
      break;
    }
    case "revoke":
      rules[op.rule].keyGroups = rules[op.rule].keyGroups.map((g) => g.filter((x) => !(x.type === "age" && x.id === op.key)));
      break;
  }
  return { rules, ops: [...draft.ops, op], next };
}

/** Where each file lands with the draft rules, and what changes for it. */
function preview(rules: DraftRule[], f: SecretFile) {
  const at = matchRule(rules, f.path);
  const rule = at === undefined || at < 0 ? undefined : rules[at];
  const moved = at !== undefined && (rule ? rule.origin !== f.rule : f.rule >= 0);
  const outOfSync = rule ? !sameRecipients(flat(rule.keyGroups), flat(f.keyGroups)) : false;
  return { at, rule, moved, outOfSync, unknown: at === undefined };
}

export function RulesView({ ws, onChanged }: { ws: Workspace; onChanged: () => Promise<void> }) {
  const { t, tp, tn, errorText } = usePrefs();
  const [draft, setDraft] = useState<Draft | null>(null);
  const [editing, setEditing] = useState<number | "new" | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const named = ws.config?.keys ?? [];

  const current = draft ?? startDraft(ws);
  const change = (op: ConfigOp) => {
    setDraft(apply(current, op, named));
    setError(null);
  };
  const discard = () => {
    setDraft(null);
    setEditing(null);
    setError(null);
  };
  const save = async () => {
    if (!draft) return;
    setSaving(true);
    setError(null);
    try {
      await api.config(ws.config?.version ?? "", draft.ops);
      setDraft(null);
      setEditing(null);
    } catch (e) {
      setError(e);
      if (e instanceof ApiError && e.code === "stale") setDraft(null);
    } finally {
      setSaving(false);
      await onChanged();
    }
  };

  const rules = current.rules;
  const files = ws.files.map((f) => ({ f, p: preview(rules, f) }));
  const unmatched = files.filter(({ p }) => p.at === -1).map(({ f }) => f);
  const moved = files.filter(({ p }) => p.moved).length;
  const toSync = files.filter(({ p, f }) => p.outOfSync && !f.drift).length;

  return (
    <div className="page">
      <header className="page__header">
        <div className="stack stack--sm">
          <h1>{t("rules.title")}</h1>
          <p className="muted">
            {ws.config
              ? tn("rules.subtitle", {
                  rules: <code>creation_rules</code>,
                  path: <code className="small">{ws.config.path}</code>,
                  first: <strong>{t("rules.first")}</strong>,
                  regex: <code>path_regex</code>,
                })
              : tn("rules.noConfigBody", { root: <code>{ws.root}</code> })}
          </p>
        </div>
        {editing !== "new" && (
          <button type="button" className="btn btn--primary" onClick={() => setEditing("new")} disabled={saving}>
            <Icon name="plus" size={14} /> {ws.config ? t("rules.add") : t("rules.addFirst")}
          </button>
        )}
      </header>

      <div className="stack stack--lg">
        {draft && draft.ops.length > 0 && (
          <Banner tone="info" icon="info" title={t("rules.unsaved")}>
            <span>
              {moved > 0 && tp("rules.previewMoved", moved)} {toSync > 0 && tp("rules.previewSync", toSync)}
              {moved === 0 && toSync === 0 && t("rules.previewNone")}
            </span>
            {error != null && <strong>{errorText(error)}</strong>}
            <div className="row">
              <button type="button" className="btn btn--primary" onClick={save} disabled={saving || editing !== null}>
                {saving ? <span className="spinner" /> : <Icon name="check" size={14} />} {t("rules.save")}
              </button>
              <button type="button" className="btn" onClick={discard} disabled={saving}>
                {t("rules.discard")}
              </button>
            </div>
          </Banner>
        )}
        {error != null && !(draft && draft.ops.length > 0) && (
          <Banner tone="danger" icon="alert" title={t("person.saveError")}>
            {errorText(error)}
          </Banner>
        )}
        {!draft && <SyncAll ws={ws} onChanged={onChanged} />}

        {editing === "new" && (
          <RuleForm
            ws={ws}
            rules={rules}
            onCancel={() => setEditing(null)}
            onDone={(ops) => {
              let d = current;
              for (const op of ops) d = apply(d, op, named);
              setDraft(d);
              setEditing(null);
            }}
          />
        )}

        {rules.length === 0 && editing !== "new" ? (
          <Empty icon="rules" title={t("rules.emptyTitle")}>
            {t("rules.emptyBody")}
          </Empty>
        ) : (
          <ol className="stack">
            {rules.map((rule, i) =>
              editing === rule.id ? (
                <li key={rule.id}>
                  <RuleForm
                    ws={ws}
                    rules={rules}
                    index={i}
                    onCancel={() => setEditing(null)}
                    onDone={(ops) => {
                      let d = current;
                      for (const op of ops) d = apply(d, op, named);
                      setDraft(d);
                      setEditing(null);
                    }}
                  />
                </li>
              ) : (
                <RuleCard
                  key={rule.id}
                  ws={ws}
                  rule={rule}
                  index={i}
                  count={rules.length}
                  files={files.filter(({ p }) => p.rule?.id === rule.id)}
                  drafting={!!draft && draft.ops.length > 0}
                  busy={saving || editing !== null}
                  onChange={change}
                  onEdit={() => setEditing(rule.id)}
                />
              ),
            )}
          </ol>
        )}

        {unmatched.length > 0 && (
          <Banner tone="warning" icon="alert" title={tp("rules.unmatched", unmatched.length)}>
            <ul className="stack stack--sm">
              {unmatched.map((f) => (
                <li key={f.path}>
                  <a href={`#/file/${encodeURIComponent(f.path)}`} className="mono">
                    {f.path}
                  </a>
                </li>
              ))}
            </ul>
            <span>{t("rules.unmatchedBody")}</span>
          </Banner>
        )}
      </div>
    </div>
  );
}

function RuleCard({
  ws,
  rule,
  index,
  count,
  files,
  drafting,
  busy,
  onChange,
  onEdit,
}: {
  ws: Workspace;
  rule: DraftRule;
  index: number;
  count: number;
  files: { f: SecretFile; p: ReturnType<typeof preview> }[];
  drafting: boolean;
  busy: boolean;
  onChange: (op: ConfigOp) => void;
  onEdit: () => void;
}) {
  const { t, tp } = usePrefs();
  const recipients = flat(rule.keyGroups);
  const label = `#${index + 1}`;
  const action = (icon: "arrowUp" | "arrowDown" | "pencil" | "trash", title: string, onClick: () => void, off = false) => (
    <button
      type="button"
      className="btn btn--ghost btn--icon"
      title={title}
      aria-label={title}
      onClick={onClick}
      disabled={busy || off}
    >
      <Icon name={icon} size={14} />
    </button>
  );
  return (
    <li className="card">
      <div className="card__header">
        <div className="row grow">
          <span className="badge">{label}</span>
          <code className="truncate">{rule.pathRegex || t("rules.noRegex")}</code>
          {rule.origin === null && <span className="badge badge--info">{t("rules.new")}</span>}
        </div>
        <span className="small muted">{tp("rules.files", files.length)}</span>
        {ws.config && (
          <div className="row">
            {action("arrowUp", t("rules.moveUp", { rule: label }), () => onChange({ op: "moveRule", rule: index, to: index - 1 }), index === 0)}
            {action("arrowDown", t("rules.moveDown", { rule: label }), () => onChange({ op: "moveRule", rule: index, to: index + 1 }), index === count - 1)}
            {action("pencil", t("rules.edit", { rule: label }), onEdit)}
            {action("trash", t("rules.delete", { rule: label }), () => onChange({ op: "deleteRule", rule: index }))}
          </div>
        )}
      </div>
      <div className="card__body stack">
        <div className="row row--wrap">
          {recipients.length === 0 ? (
            <span className="small muted">{t("rules.noRecipients")}</span>
          ) : (
            recipients.map((r) => <RecipientChip key={recipientKey(r)} r={r} me={isMe(ws, r)} />)
          )}
        </div>
        {files.length > 0 && (
          <ul className="row row--wrap">
            {files.map(({ f, p }) => (
              <li key={f.path}>
                <a className="badge" href={`#/file/${encodeURIComponent(f.path)}`}>
                  <Icon name="file" size={12} /> {f.path}
                  {drafting && p.moved && <span className="badge badge--info">{t("rules.movesHere")}</span>}
                  {(drafting ? p.outOfSync : f.drift) && <span className="dot dot--warning" title={t("rules.outOfSync")} />}
                </a>
              </li>
            ))}
          </ul>
        )}
      </div>
    </li>
  );
}

/** Adds a rule (no index) or changes rule index of the draft; reports the ops that do it. */
function RuleForm({
  ws,
  rules,
  index,
  onDone,
  onCancel,
}: {
  ws: Workspace;
  rules: DraftRule[];
  index?: number;
  onDone: (ops: ConfigOp[]) => void;
  onCancel: () => void;
}) {
  const { t, tp } = usePrefs();
  const rule = index === undefined ? undefined : rules[index];
  const shamir = (rule?.keyGroups.length ?? 0) > 1;
  const people = agePeople(ws);
  for (const r of flat(rule?.keyGroups)) if (r.type === "age" && !people.some((p) => p.id === r.id)) people.push(r);
  const others = flat(rule?.keyGroups).filter((r) => r.type !== "age");
  const before = new Set(flat(rule?.keyGroups).filter((r) => r.type === "age").map((r) => r.id));

  const [regex, setRegex] = useState(rule?.pathRegex ?? "");
  const [keys, setKeys] = useState<Set<string>>(() =>
    rule ? before : new Set((ws.identity.recipients ?? []).slice(0, 1)),
  );
  // Specific rules usually go before broad ones, so a new rule starts at the top.
  const [to, setTo] = useState(0);

  let jsValid = true;
  try {
    new RegExp(regex);
  } catch {
    jsValid = false;
  }
  const matching = jsValid ? ws.files.filter((f) => !regex || new RegExp(regex).test(f.path)) : [];
  // Which of those files the rule would really get, with the rules around it: the first match wins.
  const at = index ?? to;
  const order = [...rules.slice(0, at), { pathRegex: regex }, ...rules.slice(index === undefined ? at : at + 1)];
  const applies = matching.filter((f) => matchRule(order, f.path) === at).length;
  const ready = (keys.size > 0 || others.length > 0) && !/[\n\r]/.test(regex);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!ready) return;
    if (index === undefined) {
      onDone([{ op: "addRule", to, pathRegex: regex, keys: [...keys] }]);
      return;
    }
    const ops: ConfigOp[] = [];
    if (regex !== rule!.pathRegex) ops.push({ op: "setRegex", rule: index, pathRegex: regex });
    for (const k of keys) if (!before.has(k)) ops.push({ op: "grant", rule: index, key: k });
    for (const k of before) if (!keys.has(k)) ops.push({ op: "revoke", rule: index, key: k });
    onDone(ops);
  };

  return (
    <form className="card" onSubmit={submit}>
      <div className="card__header">
        <h3>{index === undefined ? t("rules.addTitle") : t("rules.editTitle", { rule: `#${index + 1}` })}</h3>
      </div>
      <div className="card__body stack">
        <label className="field">
          <span className="field__label">path_regex</span>
          <input
            className="input input--mono"
            value={regex}
            onChange={(e) => setRegex(e.target.value)}
            placeholder={t("rules.regexPlaceholder")}
            spellCheck={false}
            autoComplete="off"
            autoFocus
          />
          <span className="xsmall subtle">
            {!jsValid
              ? t("rules.regexUnknown")
              : regex
                ? tp("rules.regexMatches", matching.length)
                : t("rules.noRegex")}
          </span>
          {matching.length > 0 && (
            <span className="row row--wrap">
              {matching.map((f) => (
                <span key={f.path} className="badge mono">
                  {f.path}
                </span>
              ))}
            </span>
          )}
          {jsValid && applies < matching.length && (
            <span className="xsmall">{t("rules.shadowed", { applies, total: matching.length })}</span>
          )}
        </label>

        <div className="stack stack--sm">
          <span className="field__label">{t("rules.recipients")}</span>
          {shamir ? (
            <p className="small muted">{t("errors.key_groups")}</p>
          ) : people.length === 0 ? (
            <p className="small muted">{t("rules.noPeople")}</p>
          ) : (
            <div className="choices">
              {people.map((p) => (
                <label key={p.id} className="choice">
                  <input
                    type="checkbox"
                    checked={keys.has(p.id)}
                    onChange={() => {
                      const next = new Set(keys);
                      if (next.has(p.id)) next.delete(p.id);
                      else next.add(p.id);
                      setKeys(next);
                    }}
                  />
                  <strong className="truncate">{recipientLabel(p)}</strong>
                  {isMe(ws, p) && <span className="badge badge--info">{t("common.you")}</span>}
                  <span className="mono xsmall subtle truncate grow">{p.id}</span>
                </label>
              ))}
            </div>
          )}
          {others.length > 0 && (
            <div className="row row--wrap">
              {others.map((r) => (
                <RecipientChip key={recipientKey(r)} r={r} />
              ))}
              <span className="xsmall subtle">{t("rules.othersKept")}</span>
            </div>
          )}
        </div>

        {index === undefined && rules.length > 0 && (
          <label className="field">
            <span className="field__label">{t("rules.position")}</span>
            <select className="input" value={to} onChange={(e) => setTo(Number(e.target.value))}>
              {rules.map((r, i) => (
                <option key={r.id} value={i}>
                  {t("rules.before", { rule: `#${i + 1}`, regex: r.pathRegex || t("rules.everything") })}
                </option>
              ))}
              <option value={rules.length}>{t("rules.atEnd")}</option>
            </select>
          </label>
        )}

        <div className="row">
          <button type="submit" className="btn btn--primary" disabled={!ready}>
            <Icon name="check" size={14} /> {t("rules.apply")}
          </button>
          <button type="button" className="btn" onClick={onCancel}>
            {t("secrets.cancel")}
          </button>
          <span className="xsmall subtle">{t("rules.applyHint")}</span>
        </div>
      </div>
    </form>
  );
}
