# Serve a web UI from the daemon

Status: proposed.

agx is driven from the command line, plus the TUI launcher. That suits scripts
and quick edits, but some questions are hard to answer from a terminal:
- what a directory ends up with, and which rule decided each item;
- which MCP servers run, which wait for a login, and what their tools do;
- what a Git repository's skills contain before installing them.

A browser UI served by the daemon answers these. It uses the Connect services
the CLI already uses, and adds the few operations it needs.

## Decision

`agx ui` opens `http://127.0.0.1:<daemon port>/ui/` in the default browser. The
daemon serves the UI's static files from its binary and answers the UI's
requests through the same Connect handlers as the CLI. There is no separate
server process and no second port. The UI is a single-page React application,
in `web/` of this repository.

### Why in the daemon, and why a browser

- The daemon already holds the runtime state the UI shows: launches, MCP
  endpoints, shared processes, secrets, OAuth logins, installations. It also
  streams that state (`WatchRuntime`). A UI elsewhere would have to proxy all of
  it.
- A browser needs nothing installed. It works the same on macOS, Linux, and
  Windows, and on a remote machine through `ssh -L <port>:127.0.0.1:<port>`.
- The protocol is already shared: `buf generate` writes a TypeScript client
  (`gen/typescript`), so the UI calls typed RPCs without a second API.
- What the UI shows is tables, trees, forms, Markdown, diffs, and graphs. Web
  libraries cover all of these.

## Access

The daemon listens on `127.0.0.1` only, and the UI changes nothing about that.
Any local process can reach the port, so the UI is authorized like the CLI,
with the management credential, delivered without ever putting it in a URL:

1. `agx ui` asks the daemon (with the management token) for a one-time code,
   valid for 60 seconds, and opens `/ui/login?code=<code>`.
2. The daemon exchanges the code for a session cookie:
   - `HttpOnly`, `SameSite=Strict`, path `/`, no `Secure` flag, since the
     connection is plain HTTP on loopback;
   - it expires with the daemon or after 12 hours of inactivity.

   The browser is then redirected to `/ui/` without the code.
3. Requests carrying that cookie are authorized like the management token.
   The checks:
   - `Host` must be `127.0.0.1:<port>` or `localhost:<port>`, which stops DNS
     rebinding;
   - `Origin`, when present, must be the same origin;
   - Connect requests from the browser need `Connect-Protocol-Version` and a
     JSON or protobuf content type. A cross-origin page cannot send those
     without a CORS preflight, which the daemon never approves.
4. Launch grants never authorize the UI. A harness process cannot open it, and
   UI sessions are separate from launch leases.

`agx ui --print` prints the login URL instead of opening a browser, for remote
use. The code still expires after one use or 60 seconds.

## Frontend stack

| Concern | Choice |
| --- | --- |
| Language | TypeScript 7 (the native compiler), `strict` plus `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, `verbatimModuleSyntax` |
| Toolchain | Vite+ 1 (`vp dev`, `vp build`, `vp lint` with Oxlint, `vp fmt` with Oxfmt, `vp test` with Vitest), lint and format configs strict, CI failing on warnings |
| Package manager | pnpm 12, lockfile committed, `web/` as its own package |
| UI | React 19 with the React Compiler, through `@vitejs/plugin-react` with `compiler: true` (the Oxc-based Rust implementation, as T3 Code uses) |
| Routing | TanStack Router, file-based, with code splitting |
| Components | shadcn on Base UI, Tailwind CSS 4, a compact dark theme (light as an option), lucide icons |
| RPC | `@connectrpc/connect-web` over the generated `gen/typescript` messages |
| State and effects | Effect 4 with `@effect/atom-react` (see below) |
| Markdown | `react-markdown` 10 with `remark-gfm`, code blocks highlighted with Shiki, as T3 Code renders chat and pull request Markdown |
| Diffs and file trees | `@pierre/diffs` (also Shiki highlighting) and `@pierre/trees` |
| Editors | CodeMirror 6: YAML with the agx JSON Schema (`schema/agx.schema.json`) for completion and diagnostics, Markdown for skills |
| Graphs | React Flow (`@xyflow/react`), only for the scope view (below) |

### Effect

The UI uses Effect for its data layer, not for rendering:

- Every RPC goes through a small Effect service around the Connect client. Its
  failures become a tagged error union decoded from `ErrorDetail.reason`
  (`needs_login`, `mcp_failed`, `config_invalid`, …), and an unknown reason is
  a defect. Components then handle each reason exhaustively instead of
  catching `ConnectError`.
- Streams (`WatchRuntime`, `WatchOperation`, `Login`) become Effect streams.
  Their scope ends with the atom that subscribes, so leaving a page closes the
  stream, and a reconnect after `agx daemon restart` is one retry schedule.
- Data the UI reads outside generated messages, such as tool input schemas and
  `structured_content` of tool results, is decoded with `Schema` at the
  boundary.
- `@effect/atom-react` holds query and stream state, so components read atoms
  rather than managing `useEffect` lifetimes.

The cost is bundle size and an API to learn. Both are acceptable for a local
tool whose author already writes Effect, and T3 Code uses the same combination.
Plain React Query with hand-written error mapping was the alternative; it would
leave error handling and stream lifetimes unchecked.

## Build and distribution

- `web/` builds to `web/dist`. `internal/ui` embeds it with
  `//go:embed all:dist` (copied there by the build).
- A checked-in placeholder `index.html` keeps `go build` and `go install`
  working without Node: `agx ui` then says the binary was built without the
  UI.
- The Nix package builds the UI first, with pnpm's fixed-output dependency
  fetch and a `pnpmDeps` hash next to `vendorHash`. GoReleaser runs the same
  build in a `before` hook. Release binaries and the flake always contain the
  UI.
