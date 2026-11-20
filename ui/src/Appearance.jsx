import React, { useEffect, useState } from "react";

export function useAppearance(api) {
  const [theme, setTheme] = useState("light"),
    [loaded, setLoaded] = useState(false),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  useEffect(() => {
    let alive = true;
    api("preferences")
      .then((value) => {
        if (alive) {
          setTheme(value.theme);
          setLoaded(true);
        }
      })
      .catch(() => {
        if (alive)
          setError(
            "Appearance preferences could not be loaded. Other downloads still work.",
          );
      });
    return () => {
      alive = false;
    };
  }, []);
  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      document.documentElement.dataset.theme =
        theme === "system" ? (media.matches ? "dark" : "light") : theme;
    };
    apply();
    media.addEventListener("change", apply);
    return () => media.removeEventListener("change", apply);
  }, [theme]);
  async function choose(value) {
    setBusy(true);
    setError("");
    try {
      const saved = await api("preferences", { theme: value });
      setTheme(saved.theme);
    } catch {
      setError(
        "Appearance could not be saved. Check access to your app data folder and try again.",
      );
    } finally {
      setBusy(false);
    }
  }
  return { theme, loaded, error, busy, choose };
}

export function AppearanceSettings({ appearance }) {
  return (
    <section
      className="appearance-settings"
      aria-label="Appearance preferences"
    >
      <h3>Appearance</h3>
      <label className="field">
        Color theme
        <select
          value={appearance.theme}
          disabled={!appearance.loaded || appearance.busy}
          onChange={(event) => appearance.choose(event.target.value)}
        >
          <option value="light">Light</option>
          <option value="dark">Dark</option>
          <option value="system">Follow system</option>
        </select>
      </label>
      <p>
        Saved for this ODM profile and restored when the app opens. Follow
        system responds to your computer’s appearance setting.
      </p>
      {appearance.error && (
        <p role="alert" className="alert">
          {appearance.error}
        </p>
      )}
    </section>
  );
}
