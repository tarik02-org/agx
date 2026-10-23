import { markdown } from "@codemirror/lang-markdown";
import { yaml } from "@codemirror/lang-yaml";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { EditorView } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import CodeMirror from "@uiw/react-codemirror";
import { yamlSchema } from "codemirror-json-schema/yaml";
import type { JSONSchema7 } from "json-schema";

import { cn } from "cn";

interface CodeEditorProps {
  readonly value: string;
  readonly onChange: (value: string) => void;
  /** YAML is validated, completed, and documented on hover against the schema when given. */
  readonly language: "yaml" | "markdown" | "text";
  readonly schema?: JSONSchema7 | undefined;
  readonly readOnly?: boolean;
  /** Takes the height its flex container leaves, scrolling inside instead of growing. */
  readonly fill?: boolean;
  readonly className?: string;
}

// The editor takes the UI's palette: the page's surface and text, the accent for the cursor and
// selection, and a few calm colors for syntax, so it reads like the rest of the UI.
const theme = EditorView.theme(
  {
    "&": {
      backgroundColor: "var(--card)",
      color: "var(--foreground)",
      fontSize: "12px",
    },
    "&.cm-focused": { outline: "none" },
    ".cm-content": { fontFamily: "var(--font-mono)", caretColor: "var(--primary)" },
    ".cm-cursor, .cm-dropCursor": { borderLeftColor: "var(--primary)" },
    "&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, ::selection":
      { backgroundColor: "color-mix(in oklab, var(--primary) 30%, transparent)" },
    ".cm-activeLine": { backgroundColor: "color-mix(in oklab, var(--foreground) 4%, transparent)" },
    ".cm-gutters": {
      backgroundColor: "transparent",
      color: "var(--muted-foreground)",
      border: "none",
    },
    ".cm-activeLineGutter": { backgroundColor: "transparent", color: "var(--foreground)" },
    ".cm-tooltip": {
      fontSize: "12px",
      maxWidth: "32rem",
      backgroundColor: "var(--popover)",
      color: "var(--popover-foreground)",
      border: "1px solid var(--border)",
      borderRadius: "var(--radius)",
    },
    ".cm-tooltip-autocomplete > ul > li[aria-selected]": {
      backgroundColor: "var(--accent)",
      color: "var(--accent-foreground)",
    },
  },
  { dark: true },
);

const highlight = HighlightStyle.define([
  { tag: [tags.propertyName, tags.definition(tags.propertyName)], color: "#8f9cf3" },
  { tag: [tags.string, tags.special(tags.string)], color: "#d8b98a" },
  { tag: [tags.number, tags.bool, tags.null, tags.atom], color: "#e39bb4" },
  { tag: [tags.keyword, tags.typeName, tags.tagName, tags.labelName], color: "#6cc4a1" },
  { tag: [tags.comment, tags.meta], color: "var(--muted-foreground)", fontStyle: "italic" },
  { tag: [tags.punctuation, tags.separator, tags.bracket], color: "var(--muted-foreground)" },
  { tag: tags.heading, color: "var(--foreground)", fontWeight: "600" },
  { tag: tags.emphasis, fontStyle: "italic" },
  { tag: tags.strong, fontWeight: "600" },
  { tag: [tags.link, tags.url], color: "#8f9cf3", textDecoration: "underline" },
  { tag: tags.monospace, color: "#d8b98a" },
  { tag: tags.invalid, color: "var(--destructive)" },
]);

const languageExtensions = (language: CodeEditorProps["language"], schema?: JSONSchema7) => {
  switch (language) {
    case "yaml":
      return schema === undefined ? [yaml()] : yamlSchema(schema);
    case "markdown":
      return [markdown()];
    case "text":
      return [];
  }
};

/** CodeMirror for configuration and skill files. */
export function CodeEditor({
  value,
  onChange,
  language,
  schema,
  readOnly = false,
  fill = false,
  className,
}: CodeEditorProps) {
  return (
    <CodeMirror
      className={cn("overflow-hidden rounded-md border", fill && "min-h-0 flex-1", className)}
      {...(fill ? { height: "100%" } : {})}
      value={value}
      theme={[theme, syntaxHighlighting(highlight)]}
      editable={!readOnly}
      readOnly={readOnly}
      basicSetup={{ foldGutter: false, highlightActiveLine: !readOnly }}
      extensions={[EditorView.lineWrapping, ...languageExtensions(language, schema)]}
      onChange={onChange}
    />
  );
}
