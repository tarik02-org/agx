import type { KeyboardEvent } from "react";

/** In a textarea, Enter starts a line, so ⌘/Ctrl+Enter submits its form instead. */
export const submitOnModEnter = (event: KeyboardEvent<HTMLTextAreaElement>): void => {
  if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
    event.preventDefault();
    event.currentTarget.form?.requestSubmit();
  }
};
