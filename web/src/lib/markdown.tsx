import { type ReactNode, useEffect, useState } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import type { BundledLanguage } from "shiki";

// Shiki loads its grammars on first use, so pages that render no code never fetch it.
const highlighter = () => import("shiki");

/** Code highlighted with Shiki once it loads, plain until then. */
export function Code({ code, language }: { readonly code: string; readonly language: string }) {
  const [html, setHtml] = useState<string | null>(null);
  useEffect(() => {
    let current = true;
    void highlighter()
      .then(({ codeToHtml, bundledLanguages }) =>
        codeToHtml(code, {
          lang: language in bundledLanguages ? (language as BundledLanguage) : "text",
          theme: "github-dark-default",
        }),
      )
      .then((rendered) => {
        if (current) {
          setHtml(rendered);
        }
      });
    return () => {
      current = false;
    };
  }, [code, language]);
  if (html === null) {
    return <pre className="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs">{code}</pre>;
  }
  // Shiki escapes the code it renders.
  return (
    <div
      className="overflow-x-auto rounded-md text-xs [&_pre]:p-3"
      // oxlint-disable-next-line react/no-danger
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}

// Code spans stay inline; fenced blocks with a language are highlighted.
const markdownComponents: Components = {
  code: ({ className, children }): ReactNode => {
    const language = /language-(\S+)/.exec(className ?? "")?.[1];
    const text = String(children).replace(/\n$/, "");
    return language === undefined ? (
      <code className="rounded-sm bg-muted px-1">{text}</code>
    ) : (
      <Code code={text} language={language} />
    );
  },
  pre: ({ children }) => <>{children}</>,
};

/** GitHub-flavored Markdown, as skills and MCP tools write it, with highlighted code blocks. */
export function Markdown({ children }: { readonly children: string }) {
  return (
    <div className="flex flex-col gap-2 text-sm/relaxed [&_a]:text-primary [&_a]:underline [&_code]:font-mono [&_h1]:font-semibold [&_h1]:text-base [&_h2]:font-semibold [&_h3]:font-medium [&_li]:ml-4 [&_ol]:list-decimal [&_table]:w-auto [&_table]:max-w-full [&_table]:border-collapse [&_table]:text-xs [&_td]:border [&_td]:px-2.5 [&_td]:py-1 [&_th]:border [&_th]:bg-muted/50 [&_th]:px-2.5 [&_th]:py-1 [&_th]:text-left [&_th]:font-medium [&_ul]:list-disc">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
        {children}
      </ReactMarkdown>
    </div>
  );
}
