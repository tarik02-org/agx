import type { JSONSchema7 } from "json-schema";
import { lazy, Suspense } from "react";

import { cn } from "cn";

export type EditorLanguage = "yaml" | "markdown" | "text";

export interface CodeEditorProps {
  readonly value: string;
  readonly onChange: (value: string) => void;
  /** YAML is validated, completed, and documented on hover against the schema when given. */
  readonly language: EditorLanguage;
  readonly schema?: JSONSchema7 | undefined;
  readonly readOnly?: boolean;
  /** Takes the height its flex container leaves, scrolling inside instead of growing. */
  readonly fill?: boolean;
  readonly className?: string;
}

// CodeMirror and its schema support weigh more than the rest of the page, which mostly shows forms.
const CodeMirrorEditor = lazy(() => import("~/features/config/CodeMirrorEditor.tsx"));

/** CodeMirror for configuration and skill files, showing the text as it is until it loads. */
export function CodeEditor(props: CodeEditorProps) {
  const fallback = (
    <pre
      className={cn(
        "overflow-auto rounded-md border bg-card p-1 pl-8 font-mono text-xs/[1.4] whitespace-pre-wrap",
        props.fill === true && "min-h-0 flex-1",
        props.className,
      )}
    >
      {props.value}
    </pre>
  );
  return (
    <Suspense fallback={fallback}>
      <CodeMirrorEditor {...props} />
    </Suspense>
  );
}
