import { useState, type ReactNode } from "react";
import type { Recipient } from "../api";
import { recipientLabel, shortId, typeLabel } from "../model";
import { usePrefs } from "../prefs";
import { Icon, type IconName } from "./Icon";

export function CopyButton({ text, label }: { text: string; label?: string }) {
  const { t } = usePrefs();
  const [done, setDone] = useState(false);
  const title = label ?? t("common.copy");
  return (
    <button
      type="button"
      className="btn btn--ghost btn--icon"
      title={title}
      aria-label={title}
      onClick={async () => {
        await navigator.clipboard.writeText(text);
        setDone(true);
        setTimeout(() => setDone(false), 1200);
      }}
    >
      <Icon name={done ? "check" : "copy"} size={14} />
    </button>
  );
}

export function Command({ children }: { children: string }) {
  const { t } = usePrefs();
  return (
    <div className="cmd">
      <span className="grow truncate">{children}</span>
      <CopyButton text={children} label={t("common.copyCommand")} />
    </div>
  );
}

export function Avatar({ r }: { r: Recipient }) {
  return (
    <span className={r.name ? "avatar" : "avatar avatar--key"}>
      {r.name ? r.name.slice(0, 2) : <Icon name={r.type === "age" ? "key" : "cloud"} size={12} />}
    </span>
  );
}

export function YouBadge() {
  const { t } = usePrefs();
  return <span className="badge badge--info">{t("common.you")}</span>;
}

export function RecipientChip({ r, me }: { r: Recipient; me?: boolean }) {
  return (
    <span className="chip" title={`${typeLabel(r.type)}: ${r.id}`}>
      <Avatar r={r} />
      <span className="truncate">{recipientLabel(r)}</span>
      {r.type !== "age" && <span className="badge">{typeLabel(r.type)}</span>}
      {me && <YouBadge />}
    </span>
  );
}

export function RecipientRow({ r, me, status }: { r: Recipient; me?: boolean; status?: ReactNode }) {
  const { t } = usePrefs();
  return (
    <tr>
      <td>
        <div className="row">
          <Avatar r={r} />
          <strong>{recipientLabel(r)}</strong>
          {me && <YouBadge />}
        </div>
      </td>
      <td>
        <span className="badge">{typeLabel(r.type)}</span>
      </td>
      <td className="mono muted" title={r.id}>
        <div className="row">
          <span className="truncate">{shortId(r.id)}</span>
          <CopyButton text={r.id} label={t("common.copyPublicKey")} />
        </div>
      </td>
      <td className="table__actions">{status}</td>
    </tr>
  );
}

export function Banner({
  tone = "info",
  icon = "info",
  title,
  children,
}: {
  tone?: "info" | "warning" | "danger";
  icon?: IconName;
  title: string;
  children?: ReactNode;
}) {
  return (
    <div className={`banner banner--${tone}`} role={tone === "info" ? "status" : "alert"}>
      <Icon name={icon} />
      <div className="stack stack--sm grow">
        <div className="banner__title">{title}</div>
        {children}
      </div>
    </div>
  );
}

export function Empty({ icon, title, children }: { icon: IconName; title: string; children?: ReactNode }) {
  return (
    <div className="empty">
      <Icon name={icon} size={28} />
      <div className="empty__title">{title}</div>
      {children && <div className="small">{children}</div>}
    </div>
  );
}
