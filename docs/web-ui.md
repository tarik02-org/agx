# Web UI

The daemon serves a web UI for what is easier to see than to read in a
terminal. It starts with the runtime: running launches and the MCP servers in
them, updated live. Explain, the MCP inspector, skills, and the configuration
editor follow ([ADR 0026](adr/0026-web-ui.md)).

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