- CI runs `vp lint`, `vp fmt --check`, the TypeScript check, and `vp test`, and
  builds the UI before the Go tests that serve it.

## Features

Each feature is a page, listed in the order they ship.

1. **Runtime**
   - Daemon state, version, and restart. The UI reconnects by itself after a
     restart.
   - Launches and thread launches.
   - MCP processes as `agx mcp ps` lists them, with restart and stop.
   - Secrets with their state, and OAuth login and logout.
   - Package installation progress (`WatchOperation`).
2. **Explain**
   - Pick a directory: recent launch directories, scope directories from the
     configuration, or a path typed with completion from the daemon.
   - See every resource with its traces, as `agx explain` shows them.
   - A harness selector adds harness rules and the resolved
     [settings](0025-harness-settings.md).
   - Each source location opens the configuration file at that line in the
     editor page.
3. **MCP inspector**
   - Every configured server with its state, description, and tools, as
     `agx mcp tools --all --schema` reports them.
   - A form generated from a tool's input schema, or raw JSON.
   - The call goes through `CallTool`, shown with:
     - text and Markdown content;
     - images;
     - resource links;
     - `structured_content` as a tree;
     - errors by their reason.
   - A call history kept in the browser.
4. **Skills and packages**
   - **Installed skills:** rendered `SKILL.md` and a file tree with file
     previews, grouped by package.
   - **Add from Git:** enter a repository URL or `owner/repo`. The daemon
     clones it into a temporary directory and lists the skills it finds, the
     way `agx skill add` does. For each skill it shows the description, the
     rendered `SKILL.md`, and the files.
   - **Selecting skills:** choose skills and a scope (everywhere, a directory,
     a harness), see the configuration diff, then confirm.
   - **Packages:** status, lock, `agx install`, and `agx update` with progress.
5. **Configuration editor**
   - Configuration fragments and local skills are opened in CodeMirror. YAML
     gets schema completion and diagnostics as you type.
   - On save, the daemon validates the whole configuration and returns a diff;
     nothing is written until you confirm.
   - Files under `/nix/store` and other read-only paths open read-only, with a
     note on where they come from.
6. **Scope graph** (optional)
   - A React Flow view: scopes and harnesses on the left, bundles in the
     middle, and resources on the right.
   - Edges are colored add or remove, for one directory or the whole
     configuration.
   - It ships only if the explain page leaves questions it answers better.

The UI does not start harness sessions. A browser tab cannot host a terminal
harness the way the launcher does. Starting a session, which would mean a
terminal emulator, is left to the launcher and integrations such as T3 Code.

## Daemon additions

Most pages use existing RPCs. These are new, all requiring management
authorization:

| RPC | For |
| --- | --- |
| `UIService.CreateLoginCode` | `agx ui` |
| `ConfigurationService.GetConfiguration` | the configuration as loaded: definitions, scopes, packages, files with read-only marks; the same model `--json` list commands print |
| `ConfigurationService.ReadFile`, `PlanEdit`, `ApplyEdit` | the editor: read a fragment or local skill file, validate a change and return its diff, and write it if the file has not changed since |
| `PackageService.PreviewSource` | clone a Git source into a temporary directory under the cache, list its skills with `SKILL.md` and file trees, read a file; previews expire after 30 minutes |
| `PackageService.AddSkills` | the selection of the preview, written as `agx skill add` would, through the same edit path |
| `RuntimeService.RestartMcpProcess`, `StopMcpProcess` | process controls; a shared process restarts for all its clients |
| `RuntimeService.ListSessions` | sessions as the launcher lists them, moved from the CLI to the daemon |
| `ConfigurationService.CompleteDirectory` | directory completion in the explain page |

The configuration edits the CLI makes (`skill add`, `mcp add`, `enable`, …)
move from `internal/cli` into a package the daemon also uses. CLI and UI then
write identical YAML and show identical diffs.

## Alternatives

- **A native app in Rust with GPUI:** a second language and toolchain, a Rust
  Connect client generated separately, and a young framework built around Zed
  whose Windows support came late. Nothing on these pages needs native
  rendering.
- **Tauri or Electron:** a separate install and update channel for what a
  browser tab does. A desktop wrapper can still load `/ui/` later.
- **A separate `agx ui` server process:** another port and another credential
  path, plus proxying every RPC and stream to the daemon.
- **Growing the TUI:** Markdown rendering, diffs, forms from JSON Schema, and
  graphs are far harder in a terminal, and the launcher should stay a quick
  picker.
- **No Effect, with React Query:** discussed above.

## Effort

About three weeks, shippable after each step:

1. `web/` scaffold:
   - toolchain configs, theme, shell layout, and the Effect RPC layer;
   - login codes and cookie sessions, embedding, and the Nix and GoReleaser
     builds;
   - CI for all of it.

   3 days.
2. Runtime page and process controls: 2 days.
3. Explain page, `GetConfiguration`, and directory completion: 2 days.
4. MCP inspector with schema forms and content rendering: 3 days.
5. Skills browser, Git preview, and adding skills, with edits moved into a
   shared package: 4 days.
6. Configuration editor with plan and apply: 3 days.
7. Scope graph, if wanted: 2 days.

## Open questions

- Whether the editor should edit only what agx can rewrite structurally (one
  definition at a time, as the CLI does), or whole files. Whole files keep
  comments and layout, but conflict with concurrent CLI edits.
- How the UI shows a configuration that Nix or Home Manager owns: read-only
  with the store path, or with a hint to change the Nix source instead.
- Whether a browser tab may keep a session across daemon restarts. That needs
  the session store persisted next to the daemon state, or `agx ui` run again.
- Whether to support a remote daemon directly, beyond an SSH tunnel and
  `--print`, for example a UI that lists several machines' daemons.
