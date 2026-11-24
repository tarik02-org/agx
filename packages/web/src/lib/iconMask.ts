import type { LucideIconData } from "lucide-react";

/**
 * A lucide icon as a CSS mask image, for drawing it in currentColor where only CSS reaches, such
 * as inside a shadow root.
 */
export const iconMask = (icon: LucideIconData): string => {
  const shapes = icon.node
    .map(([tag, attributes]) => {
      const pairs = Object.entries(attributes)
        .filter(([name]) => name !== "key")
        .map(([name, value]) => `${name}="${String(value)}"`);
      return `<${tag} ${pairs.join(" ")}/>`;
    })
    .join("");
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="black" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${shapes}</svg>`;
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`;
};
