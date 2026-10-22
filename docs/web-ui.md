# Web UI

The daemon serves a web UI for what is easier to see than to read in a
terminal. It starts with the runtime page:

- launches, updated live, and revoking one;
- the local MCP server processes `agx mcp ps` lists, and restarting one;
- secrets with their state, and OAuth login and logout;
- installing the configuration's packages, with progress.

The explain page answers what `agx explain` does: pick a directory (scope
directories are suggested, and typed paths complete) and optionally a harness,
and see every resource with the rules that added or removed it, the harness's
resolved settings, and packages it goes without.

The MCP inspector lists every configured server's tools and calls one with a
form built from its input schema, or raw JSON, showing the result's content and
keeping recent calls in the browser.

The skills page shows each skill's SKILL.md and files and where to enable it.
Adding skills starts from a Git source (`owner/repo[@ref][/path]` or a URL):
the daemon fetches it without configuring anything, the page previews its
skills, and you pick which to enable and where.

The configuration page edits every definition (MCP servers, skills,
instructions, secrets, bundles, packages, and harnesses) with a form, or as
YAML checked against the configuration schema, and edits configuration files
directly. Secrets stored in the keychain get their value from the runtime page.

Nothing is written while you edit. Changes are staged, and **Staged changes**
shows the diff of every file they touch; **Apply** writes them all at once,
formatted and commented as the CLI leaves them, or none if a file changed since
the diff. Files that are links, such as those Home Manager manages, and
definitions from packages are shown read-only. See
[ADR 0026](adr/0026-web-ui.md) for the design.

```sh
agx ui            # opens the UI in your browser, signed in
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
