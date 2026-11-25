import claudeLogo from "@lobehub/icons-static-svg/icons/claude-color.svg?raw";
import codexLogo from "@lobehub/icons-static-svg/icons/codex-color.svg?raw";
import piLogo from "@lobehub/icons-static-svg/icons/pi.svg?raw";
import {
  Activity as activity,
  AudioLines as audioLines,
  Blocks as blocks,
  BookOpen as bookOpen,
  Bot as bot,
  Boxes as boxes,
  Braces as braces,
  Brain as brain,
  Bug as bug,
  Calculator as calculator,
  Calendar as calendar,
  Clock as clock,
  Cloud as cloud,
  Code as code,
  Cog as cog,
  Container as container,
  Database as database,
  FileCode as fileCode,
  FileText as fileText,
  FlaskConical as flaskConical,
  Folder as folder,
  FolderOpen as folderOpen,
  GitBranch as gitBranch,
  Globe as globe,
  Image as imageIcon,
  KeyRound as keyRound,
  Languages as languages,
  Lightbulb as lightbulb,
  ListTodo as listTodo,
  Mail as mail,
  MapPin as mapPin,
  MessageSquare as messageSquare,
  NotebookPen as notebookPen,
  Package as packageIcon,
  Palette as palette,
  PenLine as penLine,
  Plug as plug,
  ScanEye as scanEye,
  ScrollText as scrollText,
  Search as search,
  Server as server,
  Shield as shield,
  Snowflake as snowflake,
  Sparkles as sparkles,
  Tag as tag,
  Terminal as terminal,
  Video as video,
  WandSparkles as wandSparkles,
  Wrench as wrench,
  type IconNode,
} from "lucide";

import type { TreeIcon } from "~/components/NavTree.tsx";
import { iconMask } from "~/lib/iconMask.ts";

// Sections and namespaces are containers, drawn muted; definitions are drawn in their kind's
// color, so an item never reads as the folder holding it.
const containerColor = "var(--muted-foreground)";

interface KindStyle {
  readonly section: IconNode;
  readonly item: IconNode;
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
const hints: ReadonlyArray<readonly [ReadonlyArray<string>, IconNode]> = [
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
  [["image", "photo", "screenshot", "picture", "png"], imageIcon],
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
const hinted = (text: string): IconNode | undefined => {
  const found = words(text);
  return hints.find(([keys]) =>
    keys.some((key) =>
      found.some((word) => word === key || (key.length >= 4 && word.startsWith(key))),
    ),
  )?.[1];
};

const draw = (icon: IconNode, color: string): TreeIcon => ({
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
