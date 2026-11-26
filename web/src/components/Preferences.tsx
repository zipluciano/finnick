import type { Theme } from "../api";
import { LOCALES, usePrefs, type Locale } from "../prefs";
import { Icon, type IconName } from "./Icon";

const themes: { value: Theme; icon: IconName }[] = [
  { value: "system", icon: "monitor" },
  { value: "light", icon: "sun" },
  { value: "dark", icon: "moon" },
];

/** Theme and language switchers. Choices are saved to ~/.config/finnick/settings.json. */
export function Preferences() {
  const { t, theme, setTheme, locale, setLocale } = usePrefs();
  return (
    <div className="prefs">
      <div className="segmented segmented--icons" role="radiogroup" aria-label={t("settings.theme")}>
        {themes.map(({ value, icon }) => {
          const label = t(`settings.theme.${value}`);
          return (
            <button
              key={value}
              type="button"
              role="radio"
              aria-checked={theme === value}
              aria-pressed={theme === value}
              title={label}
              aria-label={label}
              onClick={() => setTheme(value)}
            >
              <Icon name={icon} size={14} />
            </button>
          );
        })}
      </div>
      <label className="select-with-icon" title={t("settings.language")}>
        <Icon name="globe" size={14} />
        <select
          className="select"
          value={locale}
          aria-label={t("settings.language")}
          onChange={(e) => setLocale(e.target.value as Locale)}
        >
          {LOCALES.map((l) => (
            <option key={l.code} value={l.code}>
              {l.label}
            </option>
          ))}
        </select>
      </label>
    </div>
  );
}
