import { useCallback, useEffect, useState } from "react";
import { api, type Workspace } from "./api";
import { Icon } from "./components/Icon";
import { Preferences } from "./components/Preferences";
import { Banner, Empty } from "./components/ui";
import { basename, groupByDir, members, rootName } from "./model";
import { usePrefs } from "./prefs";
import { FileView } from "./views/FileView";
import { PeopleView } from "./views/PeopleView";
import { RulesView } from "./views/RulesView";

type Route = { view: "file"; path: string } | { view: "people" } | { view: "rules" } | { view: "home" };

function parseRoute(hash: string): Route {
  const [, view, ...rest] = hash.replace(/^#/, "").split("/");
  if (view === "file" && rest.length) return { view: "file", path: decodeURIComponent(rest.join("/")) };
  if (view === "people") return { view: "people" };
  if (view === "rules") return { view: "rules" };
  return { view: "home" };
}

function useRoute(): Route {
  const [route, setRoute] = useState(() => parseRoute(location.hash));
  useEffect(() => {
    const on = () => setRoute(parseRoute(location.hash));
    addEventListener("hashchange", on);
    return () => removeEventListener("hashchange", on);
  }, []);
  return route;
}

export function App() {
  const { t, errorText } = usePrefs();
  const [ws, setWs] = useState<Workspace | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const route = useRoute();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setWs(await api.workspace());
      setError(null);
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  if (!ws) {
    return (
      <div className="empty">
        {error != null ? (
          <Banner tone="danger" icon="alert" title={t("load.error")}>
            {errorText(error)}
          </Banner>
        ) : (
          <span className="spinner" />
        )}
      </div>
    );
  }

  return (
    <div className="app">
      <Sidebar ws={ws} route={route} onRefresh={load} refreshing={loading} />
      <main className="main">
        {error != null && (
          <div className="page">
            <Banner tone="danger" icon="alert" title={t("load.refreshFailed")}>
              {errorText(error)}
            </Banner>
          </div>
        )}
        <Content ws={ws} route={route} onChanged={load} />
      </main>
    </div>
  );
}

function Content({ ws, route, onChanged }: { ws: Workspace; route: Route; onChanged: () => void }) {
  const { t, tn } = usePrefs();
  if (route.view === "people") return <PeopleView ws={ws} />;
  if (route.view === "rules") return <RulesView ws={ws} />;
  const path = route.view === "file" ? route.path : ws.files[0]?.path;
  const file = ws.files.find((f) => f.path === path);
  if (file) return <FileView key={file.path} ws={ws} file={file} onChanged={onChanged} />;

  let title = t("empty.noFiles");
  let hint = tn("empty.noFilesHint", { root: <code>{ws.root}</code> });
  if (route.view === "file") {
    title = t("empty.fileNotFound");
    hint = <code>{route.path}</code>;
  }
  return (
    <div className="page">
      <Empty icon="file" title={title}>
        {hint}
      </Empty>
    </div>
  );
}

function Sidebar({
  ws,
  route,
  onRefresh,
  refreshing,
}: {
  ws: Workspace;
  route: Route;
  onRefresh: () => void;
  refreshing: boolean;
}) {
  const { t, tp } = usePrefs();
  const activePath = route.view === "file" ? route.path : route.view === "home" ? ws.files[0]?.path : undefined;
  const drifted = ws.files.filter((f) => f.drift).length;
  const me = members(ws).find((m) => m.type === "age" && (ws.identity.recipients ?? []).includes(m.id));

  return (
    <aside className="sidebar">
      <div className="sidebar__header">
        <div className="brand grow">
          <img className="brand__logo" src="/finnick.svg" alt="" width={40} height={36} />
          <span>Finnick</span>
        </div>
        <button
          type="button"
          className="btn btn--ghost btn--icon"
          onClick={onRefresh}
          title={t("nav.rescan")}
          aria-label={t("nav.rescan")}
        >
          {refreshing ? <span className="spinner" /> : <Icon name="refresh" size={15} />}
        </button>
      </div>

      <nav className="sidebar__body" aria-label="Workspace">
        <div className="nav-section">
          <div className="nav-section__title truncate" title={ws.root}>
            {rootName(ws.root)}
          </div>
          <a className="nav-item" href="#/people" aria-current={route.view === "people" ? "page" : undefined}>
            <Icon name="users" size={15} /> {t("nav.people")}
          </a>
          <a className="nav-item" href="#/rules" aria-current={route.view === "rules" ? "page" : undefined}>
            <Icon name="rules" size={15} /> {t("nav.rules")}
            {drifted > 0 && (
              <span className="nav-item__meta" title={tp("nav.filesOutOfSync", drifted)}>
                <span className="dot dot--warning" />
              </span>
            )}
          </a>
        </div>

        <div className="nav-section">
          <div className="nav-section__title">{t("nav.secrets")}</div>
          {groupByDir(ws.files).map(({ dir, files }) => (
            <div key={dir}>
              {dir && (
                <div className="nav-group__label" title={dir}>
                  <Icon name="folder" size={15} />
                  <span className="truncate">{dir}</span>
                </div>
              )}
              {files.map((f) => (
                <a
                  key={f.path}
                  className={dir ? "nav-item nav-item--nested" : "nav-item"}
                  href={`#/file/${encodeURIComponent(f.path)}`}
                  aria-current={activePath === f.path ? "page" : undefined}
                  title={f.path}
                >
                  <span className="truncate mono">{basename(f.path)}</span>
                  <span className="nav-item__meta">
                    {f.drift && <span className="dot dot--warning" title={t("nav.outOfSync")} />}
                    <Icon
                      name={f.canDecrypt ? "unlock" : "lock"}
                      size={13}
                      label={f.canDecrypt ? t("nav.canDecrypt") : t("nav.locked")}
                    />
                  </span>
                </a>
              ))}
            </div>
          ))}
          {ws.files.length === 0 && <p className="small muted nav-item">{t("nav.noFiles")}</p>}
        </div>
      </nav>

      <div className="sidebar__footer">
        {ws.identity.error ? (
          <div className="row small" title={ws.identity.source}>
            <Icon name="alert" size={14} />
            <span className="muted">{t("identity.noKey")}</span>
          </div>
        ) : (
          <div className="row small" title={`${ws.identity.source}\n${(ws.identity.recipients ?? []).join("\n")}`}>
            <span className="avatar">{(me?.name ?? "me").slice(0, 2)}</span>
            <div className="grow">
              <div className="truncate">{me?.name ?? t("identity.yourKey")}</div>
              <div className="xsmall subtle truncate">{me ? t("identity.ageIdentity") : t("identity.notRecipient")}</div>
            </div>
            <span className="xsmall subtle">v{ws.version}</span>
          </div>
        )}
        <Preferences />
      </div>
    </aside>
  );
}
