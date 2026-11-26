import { useState } from "react";
import { api, ApiError, type AccessProposal, type Workspace } from "../api";
import { Icon } from "../components/Icon";
import { Banner, CopyButton, useAction } from "../components/ui";
import { editableRule, members, validName } from "../model";
import { usePrefs } from "../prefs";
import { RuleChoices } from "./PersonView";

/**
 * For someone new to the repository: get an age key, then propose adding it to .sops.yaml as a
 * branch on origin, for someone with access to review and merge.
 */
export function RequestAccessView({ ws, onChanged }: { ws: Workspace; onChanged: () => Promise<void> }) {
  const { t, tn } = usePrefs();
  const key = ws.identity.recipients?.[0];
  const known = key ? members(ws).find((m) => m.type === "age" && m.id === key && m.name) : undefined;
  const [generated, setGenerated] = useState(false);

  return (
    <div className="page">
      <header className="page__header">
        <div className="stack stack--sm">
          <h1>{t("request.title")}</h1>
          <p className="muted">{tn("request.subtitle", { config: <code>.sops.yaml</code> })}</p>
        </div>
      </header>

      <div className="stack stack--lg">
        <KeyCard
          ws={ws}
          generated={generated}
          onGenerated={async () => {
            setGenerated(true);
            await onChanged();
          }}
        />
        {key && known && (
          <Banner icon="check" title={t("request.alreadyIn", { name: known.name ?? "" })}>
            {t("request.alreadyInBody")}
          </Banner>
        )}
        {key && !known && <ProposeCard ws={ws} keyId={key} />}
      </div>
    </div>
  );
}

function KeyCard({ ws, generated, onGenerated }: { ws: Workspace; generated: boolean; onGenerated: () => Promise<void> }) {
  const { t, tn, errorText } = usePrefs();
  const { busy, error, run } = useAction();
  const key = ws.identity.recipients?.[0];
  const where = <code>{ws.identity.source}</code>;
  return (
    <div className="card">
      <div className="card__header">
        <h3>{t("request.keyTitle")}</h3>
      </div>
      <div className="card__body stack">
        {key ? (
          <>
            <div className="cmd cmd--plain">
              <span className="grow truncate">{key}</span>
              <CopyButton text={key} label={t("common.copyPublicKey")} />
            </div>
            <span className="xsmall subtle">{tn("request.keyFrom", { path: where })}</span>
            {generated && (
              <Banner tone="warning" icon="alert" title={t("request.backupTitle")}>
                <span>{tn("request.backupBody", { path: where })}</span>
              </Banner>
            )}
          </>
        ) : (
          <>
            <p className="small muted">{tn("request.noKey", { path: where })}</p>
            {error != null && (
              <Banner tone="danger" icon="alert" title={t("request.generateError")}>
                {errorText(error)}
              </Banner>
            )}
            <div>
              <button
                type="button"
                className="btn btn--primary"
                disabled={busy}
                onClick={async () => {
                  if (await run(() => api.generateIdentity())) await onGenerated();
                }}
              >
                {busy ? <span className="spinner" /> : <Icon name="key" size={14} />} {t("request.generate")}
              </button>
            </div>
            <span className="xsmall subtle">{t("request.generateHint")}</span>
          </>
        )}
      </div>
    </div>
  );
}

function ProposeCard({ ws, keyId }: { ws: Workspace; keyId: string }) {
  const { t, tn, errorText } = usePrefs();
  const [name, setName] = useState("");
  const [rules, setRules] = useState<Set<number>>(new Set());
  const [proposal, setProposal] = useState<AccessProposal | null>(null);
  const { busy, error, run } = useAction();

  if (!ws.config) {
    return (
      <Banner tone="warning" icon="alert" title={t("rules.noConfigTitle")}>
        {t("request.noConfig")}
      </Banner>
    );
  }
  const regexes = (ws.config.rules ?? []).filter((r) => rules.has(r.index)).map((r) => r.pathRegex);
  const ready = validName(name.trim()) && regexes.length > 0 && !busy;
  const request = (push: boolean) =>
    run(async () => setProposal(await api.requestAccess({ name: name.trim(), key: keyId, rules: regexes, push })));
  const reset = () => setProposal(null);

  return (
    <div className="card">
      <div className="card__header">
        <h3>{t("request.proposeTitle")}</h3>
      </div>
      <div className="card__body stack">
        <label className="field field--narrow">
          <span className="field__label">{t("people.name")}</span>
          <input
            className="input"
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              reset();
            }}
            placeholder="alice"
            spellCheck={false}
            autoComplete="off"
            disabled={proposal?.pushed}
          />
          <span className="xsmall subtle">
            {name.trim() && !validName(name.trim()) ? t("errors.invalid_name") : t("request.nameHint")}
          </span>
        </label>
        <div className="stack stack--sm">
          <span className="field__label">{t("request.rules")}</span>
          <RuleChoices
            ws={ws}
            checked={rules}
            disabled={(rule) => !editableRule(rule) || !!proposal?.pushed}
            onToggle={(i) => {
              const next = new Set(rules);
              if (next.has(i)) next.delete(i);
              else next.add(i);
              setRules(next);
              reset();
            }}
          />
        </div>

        {error != null && (
          <Banner tone="danger" icon="alert" title={t("request.error")}>
            {errorText(error)}
            {error instanceof ApiError && error.code === "git_failed" && <code className="xsmall">{error.message}</code>}
          </Banner>
        )}

        {proposal && (
          <div className="stack stack--sm">
            <span className="small muted">
              {tn("request.diffTitle", { file: <code>{proposal.file}</code>, base: <code>origin/{proposal.base}</code> })}
            </span>
            <pre className="diff">
              {proposal.diff.map((l, i) => (
                <div key={i} className={l.op === "+" ? "diff__add" : l.op === "-" ? "diff__del" : undefined}>
                  {l.op} {l.text}
                </div>
              ))}
            </pre>
          </div>
        )}

        {proposal?.pushed ? (
          <Banner icon="check" title={t("request.pushed", { branch: proposal.branch })}>
            {proposal.url ? (
              <a className="btn btn--primary" href={proposal.url} target="_blank" rel="noreferrer noopener">
                {t("request.openPR")}
              </a>
            ) : (
              <span>
                {tn("request.openPRByHand", { branch: <code>{proposal.branch}</code>, base: <code>{proposal.base}</code> })}
              </span>
            )}
            <span>{t("request.next")}</span>
          </Banner>
        ) : (
          <div className="row row--wrap">
            {!proposal ? (
              <button type="button" className="btn btn--primary" disabled={!ready} onClick={() => request(false)}>
                {busy && <span className="spinner" />} {t("request.preview")}
              </button>
            ) : (
              <button type="button" className="btn btn--primary" disabled={!ready} onClick={() => request(true)}>
                {busy ? <span className="spinner" /> : <Icon name="check" size={14} />} {t("request.push")}
              </button>
            )}
            <span className="xsmall subtle">
              {proposal ? tn("request.pushHint", { branch: <code>{proposal.branch}</code> }) : t("request.previewHint")}
            </span>
          </div>
        )}
      </div>
    </div>
  );
}
