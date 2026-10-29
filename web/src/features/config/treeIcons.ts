import claudeLogo from "@lobehub/icons-static-svg/icons/claude-color.svg?raw";
import codexLogo from "@lobehub/icons-static-svg/icons/codex-color.svg?raw";
import piLogo from "@lobehub/icons-static-svg/icons/pi.svg?raw";
import type { LucideIconData } from "lucide-react";
import { __iconData as activity } from "lucide-react/dist/esm/icons/activity.mjs";
import { __iconData as audioLines } from "lucide-react/dist/esm/icons/audio-lines.mjs";
import { __iconData as blocks } from "lucide-react/dist/esm/icons/blocks.mjs";
import { __iconData as bookOpen } from "lucide-react/dist/esm/icons/book-open.mjs";
import { __iconData as bot } from "lucide-react/dist/esm/icons/bot.mjs";
import { __iconData as boxes } from "lucide-react/dist/esm/icons/boxes.mjs";
import { __iconData as braces } from "lucide-react/dist/esm/icons/braces.mjs";
import { __iconData as brain } from "lucide-react/dist/esm/icons/brain.mjs";
import { __iconData as bug } from "lucide-react/dist/esm/icons/bug.mjs";
import { __iconData as calculator } from "lucide-react/dist/esm/icons/calculator.mjs";
import { __iconData as calendar } from "lucide-react/dist/esm/icons/calendar.mjs";
import { __iconData as clock } from "lucide-react/dist/esm/icons/clock.mjs";
import { __iconData as cloud } from "lucide-react/dist/esm/icons/cloud.mjs";
import { __iconData as code } from "lucide-react/dist/esm/icons/code.mjs";
import { __iconData as cog } from "lucide-react/dist/esm/icons/cog.mjs";
import { __iconData as container } from "lucide-react/dist/esm/icons/container.mjs";
import { __iconData as database } from "lucide-react/dist/esm/icons/database.mjs";
import { __iconData as fileCode } from "lucide-react/dist/esm/icons/file-code.mjs";
import { __iconData as fileText } from "lucide-react/dist/esm/icons/file-text.mjs";
import { __iconData as flaskConical } from "lucide-react/dist/esm/icons/flask-conical.mjs";
import { __iconData as folder } from "lucide-react/dist/esm/icons/folder.mjs";
import { __iconData as folderOpen } from "lucide-react/dist/esm/icons/folder-open.mjs";
import { __iconData as gitBranch } from "lucide-react/dist/esm/icons/git-branch.mjs";
import { __iconData as globe } from "lucide-react/dist/esm/icons/globe.mjs";
import { __iconData as image } from "lucide-react/dist/esm/icons/image.mjs";
import { __iconData as keyRound } from "lucide-react/dist/esm/icons/key-round.mjs";
import { __iconData as languages } from "lucide-react/dist/esm/icons/languages.mjs";
import { __iconData as lightbulb } from "lucide-react/dist/esm/icons/lightbulb.mjs";
import { __iconData as listTodo } from "lucide-react/dist/esm/icons/list-todo.mjs";
import { __iconData as mail } from "lucide-react/dist/esm/icons/mail.mjs";
import { __iconData as mapPin } from "lucide-react/dist/esm/icons/map-pin.mjs";
import { __iconData as messageSquare } from "lucide-react/dist/esm/icons/message-square.mjs";
import { __iconData as notebookPen } from "lucide-react/dist/esm/icons/notebook-pen.mjs";
import { __iconData as packageIcon } from "lucide-react/dist/esm/icons/package.mjs";
import { __iconData as palette } from "lucide-react/dist/esm/icons/palette.mjs";
import { __iconData as penLine } from "lucide-react/dist/esm/icons/pen-line.mjs";
import { __iconData as plug } from "lucide-react/dist/esm/icons/plug.mjs";
import { __iconData as scanEye } from "lucide-react/dist/esm/icons/scan-eye.mjs";
import { __iconData as scrollText } from "lucide-react/dist/esm/icons/scroll-text.mjs";
import { __iconData as search } from "lucide-react/dist/esm/icons/search.mjs";
import { __iconData as server } from "lucide-react/dist/esm/icons/server.mjs";
import { __iconData as shield } from "lucide-react/dist/esm/icons/shield.mjs";
import { __iconData as snowflake } from "lucide-react/dist/esm/icons/snowflake.mjs";
import { __iconData as sparkles } from "lucide-react/dist/esm/icons/sparkles.mjs";
import { __iconData as tag } from "lucide-react/dist/esm/icons/tag.mjs";
import { __iconData as terminal } from "lucide-react/dist/esm/icons/terminal.mjs";
import { __iconData as video } from "lucide-react/dist/esm/icons/video.mjs";
import { __iconData as wandSparkles } from "lucide-react/dist/esm/icons/wand-sparkles.mjs";
import { __iconData as wrench } from "lucide-react/dist/esm/icons/wrench.mjs";

