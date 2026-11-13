import { useStore } from "@nanostores/react";
import { Button } from "@heroui/react";
import { $theme, toggleTheme } from "../lib/theme";

// Sun / moon inline SVGs (no icon dependency) — beszel-style minimal toggle.
function Sun() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
    </svg>
  );
}

function Moon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
    </svg>
  );
}

export function ThemeToggle() {
  const theme = useStore($theme);
  return (
    <Button
      isIconOnly
      variant="ghost"
      size="sm"
      onPress={toggleTheme}
      aria-label={theme === "dark" ? "切换到浅色" : "切换到深色"}
    >
      {theme === "dark" ? <Moon /> : <Sun />}
    </Button>
  );
}
