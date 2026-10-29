// lucide-react's per-icon modules carry the icon's SVG nodes; it declares types only for its
// entry point. The data drawn into places React does not render, such as the tree's shadow root.
declare module "lucide-react/dist/esm/icons/*.mjs" {
  import type { LucideIconData } from "lucide-react";
  // The name is lucide's.
  // oxlint-disable-next-line no-underscore-dangle
  export const __iconData: LucideIconData;
}
