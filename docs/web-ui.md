# Web UI

The daemon serves a web UI for what is easier to see than to read in a
terminal. Every page's header starts with the **scope**: a directory or
everywhere, picked from the directories scope rules name or by browsing
folders, and the harness launching there. It goes on with where the page is and
every action it offers, **New**, and **Staged** while changes wait. `agx ui`
scopes the UI to the directory it runs in, and the scope stays in the address,
so a scoped page can be shared and reloaded. The sidebar holds the runtime page
and the configuration as a tree: ungrouped, grouped by bundle (each bundle with
what it selects, then what no bundle selects), or grouped by file (each fragment
with what it defines), and split by type or not. An empty kind offers to add
one, and package skills sit under their package. Expanding a
skill lists its files, and expanding an MCP server starts it and lists its
tools. Icons hint at what each item does,
from its name and description. A skill draws the `interface.icon_small` and
`interface.brand_color` its `agents/openai.yaml` declares, and an MCP server the
icon it declared when it last started, which the daemon keeps in its cache.

Until the daemon answers, and whenever the page's session ends or the daemon
stops answering, the whole window says so and how to fix it: run `agx ui` to
sign in again, or `agx daemon start`. The page carries on by itself once the
daemon answers again, with unsaved edits kept.

The runtime page shows:

- launches, updated live, and revoking one;
- the local MCP server processes `agx mcp ps` lists, and restarting one;
- secrets with their state, and OAuth login and logout;
- installing the configuration's packages, with progress.

Selecting an item in the tree opens its page. **Overview** shows what it is:
a skill's card and SKILL.md, with a local skill's files editable; an MCP server's
state, tools, and processes, starting servers without their own working
directory in the scope directory; a tool's form, built from its input schema or
raw JSON, with the result's content and recent calls kept in the browser; a
harness's resolved settings, grouped by key with where each was set; a package's
source and what it provides; what a bundle adds; instructions' text; and a
secret's state, with setting or logging in, and the MCP servers that use it. When the UI
is scoped, it also says why the item is or is not part of a launch there, with
the rules that added or removed it. **Definition** edits it with a form, or as
YAML checked against the configuration schema. The tree's **In scope** mode
shows only what a launch in the scope directory gets for the scope's harness;
**All**, the default, shows everything and marks what is in scope. **New**
creates a definition of any kind or adds skills from Git. The runtime page
starts with counts and what needs attention, such as a server that fails to
start or a secret waiting for a login.

Adding skills is a wizard: a Git source (`owner/repo[@ref][/path]` or a URL),
which the daemon fetches without configuring anything; the skills to enable,
with their SKILL.md and files; where to enable them, starting with the scope;
and the diff, applied at once or added to the staged changes.

Configuration files are edited directly from the tree's files. Secrets stored in the keychain get their value from the runtime page.

Nothing is written while you edit. Changes are staged, and **Staged changes**
lists them, with the diff of every file they touch on a second tab; **Apply** writes them all at once,
formatted and commented as the CLI leaves them, or none if a file changed since
the diff. New definitions go to the only writable fragment; with several, the review asks
which one, or for a new file. Files that are links, such as those Home Manager manages, and
definitions from packages are shown read-only. See
[ADR 0028](adr/0028-web-ui.md) for the design.

```sh
agx ui            # opens the UI in your browser, signed in and scoped to this directory
agx ui --print    # prints the sign-in link instead
```

The UI runs at `http://127.0.0.1:<port>/ui/` on the daemon's own loopback
port, so only this machine can reach it.

## Signing in

`agx ui` asks the daemon for a one-time sign-in link. The link works once and
for one minute, and the browser trades it for a session token that the page
keeps in its own storage and that lasts for 12 hours of use. The management
token never appears in an address, and the session never in a cookie, which
browsers would send to every other local server.
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
