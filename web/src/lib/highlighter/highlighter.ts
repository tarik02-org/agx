import { createHighlighterCore, type HighlighterCore } from "shiki/core";
import { createJavaScriptRegexEngine } from "shiki/engine/javascript";
import githubDarkDefault from "shiki/themes/github-dark-default.mjs";
import { bundledLanguages } from "~/lib/highlighter/languages.ts";

// The JavaScript engine spares the 600 kB Oniguruma WASM build; it covers the grammars above.
let highlighter: Promise<HighlighterCore> | undefined;

/** Code as highlighted HTML, as plain text when its language has no grammar here. */
export async function highlight(code: string, language: string): Promise<string> {
  highlighter ??= createHighlighterCore({
    themes: [githubDarkDefault],
    langs: [],
    engine: createJavaScriptRegexEngine(),
  });
  const loaded = await highlighter;
  const grammar = bundledLanguages[language];
  const registrations = grammar === undefined ? [] : (await grammar()).default;
  await loaded.loadLanguage(...registrations);
  // A module lists the grammars it embeds first and its own last, whose name also stands for the
  // extensions only this catalog knows, such as `h`.
  const lang = registrations.at(-1)?.name ?? "text";
  return loaded.codeToHtml(code, { lang, theme: "github-dark-default" });
}
