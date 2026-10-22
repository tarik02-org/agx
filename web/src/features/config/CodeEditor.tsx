import { markdown } from "@codemirror/lang-markdown";
import { yaml } from "@codemirror/lang-yaml";
import { EditorView } from "@codemirror/view";
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
  readonly className?: string;
}

// The editor takes the page's colors, so it sits in a card like any other control.
const theme = EditorView.theme(
  {
    "&": { backgroundColor: "transparent", fontSize: "12px" },
    ".cm-gutters": { backgroundColor: "transparent", border: "none" },
    ".cm-content": { fontFamily: "var(--font-mono)" },
    ".cm-tooltip": { fontSize: "12px", maxWidth: "32rem" },
  },
  { dark: true },
);

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
  className,
}: CodeEditorProps) {
  return (
    <CodeMirror
      className={cn("overflow-hidden rounded-md border", className)}
      value={value}
      theme="dark"
      editable={!readOnly}
      readOnly={readOnly}
      basicSetup={{ foldGutter: false, highlightActiveLine: !readOnly }}
      extensions={[theme, EditorView.lineWrapping, ...languageExtensions(language, schema)]}
      onChange={onChange}
    />
  );
}
