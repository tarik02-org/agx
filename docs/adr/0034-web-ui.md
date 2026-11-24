# Serve a web UI from the daemon

Status: accepted.

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
in `packages/web` of this repository's pnpm workspace.

### Why in the daemon, and why a browser

- The daemon already holds the runtime state the UI shows: launches, MCP
  endpoints, shared processes, secrets, OAuth logins, installations. It also
  streams that state (`WatchRuntime`). A UI elsewhere would have to proxy all of
  it.
- A browser needs nothing installed. It works the same on macOS, Linux, and
  Windows, and on a remote machine through `ssh -L <port>:127.0.0.1:<port>`.
- The protocol is already shared: `buf generate` writes the TypeScript messages
  that `agx-client` exports, so the UI calls typed RPCs without a second API.
- What the UI shows is tables, trees, forms, Markdown, diffs, and graphs. Web
  libraries cover all of these.

## Access

The daemon listens on `127.0.0.1` only, and the UI changes nothing about that.
Any local process can reach the port, so the UI is authorized like the CLI,
with the management credential, delivered without ever putting it in a URL:

1. `agx ui` asks the daemon (with the management token) for a one-time code,
   valid for 60 seconds, and opens `/ui/login?code=<code>`.
2. The daemon exchanges the code for a session token and redirects to
   `/ui/#session=<token>`, without the code. Browsers never send the fragment
   to a server; the page moves the token into its `localStorage`, which is
   kept per origin and so per port, drops it from the address, and sends it as
   `Authorization: Bearer`.
   - Not a cookie: browsers scope cookies by host, not port, so a cookie on
     `127.0.0.1` would reach every other local server the browser visits, and
     two daemons on different ports would overwrite each other's.
   - It expires with the daemon or after 12 hours of inactivity. Sessions live
     in the daemon's memory only: after a daemon restart the UI asks to run
     `agx ui` again.
   - A session cannot create login codes, which take the management token
     only, so it cannot extend itself past its idle limit.
3. Requests carrying a session token are authorized like the management token.
   The checks:
   - `Host` must be `127.0.0.1:<port>` or `localhost:<port>`, which stops DNS
     rebinding;
   - `Origin`, when present, must be the same origin;
   - Connect requests from the browser need `Connect-Protocol-Version` and a
     JSON or protobuf content type. A cross-origin page cannot send those
     without a CORS preflight, which the daemon never approves.
4. Launch grants never authorize the UI. A harness process cannot open it, and
   UI sessions are separate from launch leases.

`agx ui --print` prints the login URL instead of opening a browser, for use
through an SSH tunnel. The code still expires after one use or 60 seconds.
Connecting to a remote daemon directly is out of scope.

## Frontend stack

