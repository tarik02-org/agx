# MCP servers

agx runs your MCP servers once, in its [daemon](daemon.md), and gives each
launch access to the servers selected for its directory. Credentials stay in
the daemon; the harness only gets a short-lived grant for its own launch. The
same servers can be [called from the shell](#calling-servers-from-the-shell),
and [on-demand servers](#on-demand-servers) stay out of the harness's tool list
until an agent calls them that way.

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
refreshed automatically: an expired access token is renewed when the server is
next used, and one the server rejects early, with 401, is renewed and the request
sent again. `agx secret status` shows such a token as "renews on next use". When
the authorization server refuses the refresh token itself, the secret reports
`needs_login` with the server's error code until you log in again.

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

Optional OAuth fields: `scopes` (a list), `issuer`, and `resource`. The issuer and its
endpoints must use https; plain http is accepted only on loopback addresses.

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

## Calling servers from the shell

`agx mcp tools` and `agx mcp call` reach the same servers a harness does, through
the daemon, so you can try a tool without a harness and agents can call one from
their shell:

```sh
agx mcp tools                     # servers selected here, their state and tools
agx mcp tools tracker             # one server's tool signatures
agx mcp call tracker.search_issues query='login bug' status=open limit=5
```

`agx mcp tools tracker` prints each tool as a compact, TypeScript-like signature
with the first line of its description:

```
search_issues(query: string, limit?: number, status?: "open" | "closed")
  Search issues by text.
```

Required arguments come first; `?` marks optional ones. A tool that declares a
structured result ends with its type, as in `): { total: number }`. `--schema`
adds the raw input and output JSON Schemas.

Arguments are `key=value` pairs, converted by the tool's schema:

| You write | The tool receives |
| --- | --- |
| `limit=5` | `5` when `limit` is a number or integer, `"5"` when it is a string |
| `exact=true` | `true` for a boolean |
| `note=null` | `null` when the schema allows it |
| `status=open` | `"open"`, checked against the enum |
| `tags=a tags=b` | `["a", "b"]`; a single `tags=a` becomes `["a"]` |
| `filter='{"owner":"me"}'` | an object or array, written as JSON |
| `body=@notes.md` | the file's content |
| `handle=@@me` | `"@me"`: `@@` escapes a leading `@` |

`--raw-strings` passes every value as a string, and `--args '<json>'` (or
`--args -` to read standard input) gives the whole arguments object; `key=value`
pairs then override its members. An unknown tool or argument, a value the schema
rejects, or a missing required argument fails before the call, naming the
closest matches and the tool's signature.

Text content is printed. Binary content (images, audio, binary resources) is
always written to files, into `--save-dir <dir>` or otherwise a new temporary
directory per call, and printed as one line per file:

```
[image/png, 34 KB] /tmp/agx-mcp-123/1.png
```

`--json` prints the result, with file content as `{type, mime_type, path,
size}` instead of base64:

```json
{"version":1,"data":{"mcp_id":"tracker","tool":"search_issues","is_error":false,"content":[{"type":"text","text":"…"}],"structured_content":null}}
```

`call` exits 0 on success, 1 when the call fails or the tool reports an error
(the result still prints), and 2 for malformed arguments. Errors inside the
server have the code `mcp_failed`; a server that needs a login fails with
`needs_login` and the `agx secret login` command to run. `--timeout` (default
`60s`) bounds the wait.

Which servers a call reaches:

- Inside a launch, the servers selected for that launch, native and on-demand,
  through the launch's own endpoints: the harness environment carries
  `AGX_LAUNCH_ID` and the launch grant `AGX_MCP_GRANT`, so values and
  environment inputs are the launch's.
- Outside a launch, the servers selected for the current directory, with the
  variables their values read taken from your shell, as `agx mcp test` does.
- With `--all`, every configured server, also inside a launch.

The daemon keeps its client session to each server for about five idle
minutes, per launch or directory, so repeated calls reuse a running local server
instead of starting it again.

## On-demand servers

Every server a launch selects normally sits in the harness's tool list, and its
tool definitions cost context in every request. A server you need now and then
can stay out of it:

```yaml
mcp:
  jira:
    url: https://mcp.atlassian.example/v1
    description: Jira issues and Confluence pages for the platform team
    load: on_demand
```

`load: on_demand` keeps the server out of every harness's configuration, while
launches still select it as usual: `agx mcp tools` lists it, with its
description, and `agx mcp call jira.<tool> …` calls it. `agx mcp add` takes
`--load on_demand` and `--description <text>`; `agx mcp list`, `agx resolve`,
and `agx explain` show the load mode.

agx does not tell agents about on-demand servers itself. Add an instruction that
does, worded for your setup, for example:

```yaml
instructions:
  on-demand-mcp:
    text: |
      More MCP servers are available on demand. Run `agx mcp tools` to list them,
      `agx mcp tools <server>` for a server's tool signatures, and
      `agx mcp call <server>.<tool> key=value … --json` to call a tool.
scopes:
  - add:
      instructions: [on-demand-mcp]
```

## Sharing a local server

Each harness session normally starts its own copy of a local server, and so does
`agx mcp call` from a shell. For servers whose value is warm state, such as a
file search server that indexes the project, one process per project serves
better:

```yaml
mcp:
  fff:
    command: [fff-mcp]
    share: repo
```

| `share` | One process for |
| --- | --- |
| `none` (default) | each harness session and each `agx mcp call` session |
| `directory` | every launch in the same directory |
| `repo` | every launch in the same repository, from any of its subdirectories; each worktree is a repository of its own, and outside a repository it acts like `directory` |
| `global` | every launch, wherever it runs |

A shared server runs in the directory or repository root it is shared for, or in
your home directory for `global`, unless `cwd` says otherwise. It stays five
minutes after its last session ends, so the next launch finds it warm; set how
long with the mapping form:

```yaml
mcp:
  fff:
    command: [fff-mcp]
    share: { type: repo, idle: 30m }   # idle: 0s stops it with its last session
```
 Launches still
reach only the servers they select. Sessions that differ in the server's command,
environment, secrets, `!env` values, or idle period get separate processes. A
shared server also stops when a secret it uses changes or you log in again, and
when the daemon stops. If it crashes, its sessions end and the next one starts
it again.

Share only servers that hold the same state for every session. One process sees
every launch's requests, so a server that remembers a selected workspace, a
logged-in user, or a conversation would mix launches up. A shared server also
cannot ask any one harness for anything: agx answers its `roots/list` requests
with the shared directory or repository (none for `global`) and refuses
sampling and elicitation. `share` applies only to local servers; remote servers
already serve many sessions.

`agx mcp add` takes `--share`; `agx mcp list`, `agx resolve`, and `agx explain`
show the share mode. `agx mcp ps` lists the local server processes running now,
shared or not, with their sessions and launches, and when an idle one stops.
`--cwd <directory>` keeps those working in exactly that directory, `--under
<directory>` those working in it or below it:

```console
$ agx mcp ps --under ~/work
ID   PID    SHARE       SESSIONS  LAUNCHES  STATE                  DIRECTORY
fff  41822  repo        3         2         in use                 ~/work/acme-web
fff  40310  repo        0         0         idle, stops in 4m12s   ~/work/blog
```

## Code mode

When a task chains several calls or filters large results, a script is cheaper
than one call per step. `agx mcp types` generates a typed client for the servers
you reach:

```sh
agx mcp types --out .agx          # writes .agx/agx-mcp.d.mts and .agx/agx-mcp.mjs
agx mcp types tracker             # prints the declarations only
```

`agx-mcp.d.mts` declares a typed function per tool, from its input and output
schemas. `agx-mcp.mjs` is a dependency-free ES module for node, bun, or deno
that calls the daemon directly, with the same `CallTool` request as
`agx mcp call`: inside a launch with `AGX_LAUNCH_ID` and `AGX_MCP_GRANT`,
outside one with the daemon's management token and the current directory's
servers. Credentials never reach the script.

```js
import { tracker, call } from "./.agx/agx-mcp.mjs";

const { structuredContent } = await tracker.search_issues({ query: "login bug", limit: 50 });
const open = structuredContent.issues.filter((issue) => issue.status === "open");
console.log(open.map((issue) => issue.key).join("\n"));
await call("tracker", "add_comment", { key: open[0].key, body: "Investigating." });
```

Each call resolves to `{isError, content, structuredContent, text}` and throws
an `AgxError` with one of the JSON error codes (`invalid_request`,
`mcp_failed`, `needs_login`, …) when it fails. A server whose ID is not a
JavaScript identifier is under `servers["team/docs"]`. Generated with `--all`,
the client reaches every configured server, also inside a launch. Regenerate it
when servers or their tools change.

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
    share: repo                     # one process per repository
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
| `description` | both | what the server offers, shown by `agx mcp tools` |
| `load` | both | `native` (default): in the harness's tool list; `on_demand`: reached with `agx mcp call` only |
| `share` | local | `none` (default), `directory`, `repo`, or `global`, or `{type, idle}`: see [Sharing a local server](#sharing-a-local-server) |

A server has either `url` or `command`. agx manages the `Authorization`,
`Cookie`, `Host`, and `Mcp-Session-Id` headers itself.

A local server starts with a minimal environment (`PATH`, `HOME`, temporary
directories, locale, and certificate settings) plus its `environment`, not your whole shell environment. Commands are
looked up on the daemon's `PATH`, which it takes from the shell that started
it; use `~/…` or absolute paths for programs outside the usual places.

Values from `!env` come from the shell you launch the harness from; see
[Secrets](secrets.md).
