import { useEffect, useMemo, useState } from "react";
import { api, type Recipient, type SecretFile, type Workspace } from "../api";
import { Icon } from "../components/Icon";
import { Banner, Command, CopyButton, Empty, RecipientRow } from "../components/ui";
import { access, flat, isMe, recipientKey } from "../model";
import { usePrefs } from "../prefs";

const AUTO_HIDE_MS = 2 * 60 * 1000;

export function FileView({ ws, file }: { ws: Workspace; file: SecretFile }) {
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

      {file.drift && <DriftBanner file={file} />}

      <div className="tabs" role="tablist">
        <button type="button" role="tab" className="tab" aria-selected={tab === "secrets"} onClick={() => setTab("secrets")}>
          <Icon name="key" size={14} /> {t("tabs.secrets")}
        </button>
        <button type="button" role="tab" className="tab" aria-selected={tab === "access"} onClick={() => setTab("access")}>
          <Icon name="users" size={14} /> {t("tabs.access")}
          {file.drift && <span className="dot dot--warning" />}
        </button>
      </div>

      {tab === "secrets" ? <SecretsTab file={file} /> : <AccessTab ws={ws} file={file} />}
    </div>
  );
}

function DriftBanner({ file }: { file: SecretFile }) {
  const { t, tp } = usePrefs();
  const { missing, extra } = file.drift!;
  const names = (rs: Recipient[]) => rs.map((r) => r.name ?? r.type).join(", ");
  return (
    <div className="page__section">
      <Banner tone="warning" icon="alert" title={t("drift.title")}>
        <span>
          {missing.length > 0 && tp("drift.missing", missing.length, { names: names(missing) })}{" "}
          {extra.length > 0 && tp("drift.extra", extra.length, { names: names(extra) })} {t("drift.fix")}
        </span>
        <Command>{`sops updatekeys -y ${file.path}`}</Command>
        {extra.length > 0 && <span>{t("drift.revokeWarning")}</span>}
      </Banner>
    </div>
  );
}

function SecretsTab({ file }: { file: SecretFile }) {
  const { t, errorText } = usePrefs();
  const [values, setValues] = useState<Map<string, string> | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [filter, setFilter] = useState("");

  useEffect(() => {
    if (!values) return;
    const timer = setTimeout(() => setValues(null), AUTO_HIDE_MS);
    return () => clearTimeout(timer);
  }, [values]);

  const reveal = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.reveal(file.path);
      setValues(new Map(res.entries.map((e) => [e.key, e.value])));
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  };

  const keys = useMemo(
    () => file.keys.filter((k) => k.toLowerCase().includes(filter.trim().toLowerCase())),
    [file.keys, filter],
  );

  return (
    <div className="stack">
      {!file.canDecrypt && (
        <Banner tone="info" icon="lock" title={t("secrets.notRecipientTitle")}>
          {t("secrets.notRecipientBody")}
        </Banner>
      )}
      {error != null && (
        <Banner tone="danger" icon="alert" title={t("secrets.decryptError")}>
          {errorText(error)}
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
          {values ? (
            <button type="button" className="btn" onClick={() => setValues(null)}>
              <Icon name="eyeOff" size={14} /> {t("secrets.hide")}
            </button>
          ) : (
            <button type="button" className="btn btn--primary" onClick={reveal} disabled={!file.canDecrypt || loading}>
              {loading ? <span className="spinner" /> : <Icon name="eye" size={14} />} {t("secrets.reveal")}
            </button>
          )}
        </div>
        {keys.length === 0 ? (
          <Empty icon="search" title={file.keys.length ? t("secrets.noMatch") : t("secrets.noKeys")} />
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>{t("secrets.key")}</th>
                <th>{t("secrets.value")}</th>
                <th className="table__actions" />
              </tr>
            </thead>
            <tbody>
              {keys.map((k) => {
                const v = values?.get(k);
                return (
                  <tr key={k}>
                    <td className="mono">{k}</td>
                    <td>
                      {v === undefined ? (
                        <span className="secret-value secret-value--masked">••••••••••</span>
                      ) : (
                        <span className="secret-value">{v === "" ? <span className="subtle">{t("secrets.empty")}</span> : v}</span>
                      )}
                    </td>
                    <td className="table__actions">
                      {v !== undefined && <CopyButton text={v} label={t("secrets.copy", { key: k })} />}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
      {values && <p className="xsmall subtle">{t("secrets.autoHide")}</p>}
    </div>
  );
}

function AccessTab({ ws, file }: { ws: Workspace; file: SecretFile }) {
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