| Concern              | Choice                                                                                                                                                                                                                                  |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Language             | TypeScript 7 (the native compiler), `strict` plus `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, `verbatimModuleSyntax`                                                                                                      |
| Toolchain            | Vite+ 1 (`vp dev`, `vp build`, `vp lint` with Oxlint, `vp fmt` with Oxfmt, `vp test` with Vitest), lint and format configs strict, CI failing on warnings                                                                               |
| Package manager      | pnpm, `packages/web` (`agx-web`) as a member of the repository's workspace, with its one committed lockfile                                                                                                                             |
| UI                   | React 19 with the React Compiler, through `@vitejs/plugin-react` with `compiler: true` (the Oxc-based Rust implementation, as T3 Code uses)                                                                                             |
| Routing              | TanStack Router, file-based, with code splitting                                                                                                                                                                                        |
| Components           | shadcn on Base UI, Tailwind CSS 4, a compact dark theme (light as an option), lucide icons                                                                                                                                              |
| RPC                  | `@connectrpc/connect-web` over the generated messages `agx-client/protocol` exports                                                                                                                                                     |
| State and effects    | Effect 4 in a `ManagedRuntime` with `useFork`-style hooks, TanStack Query for cached queries (see below)                                                                                                                                |
| Markdown             | `react-markdown` 10 with `remark-gfm`, code blocks highlighted with Shiki, as T3 Code renders chat and pull request Markdown                                                                                                            |
| Diffs and file trees | `@pierre/diffs` (also Shiki highlighting) and `@pierre/trees`                                                                                                                                                                           |
| Harness logos        | `@lobehub/icons-static-svg` (MIT), as SVG files                                                                                                                                                                                         |
| Editors              | forms for definitions and scopes (below); CodeMirror 6 for skill files and as the JSON fallback, validated and completed against the configuration JSON Schema the daemon serves at `/ui/schema.json` for its own version and harnesses |
| Local state          | Zustand for the staged changes, which live in the tab until applied                                                                                                                                                                     |
| Graphs               | React Flow (`@xyflow/react`), only for the scope view (below)                                                                                                                                                                           |

### Effect

The UI uses Effect for its data layer, not for rendering:

- Every RPC goes through a small Effect service around the Connect client. Its
  failures become a tagged error union classified from the Connect code
  (`conflict`, `not_found`, …), with `config_invalid` for an error carrying a
  configuration `ErrorDetail`, and `needs_login` and `mcp_failed` for an MCP
  server's failures. Components then handle each reason exhaustively instead
  of catching `ConnectError`.
- Streams (`WatchRuntime`, `Install`, `Login`) are Effect streams whose
  scope ends with the component that runs them, so leaving a page closes the
  stream, and a reconnect after `agx daemon restart` is one retry schedule.
- Data the UI reads outside generated messages, such as tool input schemas and
  `structured_content` of tool results, is decoded with `Schema` at the
  boundary.
- One `ManagedRuntime` built from the service layers lives as long as the page
  and reaches components through React context, with hooks taken from
  Aperture's `session-react`:
  - `useFork` runs an effect while a component is mounted, for subscriptions
    such as the runtime stream;
  - `useEffectCallback` forks one from an event handler and interrupts it on
    unmount, for actions;
  - `useRunEffect` turns one into a promise for TanStack Query, which caches
    queries and passes its abort signal on as interruption.

The cost is bundle size and an API to learn. Both are acceptable for a local
tool whose author already writes Effect, and Aperture uses the same model.

## Build and distribution

- `packages/web` builds into `internal/ui/dist`, which `internal/ui` embeds
  with `//go:embed all:dist`.
- A checked-in placeholder `index.html` keeps `go build` and `go install`
  working without Node: `agx ui` then says the binary was built without the
  UI.
- The Nix package builds the UI first, with pnpm's fixed-output fetch of the
  dependencies of `agx-web` and the packages it uses, and a `pnpmDeps` hash
  next to `vendorHash`. GoReleaser runs the same
  build in a `before` hook. Release binaries and the flake always contain the
  UI.
- CI checks the workspace (`vp check`: lint, format, and types), runs its
  tests, and builds the UI.

## Layout and scope

- Each page has one header, one row as tall as the sidebar's, with the
  sidebar's toggle, the breadcrumbs, the page's views, and every action, an
  editor's Stage and Revert included, then the staged changes, so actions are
  always at the top. What does not act (where a definition is, Form or JSON)
  sits with the content. The sidebar's top row holds the runtime page and New, and
  the scope (directory and harness) sits below it, above the tree it marks; in
  the header
  they crowded the actions until the row wrapped, and a second row read as a
  second header.
- The sidebar, the same on every page, is the configuration as a tree, not a
  list of pages: definitions ungrouped, by the bundles selecting them, or by the
  file defining them, each split by kind or not, package skills under their
  package, and the files. Skills hold their files and MCP servers their tools, loaded when
  first expanded, so a server starts only when someone looks inside it.
  Selecting an entry opens its page, with an overview of what it is and does
  and its definition on a second tab; there are no pages per kind, since each
  would need an item picked anyway, and pages keep no navigation of their own.
- The tree shows all of the configuration, marking what is effective in the
  scope directory, or only that, for any harness or the one picked.
  This replaces a separate explain page: why an item is or is not effective is
  on the item's overview, and a harness's resolved settings on the harness's.
- The scope is a directory, or everywhere, kept in the address (`?dir=`) that
  every link carries. `agx ui` passes its working directory with the sign-in
  code, and the sign-in redirect keeps it only as a query value of the UI's own
  address. Explain resolves the scope directory, the MCP inspector starts
  servers in it, the tree and the skills page mark or filter what it gets, and
  enabling starts there.

## Features

Each feature is a page, listed in the order they ship.

1. **Runtime**
   - Daemon state, version, and restart. The UI reconnects by itself after a
     restart.
   - Launches and thread launches.
   - MCP processes as `agx mcp ps` lists them, with restart and stop.
   - Secrets with their state, and OAuth login and logout.
   - Package installation progress (`Install`).
