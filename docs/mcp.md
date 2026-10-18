# MCP servers

agx runs your MCP servers once, in its [daemon](daemon.md), and gives each
launch access to the servers selected for its directory. Credentials stay in
the daemon; the harness only gets a short-lived grant for its own launch.

## Adding a server

A remote server, by URL:

```sh
agx mcp add tracker https://tracker.example.com/mcp
```

A local server, by the command that starts it (after `--`):

```sh
agx mcp add fs -- npx -y @modelcontextprotocol/server-filesystem .
```

Both are enabled everywhere unless you pass `--scope here`, `--scope <dir>`, or
`--harness <name>`; see [Editing with commands](configuration.md#editing-with-commands).
More options:

```sh
# an API key header, read from your shell's DOCS_KEY at each launch
agx mcp add docs https://docs.example.com/mcp --auth none --header 'X-Api-Key=!env DOCS_KEY'

# an environment variable for a local server, from your shell
agx mcp add db --env 'DB_URL=!env DATABASE_URL' -- ~/bin/db-mcp --stdio
```

Harnesses see each server as `agx-<id>`: `agx-tracker`, `agx-fs`. A server from
a package, such as `team/github`, becomes `agx-team-github`.

## Logging in

Many remote servers use OAuth. When you add one, agx asks it whether it needs a
login, and on a terminal offers to log in right away. Log in later, or again,
with:

```sh
agx secret login tracker
```

`agx mcp login tracker` does the same for a server's own login.

This opens the authorization page in your browser and waits until you finish.
`--no-browser` prints the URL instead, for example to open it on another device
of yours; the browser must still be able to reach `127.0.0.1` on this machine,
since that is where the login returns. Tokens are stored in the OS keychain
(macOS Keychain, Windows Credential Manager, or the Secret Service on Linux) and
refreshed automatically.

Until you log in, the server reports `needs-auth`, and tool calls from the
harness fail with an error telling you to run the login.

### Servers that need a client ID

agx registers itself with the authorization server automatically when the
server supports it. When it does not, register an OAuth application with the
provider and give agx its client ID:

```sh
agx mcp add internal https://mcp.internal.example/mcp --client-id agx-cli
```

which writes:

```yaml
mcp:
  internal:
    url: https://mcp.internal.example/mcp
    auth:
      kind: oauth
      client_id: agx-cli
```

Use `http://127.0.0.1/oauth/callback` as the application's callback URL; the
daemon receives the login there, on a port that can change. agx acts as a
public client: it uses PKCE and no client secret, so this only works with
providers that allow public clients.

Optional OAuth fields: `scopes` (a list), `issuer`, and `resource`.

### GitHub

GitHub's OAuth applications always require a client secret, which agx does not
support. Use a [personal access token](https://github.com/settings/personal-access-tokens)
instead, stored in the OS keychain:

```sh
agx mcp add github https://api.githubcopilot.com/mcp/ --auth keychain
agx secret set github --stdin < github-token.txt
```

agx sends the token as a bearer token. To replace it, run `secret set` again.

### Other ways to authenticate

`auth` decides what agx sends to a remote server:

```yaml
mcp:
  tracker:
    url: https://tracker.example.com/mcp      # no auth: OAuth when the server asks
  sso:
    url: https://sso.example.com/mcp
    auth: {kind: oauth, client_id: agx-cli}   # OAuth with a registered client
  github:
    url: https://api.githubcopilot.com/mcp/
    auth: keychain                            # a token you store once
  shared:
    url: https://shared.example.com/mcp
    auth: !secret team-token                  # a secret defined under secrets
  public:
    url: https://public.example.com/mcp
    auth: none                                # never send credentials
```

- **Omitted**: the server is used without credentials until it answers with an
  OAuth challenge; then it needs `agx secret login <id>`.
- **`oauth`**, or a mapping with `kind: oauth`: always OAuth.
- **`keychain`**: a token stored with `agx secret set github --stdin`, sent as
  a bearer token.
- **`!secret <id>`**: any [secret](secrets.md), such as an environment variable
  or a command's output, sent as a bearer token. Several servers can share one.
- **`none`**: no credentials.

The secret of an inline `auth` takes the server's ID: `agx secret set github
--stdin`, `agx secret login sso`, `agx secret logout github`.

## Testing a server

```sh
agx mcp test tracker
```

`test` connects to the server through the daemon, with its credentials, and
lists its tools. When it fails, it says why: unreachable, needs a login, or the
server's error. `agx doctor` tests every server selected for the current
directory (`--all` for every server).

`agx mcp list` shows all configured servers, and `agx launch show <id>` the
state of each server in a running launch.

## Configuration reference

```yaml
mcp:
  tracker:                          # remote (HTTP)
    url: https://tracker.example.com/mcp
    headers:
      X-Team: platform
      X-Api-Key: !secret tracker-key
    auth: none

  fs:                               # local (stdio)
    command: [npx, -y, '@modelcontextprotocol/server-filesystem', .]
    cwd: ~/work                     # default: the launch directory
    environment:
      LOG_LEVEL: warning
      API_TOKEN: !secret fs-token
```

| Field | Applies to | Meaning |
| --- | --- | --- |
| `url` | remote | the server's MCP endpoint |
| `headers` | remote | extra HTTP headers; values may use `!secret`, `!env`, `!file` |
| `auth` | remote | credentials, see above |
| `command` | local | the program and its arguments |
| `cwd` | local | working directory; relative to the configuration file |
| `environment` | local | environment variables; values may use `!secret`, `!env`, `!file` |

A server has either `url` or `command`. agx manages the `Authorization`,
`Cookie`, `Host`, and `Mcp-Session-Id` headers itself.

A local server starts with a minimal environment (`PATH`, `HOME`, temporary
directories, locale, and certificate settings) plus its `environment`, not your whole shell environment. Commands are
looked up on the daemon's `PATH`, which it takes from the shell that started
it; use `~/…` or absolute paths for programs outside the usual places.

Values from `!env` come from the shell you launch the harness from; see
[Secrets](secrets.md).
