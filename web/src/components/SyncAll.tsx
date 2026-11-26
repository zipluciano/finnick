import { useState } from "react";
import { api, type Workspace } from "../api";
import { usePrefs } from "../prefs";
import { Icon } from "./Icon";
import { Banner } from "./ui";

type SyncResult = { updated: number; failed: { path: string; error: unknown }[] };

/** Runs updatekeys on every out-of-sync file the user can decrypt, one at a time. */
export function SyncAll({
  ws,
  onChanged,
  exclude = [],
}: {
  ws: Workspace;
  onChanged: () => Promise<void>;
  /** Files another part of the page already deals with. */
  exclude?: string[];
}) {
  const { t, tp, errorText } = usePrefs();
  const [progress, setProgress] = useState<number | null>(null);
  const [result, setResult] = useState<SyncResult | null>(null);
  const drifted = ws.files.filter((f) => f.drift && f.rule >= 0 && !exclude.includes(f.path));
  const mine = drifted.filter((f) => f.canDecrypt);
  const locked = drifted.length - mine.length;

  const run = async () => {
    const res: SyncResult = { updated: 0, failed: [] };
    setResult(null);
    for (const [i, f] of mine.entries()) {
      setProgress(i);
      try {
        await api.updateKeys(f.path);
        res.updated++;
      } catch (error) {
        res.failed.push({ path: f.path, error });
      }
    }
    setProgress(null);
    setResult(res);
    await onChanged();
  };

  if (drifted.length === 0 && !result) return null;
  return (
    <div className="stack">
      {drifted.length > 0 && (
        <Banner tone="warning" icon="alert" title={tp("rules.drifted", drifted.length)}>
          {mine.length > 0 && (
            <div className="row row--wrap">
              <button type="button" className="btn btn--primary" onClick={run} disabled={progress !== null}>
                {progress !== null ? <span className="spinner" /> : <Icon name="refresh" size={14} />}{" "}
                {tp("rules.updateAll", mine.length)}
              </button>
              {progress !== null && (
                <span className="small">{t("rules.updating", { done: progress, total: mine.length })}</span>
              )}
            </div>
          )}
          {locked > 0 && <span>{tp("rules.lockedDrift", locked)}</span>}
        </Banner>
      )}
      {result && (
        <Banner
          tone={result.failed.length ? "danger" : "info"}
          icon={result.failed.length ? "alert" : "check"}
          title={tp("rules.updated", result.updated)}
        >
          {result.failed.length > 0 && (
            <ul className="stack stack--sm">
              {result.failed.map((f) => (
                <li key={f.path}>
                  <code>{f.path}</code>: {errorText(f.error)}
                </li>
              ))}
            </ul>
          )}
        </Banner>
      )}
    </div>
  );
}
