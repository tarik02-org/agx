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
   - it expires with the daemon or after 12 hours of inactivity. Sessions live
     in the daemon's memory only: after a daemon restart the UI asks to run
     `agx ui` again.

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

`agx ui --print` prints the login URL instead of opening a browser, for use
through an SSH tunnel. The code still expires after one use or 60 seconds.
Connecting to a remote daemon directly is out of scope.

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
| State and effects | Effect 4 in a `ManagedRuntime` with `useFork`-style hooks, TanStack Query for cached queries (see below) |
| Markdown | `react-markdown` 10 with `remark-gfm`, code blocks highlighted with Shiki, as T3 Code renders chat and pull request Markdown |
| Diffs and file trees | `@pierre/diffs` (also Shiki highlighting) and `@pierre/trees` |
| Editors | forms for definitions and scopes (below); CodeMirror 6 for skill files and as the YAML fallback, validated and completed against the configuration JSON Schema the daemon serves at `/ui/schema.json` for its own version and harnesses |
| Local state | Zustand for the staged changes, which live in the tab until applied |
| Graphs | React Flow (`@xyflow/react`), only for the scope view (below) |

### Effect

The UI uses Effect for its data layer, not for rendering:

- Every RPC goes through a small Effect service around the Connect client. Its
  failures become a tagged error union decoded from `ErrorDetail.reason`
  (`needs_login`, `mcp_failed`, `config_invalid`, …), and an unknown reason is
  a defect. Components then handle each reason exhaustively instead of
  catching `ConnectError`.
- Streams (`WatchRuntime`, `WatchOperation`, `Login`) are Effect streams whose
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

## Layout and scope

- A top bar navigates between pages and holds the scope and the staged changes.
- The sidebar, the same on every page, is the configuration as a tree, not a
  list of pages: definitions by kind, package skills under their package, and
  the files. Skills hold their files and MCP servers their tools, loaded when
  first expanded, so a server starts only when someone looks inside it.
  Selecting an entry opens it in the skills page, the MCP inspector, or the
  editor; pages keep no navigation of their own.
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
     a harness), see the configuration diff, then confirm.
   - **Packages:** status, lock, `agx install`, and `agx update` with progress.
5. **Configuration editor**
   - Configuration is edited through the interface, not as text:
     - create, edit, and delete skills, instructions, MCP servers, secrets,
       bundles, harness definitions, packages, and scope rules, in forms built
       for each kind;
     - scope rules are edited as lists of resources per directory and harness,
       with resources picked from what the configuration defines.
   - Changes are staged, not written one by one. The page shows the pending
     changes as one diff across every file they touch, and applies them
     atomically:
     - the daemon plans the whole set, validates the configuration it would
       produce, and writes nothing if anything fails;
     - files are rewritten the way the CLI's edit commands rewrite them, so
       comments, order, and formatting of everything else survive;
     - apply fails if a file changed since the plan, such as through a CLI
       edit, and the plan is made again.
   - Each definition's form shows the file and line it comes from, and new
     definitions go to the file the CLI would pick, or one you choose.
   - A definition whose fields no form models yet, such as an unusual MCP
     `auth` or harness `settings`, opens in a YAML editor for that one
     definition (CodeMirror with the agx JSON Schema), staged like a form.
   - Local skill files are edited in CodeMirror with Markdown support, staged
     like the rest.
   - A file is read-only when it is a symlink or not writable, which covers
     files Nix or Home Manager link from `/nix/store`. Its definitions open
     read-only, with the file's path and where the link points.
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
| `ConfigurationService.PlanChanges`, `ApplyChanges` | the editor: plan a set of definition, scope, and file changes, validate the configuration it would produce, and return the diff; apply it atomically if no file changed since |
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