import type { TreeIcon } from "~/components/NavTree.tsx";
import { iconMask } from "~/lib/iconMask.ts";

// Sections and namespaces are containers, drawn muted; definitions are drawn in their kind's
// color, so an item never reads as the folder holding it.
const containerColor = "var(--muted-foreground)";

interface KindStyle {
  readonly section: LucideIconData;
  readonly item: LucideIconData;
  readonly color: string;
  /** Whether a definition's name and description choose its icon. */
  readonly guessed: boolean;
}

const kindStyles: Readonly<Record<string, KindStyle>> = {
  skills: { section: blocks, item: wandSparkles, color: "#b4a5fb", guessed: true },
  mcp: { section: server, item: plug, color: "#86c5f9", guessed: true },
  instructions: { section: scrollText, item: scrollText, color: "#f3cd6b", guessed: true },
  secrets: { section: keyRound, item: keyRound, color: "#f6a5b4", guessed: false },
  bundles: { section: boxes, item: boxes, color: "#6fdcc6", guessed: false },
  packages: { section: packageIcon, item: packageIcon, color: "#f8b97f", guessed: false },
  harnesses: { section: bot, item: bot, color: "#bde77a", guessed: false },
};

// Words in a definition's name or description, and the icon they suggest, most specific first.
const hints: ReadonlyArray<readonly [ReadonlyArray<string>, LucideIconData]> = [
  [["test", "tdd", "spec", "qa"], flaskConical],
  [["review", "lint", "audit"], scanEye],
  [["changelog", "release", "version"], tag],
  [["git", "github", "gitlab", "commit", "branch", "pr"], gitBranch],
  [["docs", "doc", "documentation", "readme", "wiki", "guide", "manual"], bookOpen],
  [["search", "find", "grep", "lookup"], search],
  [["browser", "playwright", "puppeteer", "chrome", "web", "fetch", "http", "scrape"], globe],
  [["database", "db", "sql", "postgres", "mysql", "sqlite", "redis", "mongo"], database],
  [["slack", "chat", "discord", "message", "telegram"], messageSquare],
  [["linear", "jira", "issue", "ticket", "task", "todo"], listTodo],
  [["figma", "design", "ui", "ux", "css", "style", "theme"], palette],
  [["deploy", "cloud", "aws", "gcp", "azure", "vercel", "netlify"], cloud],
  [["docker", "container", "kubernetes", "k8s"], container],
  [["mail", "email", "gmail", "inbox"], mail],
  [["calendar", "schedule", "meeting"], calendar],
  [["filesystem", "fs", "file", "files", "folder", "directory"], folderOpen],
  [["shell", "terminal", "bash", "zsh", "cli", "command", "script"], terminal],
  [["image", "photo", "screenshot", "picture", "png"], image],
  [["pdf", "docx", "office", "spreadsheet", "excel"], fileText],
  [["memory", "knowledge", "remember", "context"], brain],
  [["time", "clock", "timer", "date"], clock],
  [["map", "maps", "location", "geo"], mapPin],
  [["translate", "translation", "i18n", "language"], languages],
  [["security", "auth", "oauth", "password", "vault"], shield],
  [["nix", "nixos", "flake"], snowflake],
  [["json", "yaml", "schema", "typescript", "javascript", "node", "npm", "pnpm"], braces],
  [["config", "settings", "setup", "rust", "cargo"], cog],
  [["log", "logs", "monitor", "metrics", "sentry", "grafana", "observability"], activity],
  [["ai", "llm", "openai", "claude", "gpt", "model", "prompt", "agent"], sparkles],
  [["write", "writing", "blog", "copy", "draft"], penLine],
  [["plan", "planning", "roadmap", "idea", "brainstorm"], lightbulb],
  [["math", "calc", "calculator"], calculator],
  [["audio", "music", "sound", "voice", "speech"], audioLines],
  [["video", "youtube", "movie"], video],
  [["notion", "obsidian", "note", "notes", "notebook"], notebookPen],
  [["debug", "bug", "error", "crash", "fail"], bug],
  [["code", "coding", "refactor", "program"], code],
];

