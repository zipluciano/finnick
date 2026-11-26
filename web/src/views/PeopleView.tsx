import type { Workspace } from "../api";
import { Icon } from "../components/Icon";
import { Avatar, CopyButton, Empty, YouBadge } from "../components/ui";
import { access, isMe, members, recipientKey, recipientLabel, shortId, typeLabel } from "../model";
import { usePrefs } from "../prefs";

export function PeopleView({ ws }: { ws: Workspace }) {
  const { t, tn } = usePrefs();
  const people = members(ws);
  const config = <code>.sops.yaml</code>;

  return (
    <div className="page">
      <header className="page__header">
        <div className="stack stack--sm">
          <h1>{t("people.title")}</h1>
          <p className="muted">{tn("people.subtitle", { config })}</p>
        </div>
      </header>

      {people.length === 0 ? (
        <Empty icon="users" title={t("people.emptyTitle")}>
          {tn("people.emptyBody", { config })}
        </Empty>
      ) : (
        <div className="stack">
          <div className="card matrix">
            <table className="table">
              <thead>
                <tr>
                  <th>{t("people.recipient")}</th>
                  {ws.files.map((f) => (
                    <th key={f.path} className="matrix__col" title={f.path}>
                      <a href={`#/file/${encodeURIComponent(f.path)}`}>{f.path}</a>
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {people.map((p) => (
                  <tr key={recipientKey(p)}>
                    <td>
                      <div className="row">
                        <Avatar r={p} />
                        <div className="stack stack--sm grow" title={p.id}>
                          <div className="row">
                            <strong className="truncate">{recipientLabel(p)}</strong>
                            {isMe(ws, p) && <YouBadge />}
                          </div>
                          <div className="row xsmall muted">
                            <span>{typeLabel(p.type)}</span>
                            <span className="mono truncate">{shortId(p.id)}</span>
                            <CopyButton text={p.id} label={t("common.copyPublicKey")} />
                          </div>
                        </div>
                      </div>
                    </td>
                    {ws.files.map((f) => (
                      <td key={f.path} className="matrix__cell">
                        <Cell state={access(ws, f, p)} />
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="legend">
            <span className="row">
              <Cell state="granted" /> {t("people.legendGranted")}
            </span>
            <span className="row">
              <Cell state="pending" /> {t("people.legendPending")}
            </span>
            <span className="row">
              <Cell state="revoking" /> {t("people.legendRevoking")}
            </span>
          </div>
        </div>
      )}
    </div>
  );
}

function Cell({ state }: { state: ReturnType<typeof access> }) {
  const { t } = usePrefs();
  switch (state) {
    case "granted":
      return (
        <span className="badge badge--success" title={t("access.granted")}>
          <Icon name="check" size={12} />
        </span>
      );
    case "pending":
      return (
        <span className="badge badge--warning" title={t("access.pending")}>
          <Icon name="refresh" size={12} />
        </span>
      );
    case "revoking":
      return (
        <span className="badge badge--danger" title={t("access.revoking")}>
          <Icon name="alert" size={12} />
        </span>
      );
    default:
      return <span className="subtle">·</span>;
  }
}
