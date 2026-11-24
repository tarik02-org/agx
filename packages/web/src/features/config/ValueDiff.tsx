import { preloadHighlighter, registerCustomTheme } from "@pierre/diffs";
import { MultiFileDiff } from "@pierre/diffs/react";

// The same theme as the page's other code; @pierre/diffs resolves theme names through loaders.
registerCustomTheme("github-dark-default", () => import("shiki/themes/github-dark-default.mjs"));

// A diff rendered before the shared highlighter is ready stays empty, so this module, which the
// review loads lazily, is ready only once the highlighter is.
await preloadHighlighter({ themes: ["github-dark-default"], langs: ["yaml"] });

/** A definition's value before and after a change, as YAML. */
export default function ValueDiff({
  name,
  before,
  after,
}: {
  readonly name: string;
  readonly before: string;
  readonly after: string;
}) {
  return (
    <MultiFileDiff
      oldFile={{ name, contents: before, lang: "yaml" }}
      newFile={{ name, contents: after, lang: "yaml" }}
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
