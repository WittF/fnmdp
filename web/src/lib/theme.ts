// Theme toggle for the Vite (non-Next) app. Keeps <html> class + data-theme in
// sync and persists the choice. Default is dark (monitoring station, big
// screens). The initial value is set by an inline anti-flash script in
// index.html before React mounts, so this module just mirrors/updates it.
import { atom } from "nanostores";

export type Theme = "light" | "dark";

const KEY = "fnmdp.theme";

function read(): Theme {
  try {
    const v = localStorage.getItem(KEY);
    if (v === "light" || v === "dark") return v;
  } catch {
    // ignore (private mode / SSR)
  }
  return "dark";
}

export const $theme = atom<Theme>(read());

export function applyTheme(theme: Theme): void {
  const el = document.documentElement;
  el.classList.toggle("dark", theme === "dark");
  el.classList.toggle("light", theme === "light");
  el.setAttribute("data-theme", theme);
}

export function setTheme(theme: Theme): void {
  $theme.set(theme);
  applyTheme(theme);
  try {
    localStorage.setItem(KEY, theme);
  } catch {
    // ignore
  }
}

export function toggleTheme(): void {
  setTheme($theme.get() === "dark" ? "light" : "dark");
}
