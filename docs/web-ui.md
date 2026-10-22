# Web UI

The daemon serves a web UI for what is easier to see than to read in a
terminal. The top bar switches pages and holds the **scope**: a directory, or
everywhere. `agx ui` scopes the UI to the directory it runs in, and the scope
stays in the address, so a scoped page can be shared and reloaded. The sidebar,
on every page, is the configuration as a tree: every definition by kind,
package skills under their package, and the configuration files. Expanding a
skill lists its files, and expanding an MCP server starts it and lists its
tools. When the UI is scoped, the tree shows only what a launch in that
directory gets, or everything with one switch.

The runtime page shows:

- launches, updated live, and revoking one;
- the local MCP server processes `agx mcp ps` lists, and restarting one;
- secrets with their state, and OAuth login and logout;
- installing the configuration's packages, with progress.

The explain page answers what `agx explain` does for the scope directory and
optionally a harness: every resource with the rules that added or removed it,
the harness's resolved settings grouped by key with where each was set, and
packages it goes without.

The MCP inspector shows the server or tool picked in the tree, starting servers
without their own working directory in the scope directory, and calls a tool
with a form built from its input schema, or raw JSON, showing the result's
content and keeping recent calls in the browser.

The skills page shows the skill or skill file picked in the tree, editing a
local skill's files, and where to enable it.
Adding skills is a wizard: a Git source (`owner/repo[@ref][/path]` or a URL),
which the daemon fetches without configuring anything; the skills to enable,
with their SKILL.md and files; where to enable them, starting with the scope;
and the diff, applied at once or added to the staged changes.

The configuration page edits every definition (MCP servers, skills,
instructions, secrets, bundles, packages, and harnesses) with a form, or as
YAML checked against the configuration schema, and edits configuration files
directly. Secrets stored in the keychain get their value from the runtime page.

Nothing is written while you edit. Changes are staged, and **Staged changes**
lists them, with the diff of every file they touch on a second tab; **Apply** writes them all at once,
formatted and commented as the CLI leaves them, or none if a file changed since
the diff. Files that are links, such as those Home Manager manages, and
definitions from packages are shown read-only. See
[ADR 0026](adr/0026-web-ui.md) for the design.

```sh
agx ui            # opens the UI in your browser, signed in and scoped to this directory
agx ui --print    # prints the sign-in link instead
```

The UI runs at `http://127.0.0.1:<port>/ui/` on the daemon's own loopback
port, so only this machine can reach it.

## Signing in

`agx ui` asks the daemon for a one-time sign-in link. The link works once and
for one minute, and the browser trades it for a session cookie that the page
keeps for 12 hours of use. The management token never appears in an address.
Sessions live in the daemon's memory, so after `agx daemon restart` the page
says it is signed out: run `agx ui` again.

For a daemon on another machine, print a link there, forward the port it
names, and open the link in your local browser:

```sh
ssh host agx ui --print                 # http://127.0.0.1:<port>/ui/login?code=…
ssh -N -L <port>:127.0.0.1:<port> host
```

Binaries built with plain `go build` or `go install` from source do not contain
the UI; `/ui/` then says so. Release binaries and the Nix package include it.
