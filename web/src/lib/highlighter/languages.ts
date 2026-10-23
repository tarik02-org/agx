import type { DynamicImportLanguageRegistration } from "shiki/core";

// The grammars the UI highlights, by name and by the aliases and file extensions they go by:
// configuration, the languages skills write scripts in, and what their Markdown fences use.
// Shiki's own catalog has over 300, and the daemon binary embeds whatever the build emits; C++,
// or Ruby, which embeds it, would add 800 kB.
// The build serves this module for `shiki/langs`, so @pierre/diffs resolves from it too.

const bat = () => import("shiki/langs/bat.mjs");
const c = () => import("shiki/langs/c.mjs");
const csharp = () => import("shiki/langs/csharp.mjs");
const css = () => import("shiki/langs/css.mjs");
const diff = () => import("shiki/langs/diff.mjs");
const dockerfile = () => import("shiki/langs/dockerfile.mjs");
const go = () => import("shiki/langs/go.mjs");
const html = () => import("shiki/langs/html.mjs");
const ini = () => import("shiki/langs/ini.mjs");
const java = () => import("shiki/langs/java.mjs");
const javascript = () => import("shiki/langs/javascript.mjs");
const json = () => import("shiki/langs/json.mjs");
const json5 = () => import("shiki/langs/json5.mjs");
const jsonc = () => import("shiki/langs/jsonc.mjs");
const jsx = () => import("shiki/langs/jsx.mjs");
const kotlin = () => import("shiki/langs/kotlin.mjs");
const lua = () => import("shiki/langs/lua.mjs");
const make = () => import("shiki/langs/make.mjs");
const markdown = () => import("shiki/langs/markdown.mjs");
const nix = () => import("shiki/langs/nix.mjs");
const php = () => import("shiki/langs/php.mjs");
const powershell = () => import("shiki/langs/powershell.mjs");
const python = () => import("shiki/langs/python.mjs");
const rust = () => import("shiki/langs/rust.mjs");
const shellscript = () => import("shiki/langs/shellscript.mjs");
const sql = () => import("shiki/langs/sql.mjs");
const swift = () => import("shiki/langs/swift.mjs");
const toml = () => import("shiki/langs/toml.mjs");
const tsx = () => import("shiki/langs/tsx.mjs");
const typescript = () => import("shiki/langs/typescript.mjs");
const xml = () => import("shiki/langs/xml.mjs");
const yaml = () => import("shiki/langs/yaml.mjs");

export const bundledLanguages: Readonly<Record<string, DynamicImportLanguageRegistration>> = {
  bat,
  batch: bat,
  cmd: bat,
  c,
  h: c,
  csharp,
  "c#": csharp,
  cs: csharp,
  css,
  diff,
  patch: diff,
  dockerfile,
  go,
  html,
  htm: html,
  ini,
  properties: ini,
  java,
  javascript,
  js: javascript,
  cjs: javascript,
  mjs: javascript,
  json,
  json5,
  jsonc,
  jsx,
  kotlin,
  kt: kotlin,
  kts: kotlin,
  lua,
  make,
  makefile: make,
  markdown,
  md: markdown,
  nix,
  php,
  powershell,
  ps: powershell,
  ps1: powershell,
  pwsh: powershell,
  python,
  py: python,
  rust,
  rs: rust,
  shellscript,
  bash: shellscript,
  sh: shellscript,
  shell: shellscript,
  zsh: shellscript,
  sql,
  swift,
  toml,
  tsx,
  typescript,
  ts: typescript,
  cts: typescript,
  mts: typescript,
  xml,
  svg: xml,
  yaml,
  yml: yaml,
};