const words = (text: string): ReadonlyArray<string> =>
  text
    .toLowerCase()
    .split(/[^a-z0-9]+/)
    .filter((word) => word !== "");

/**
 * The icon some words hint at: a word equal to a hint, or, for hints of four letters or more,
 * one starting with it, so "testing" and "databases" count.
 */
const hinted = (text: string): LucideIconData | undefined => {
  const found = words(text);
  return hints.find(([keys]) =>
    keys.some((key) =>
      found.some((word) => word === key || (key.length >= 4 && word.startsWith(key))),
    ),
  )?.[1];
};

const draw = (icon: LucideIconData, color: string): TreeIcon => ({
  mask: iconMask(icon),
  color,
});

/** A section of the tree, such as Skills. */
export const sectionIcon = (kind: string): TreeIcon | undefined => {
  const style = kindStyles[kind];
  return style === undefined ? undefined : draw(style.section, containerColor);
};

/** The section of configuration files. */
export const filesIcon: TreeIcon = draw(fileCode, containerColor);

/** A folder grouping definitions by their IDs' prefix, such as a package's skills. */
export const namespaceIcon = (isPackage: boolean): TreeIcon =>
  draw(isPackage ? packageIcon : folder, containerColor);

/**
 * A definition's icon: the image it declares, if any; otherwise guessed from its name, then its
 * description, for kinds whose names say what they do, and its kind's otherwise, drawn in the
 * brand color it declares, if any.
 */
export const definitionIcon = (
  kind: string,
  id: string,
  description: string,
  brandColor: string,
  image: string,
): TreeIcon | undefined => {
  const style = kindStyles[kind];
  if (style === undefined) {
    return undefined;
  }
  if (image !== "") {
    return { image };
  }
  const name = id.slice(id.lastIndexOf("/") + 1);
  const icon = style.guessed ? (hinted(name) ?? hinted(description) ?? style.item) : style.item;
  return draw(icon, brandColor === "" ? style.color : brandColor);
};

/** An MCP server's tool: guessed from its name, a wrench otherwise; drawn plain, under its server. */
export const toolIcon = (name: string): TreeIcon =>
  draw(hinted(name) ?? wrench, "var(--foreground)");

const svgURI = (svg: string) => `data:image/svg+xml,${encodeURIComponent(svg)}`;

// Built-in harnesses by their makers' logos: in color where the logo has its own, and in the
// text color where it is a single shape.
const harnessLogos: Readonly<Record<string, TreeIcon>> = {
  claude: { image: svgURI(claudeLogo) },
  codex: { image: svgURI(codexLogo) },
  pi: { mask: `url(${JSON.stringify(svgURI(piLogo))})`, color: "var(--foreground)" },
};

/** A harness's icon: the logo of the built-in harness it is or extends, if one is known. */
export const harnessIcon = (builtin: string): TreeIcon | undefined => harnessLogos[builtin];
