import { preloadHighlighter, registerCustomTheme } from "@pierre/diffs";
import { PatchDiff } from "@pierre/diffs/react";

// The same theme as the page's other code; @pierre/diffs resolves theme names through loaders.
registerCustomTheme("github-dark-default", () => import("shiki/themes/github-dark-default.mjs"));

// A diff rendered before the shared highlighter is ready stays empty, so this module, which the
// review loads lazily, is ready only once the highlighter is. Plans touch YAML and lockfiles.
await preloadHighlighter({ themes: ["github-dark-default"], langs: ["yaml", "json"] });

/** A planned file's unified diff. */
export default function PlannedDiff({ diff }: { readonly diff: string }) {
  return (
    <PatchDiff
      patch={diff}
      disableWorkerPool
      options={{
        theme: "github-dark-default",
        themeType: "dark",
        diffStyle: "unified",
        disableFileHeader: true,
      }}
    />
  );
}
