import type { Workspace } from "../api";
import { Icon } from "../components/Icon";
import { SyncAll } from "../components/SyncAll";
import { Banner, Empty, RecipientChip } from "../components/ui";
import { flat, isMe, recipientKey } from "../model";
import { usePrefs } from "../prefs";

export function RulesView({ ws, onChanged }: { ws: Workspace; onChanged: () => Promise<void> }) {
  const { t, tp, tn } = usePrefs();

  if (!ws.config) {
    return (
      <div className="page">
        <header className="page__header">
          <h1>{t("rules.title")}</h1>
        </header>
        <Banner tone="warning" icon="alert" title={t("rules.noConfigTitle")}>
          {tn("rules.noConfigBody", { root: <code>{ws.root}</code> })}
        </Banner>
      </div>
    );
  }

  const rules = ws.config.rules ?? [];
  const unmatched = ws.files.filter((f) => f.rule < 0);

  return (
    <div className="page">
      <header className="page__header">
        <div className="stack stack--sm">
          <h1>{t("rules.title")}</h1>
          <p className="muted">
            {tn("rules.subtitle", {
              rules: <code>creation_rules</code>,
              path: <code className="small">{ws.config.path}</code>,
              first: <strong>{t("rules.first")}</strong>,
              regex: <code>path_regex</code>,
            })}
          </p>
        </div>
      </header>

      <div className="stack stack--lg">
        <SyncAll ws={ws} onChanged={onChanged} />
        {rules.length === 0 ? (
          <Empty icon="rules" title={t("rules.emptyTitle")}>
            {t("rules.emptyBody")}
          </Empty>
        ) : (
          <ol className="stack">
            {rules.map((rule) => {
              const files = ws.files.filter((f) => f.rule === rule.index);
              const recipients = flat(rule.keyGroups);
              return (
                <li key={rule.index} className="card">
                  <div className="card__header">
                    <div className="row grow">
                      <span className="badge">#{rule.index + 1}</span>
                      <code className="truncate">{rule.pathRegex || t("rules.noRegex")}</code>
                    </div>
                    <span className="small muted">{tp("rules.files", files.length)}</span>
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
                        {files.map((f) => (
                          <li key={f.path}>
                            <a className="badge" href={`#/file/${encodeURIComponent(f.path)}`}>
                              <Icon name="file" size={12} /> {f.path}
                              {f.drift && <span className="dot dot--warning" title={t("rules.outOfSync")} />}
                            </a>
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                </li>
              );
            })}
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