2. **Explain**
   - Pick a directory: recent launch directories, scope directories from the
     configuration, or a path typed with completion from the daemon.
   - See every resource with its traces, as `agx explain` shows them.
   - A harness selector adds harness rules and the resolved
     [settings](0025-harness-settings.md).
   - Each source location opens the configuration file at that line in the
     editor page.
3. **MCP inspector**
   - A server's state, description, and tools, as `agx mcp tools <id> --schema`
     reports them, listed when it is expanded in the tree.
   - A form generated from a tool's input schema, or raw JSON.
   - The call goes through `CallTool`, shown with:
     - text and Markdown content;
     - images;
     - resource links;
     - `structured_content` as a tree;
     - errors by their reason.
   - A call history kept in the browser.
4. **Skills and packages**
   - **Installed skills:** rendered `SKILL.md` and file previews; the files
     are in the sidebar tree, grouped by package.
   - **Add from Git:** enter a repository URL or `owner/repo`. The daemon
     clones it into a temporary directory and lists the skills it finds, the
     way `agx skill add` does. For each skill it shows the description, the
     rendered `SKILL.md`, and the files.
   - **Selecting skills:** choose skills and a scope (everywhere, a directory,
     a harness), review the changes, then confirm.
   - **Packages:** status, lock, `agx install`, and `agx update` with progress.
5. **Configuration editor**
   - Configuration is edited through the interface, not as text:
     - create, edit, and delete skills, instructions, MCP servers, secrets,
       bundles, harness definitions, packages, and scope rules, in forms built
       for each kind;
     - scope rules are edited as lists of resources per directory and harness,
       with resources picked from what the configuration defines.
   - The UI edits local configuration, which the daemon keeps in its database
     ([ADR 0029](0029-local-configuration.md)), with the same semantic changes
     as the CLI's editing commands. Definitions that configuration files or
     packages define open read-only, with the file and line they come from.
   - Changes are staged, not written one by one. The page shows each pending
     definition's value before and after, and applies the set atomically with
     `UpdateConfiguration`:
     - the daemon validates the configuration the changes produce, and commits
       nothing if anything fails;
     - apply fails if the local configuration's revision moved since the
       review, such as through a CLI edit; the staged changes, being semantic,
       are reviewed against what is there now and applied again.
   - A definition whose fields no form models yet, such as an unusual MCP
     `auth` or harness `settings`, opens in a JSON editor for that one
     definition's value (CodeMirror with the agx JSON Schema), staged like a
     form. Injected values are written there as `{"$env": …}`, `{"$file": …}`,
     or `{"$secret": …}`.
   - Local skill files are edited in CodeMirror with Markdown support and saved
     with `WriteResourceFile`; they are not configuration, so they are not
     staged.
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

| RPC                                                  | For                                                                                                                                                          |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `UIService.CreateLoginCode`                          | `agx ui`                                                                                                                                                     |
| `ConfigurationService.GetConfiguration`              | the configuration as loaded: harnesses, scope directories, overlays, and the local configuration's revision                                                  |
| `ConfigurationService.UpdateConfiguration`           | the editor: commit a set of definition, scope, and package changes atomically if the local configuration is still at the revision the UI reviewed            |
| `PackageService.PreviewSource`                       | clone a Git source into a temporary directory under the cache, list its skills with `SKILL.md` and file trees, read a file; previews expire after 30 minutes |
| `PackageService.AddSkills`                           | the selection of the preview, written as `agx skill add` would, through the same edit path                                                                   |
| `RuntimeService.RestartMcpProcess`, `StopMcpProcess` | process controls; a shared process restarts for all its clients                                                                                              |
| `RuntimeService.ListSessions`                        | sessions as the launcher lists them, moved from the CLI to the daemon                                                                                        |
| `ConfigurationService.CompleteDirectory`             | directory completion in the explain page                                                                                                                     |

The configuration edits the CLI makes (`skill add`, `mcp add`, `enable`, …)
move from `internal/cli` into a package the daemon also uses, so CLI and UI
make identical changes.

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

1. `packages/web` scaffold:
   - toolchain configs, theme, shell layout, and the Effect RPC layer;
   - login codes and sessions, embedding, and the Nix and GoReleaser
     builds;
   - CI for all of it.

   3 days.

2. Runtime page and process controls: 2 days.
3. Explain page, `GetConfiguration`, and directory completion: 2 days.
4. MCP inspector with schema forms and content rendering: 3 days.
5. Skills browser, Git preview, and adding skills, with edits moved into a
   shared package: 4 days.
6. Configuration editor with staged changes: 3 days.
7. Scope graph, if wanted: 2 days.
