import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faMoon, faSun } from "@fortawesome/free-solid-svg-icons";
import { useAdminTheme } from "./theme";

export function ThemeToggle() {
  const { theme, toggleTheme } = useAdminTheme();
  const nextTheme = theme === "dark" ? "light" : "dark";
  const label = `Chuyển sang giao diện ${nextTheme === "dark" ? "tối" : "sáng"}`;

  return (
    <button
      className="grid h-9 w-9 place-items-center rounded-(--admin-radius-md) border border-(--admin-border) bg-(--admin-background) text-[13px] text-(--admin-text-muted) shadow-(--admin-shadow-card) transition-colors hover:border-(--admin-primary) hover:bg-(--admin-primary-soft) hover:text-(--admin-primary) focus-visible:outline focus-visible:outline-offset-1 focus-visible:outline-(--admin-primary)"
      type="button"
      aria-label={label}
      aria-pressed={theme === "dark"}
      title={label}
      onClick={toggleTheme}
    >
      <FontAwesomeIcon
        icon={theme === "dark" ? faSun : faMoon}
        aria-hidden="true"
      />
    </button>
  );
}
