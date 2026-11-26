import { createContext, Fragment, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { api, ApiError, type Settings, type Theme } from "./api";
import { en, type MessageKey, type Messages } from "./i18n/en";
import { es } from "./i18n/es";
import { ptBR } from "./i18n/pt-BR";

export type Locale = "en" | "pt-BR" | "es";

export const LOCALES: { code: Locale; label: string }[] = [
  { code: "en", label: "English" },
  { code: "pt-BR", label: "Português (Brasil)" },
  { code: "es", label: "Español" },
];

const dictionaries: Record<Locale, Messages> = { en, "pt-BR": ptBR, es };

type PluralBase = MessageKey extends infer K ? (K extends `${infer B}_one` ? B : never) : never;
type Params = Record<string, string | number>;

/** Best match for the browser's languages, used until the user picks one. */
function browserLocale(): Locale {
  for (const lang of navigator.languages ?? [navigator.language]) {
    const l = lang.toLowerCase();
    if (l.startsWith("pt")) return "pt-BR";
    if (l.startsWith("es")) return "es";
    if (l.startsWith("en")) return "en";
  }
  return "en";
}

function applyTheme(theme: Theme) {
  if (theme === "system") delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = theme;
}

type Prefs = {
  theme: Theme;
  setTheme: (t: Theme) => void;
  locale: Locale;
  setLocale: (l: Locale) => void;
  /** Plain text. */
  t: (key: MessageKey, params?: Params) => string;
  /** Plural text: tp("file.keys", 3) uses file.keys_one / file.keys_other; {count} is filled in. */
  tp: (base: PluralBase, count: number, params?: Params) => string;
  /** Text with React nodes in placeholders: tn("people.subtitle", { config: <code>.sops.yaml</code> }). */
  tn: (key: MessageKey, nodes: Record<string, ReactNode>) => ReactNode;
  /** Translated message for an error from the API (by code), or its raw message. */
  errorText: (e: unknown) => string;
  relativeTime: (iso: string) => string;
};

const PrefsContext = createContext<Prefs | null>(null);

export function PrefsProvider({ children }: { children: ReactNode }) {
  // The server already applied saved settings to <html>, so the first paint is right.
  const [theme, setThemeState] = useState<Theme>(
    () => (document.documentElement.dataset.theme as Theme | undefined) ?? "system",
  );
  const [chosenLocale, setChosenLocale] = useState<Locale | "">(() => {
    const lang = document.documentElement.lang as Locale;
    return lang !== "en" && lang in dictionaries ? lang : "";
  });
  const locale: Locale = chosenLocale || browserLocale();

  useEffect(() => {
    api
      .settings()
      .then((s) => {
        setThemeState(s.theme);
        applyTheme(s.theme);
        setChosenLocale((s.locale as Locale) || "");
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const save = useCallback((s: Settings) => {
    api.saveSettings(s).catch(() => {});
  }, []);

  const value = useMemo<Prefs>(() => {
    const messages = dictionaries[locale];
    const fill = (text: string, params?: Params) =>
      params ? text.replace(/\{(\w+)\}/g, (m, k: string) => (k in params ? String(params[k]) : m)) : text;
    const t = (key: MessageKey, params?: Params) => fill(messages[key] ?? en[key], params);
    const plural = new Intl.PluralRules(locale);
    const tp = (base: PluralBase, count: number, params?: Params) => {
      const form = plural.select(count) === "one" ? "one" : "other";
      return t(`${base}_${form}` as MessageKey, { count, ...params });
    };
    const tn = (key: MessageKey, nodes: Record<string, ReactNode>) =>
      (messages[key] ?? en[key]).split(/(\{\w+\})/).map((part, i) => {
        const m = /^\{(\w+)\}$/.exec(part);
        return <Fragment key={i}>{m && m[1] in nodes ? nodes[m[1]] : part}</Fragment>;
      });
    const errorText = (e: unknown) => {
      if (e instanceof ApiError && e.code && `errors.${e.code}` in en) return t(`errors.${e.code}` as MessageKey);
      return e instanceof Error ? e.message : String(e);
    };
    const rtf = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
    const relativeTime = (iso: string) => {
      const then = new Date(iso).getTime();
      if (!then) return "";
      let v = Math.round((Date.now() - then) / 1000);
      const steps: [number, Intl.RelativeTimeFormatUnit][] = [
        [60, "second"],
        [60, "minute"],
        [24, "hour"],
        [30, "day"],
        [12, "month"],
        [Infinity, "year"],
      ];
      for (const [step, unit] of steps) {
        if (Math.abs(v) < step) return rtf.format(-v, unit);
        v = Math.round(v / step);
      }
      return "";
    };
    return {
      theme,
      setTheme: (next) => {
        setThemeState(next);
        applyTheme(next);
        save({ theme: next, locale: chosenLocale });
      },
      locale,
      setLocale: (next) => {
        setChosenLocale(next);
        save({ theme, locale: next });
      },
      t,
      tp,
      tn,
      errorText,
      relativeTime,
    };
  }, [theme, locale, chosenLocale, save]);

  return <PrefsContext.Provider value={value}>{children}</PrefsContext.Provider>;
}

export function usePrefs(): Prefs {
  const ctx = useContext(PrefsContext);
  if (!ctx) throw new Error("usePrefs must be used inside <PrefsProvider>");
  return ctx;
}
