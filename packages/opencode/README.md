# AGX for OpenCode 2

Use your AGX skills, instructions and MCP servers in OpenCode. Each directory gets
its matching AGX configuration when you open it in OpenCode.

## Install

[Install a current AGX build](https://github.com/tarik02-org/agx#install) on the machine running
OpenCode's server, then install the plugin through OpenCode:

```sh
opencode2 plugin add github:tarik02-org/agx#opencode-plugin
```

OpenCode downloads a ready-to-run package and manages its installation. Restart
OpenCode after adding the plugin. AGX must be available on the server's PATH; the
plugin connects to the daemon and starts it when needed.

The package is published to the `opencode-plugin` branch after successful builds
of AGX's main branch. It is tested with OpenCode 2.2.5.

## Use it

Start OpenCode normally in a project directory:

- AGX skills appear in OpenCode's skill catalog, including their supporting files.
- AGX instructions are included in conversations and compaction.
- Eager MCP servers appear as native OpenCode tools. On-demand servers remain
  available through `agx mcp call`.

Sessions in the same directory share its resources. Opening another directory
selects the AGX scopes for that directory.

After changing your AGX configuration, run this command inside OpenCode:

```text
/agx-reload
```

## Launch and resume

With the plugin installed, launch OpenCode through AGX:

```sh
agx opencode
```

Open `agx` to pick a session, or list sessions with `agx sessions --all`.
The plugin publishes session summaries from locations opened in OpenCode. AGX
keeps these summaries when a location goes offline; resuming asks the plugin to
validate the session and provide its server connection.

The normal local OpenCode service is detected automatically. For a manually
started or remote server, set the plugin's `sessionServer` option to its URL
reachable from the client. Authentication comes from the server's
`OPENCODE_USERNAME` and `OPENCODE_PASSWORD` environment variables. An offline
location can be activated automatically when resuming with the same local
OpenCode profile; otherwise start its server first.

AGX starts the native client; the plugin supplies resources for its location.
Custom AGX launcher profiles with per-launch resources, arguments or environment
settings are unsupported for this integration.

To see what AGX selects for the current directory:

```sh
agx explain --harness opencode
```

Manage skills, MCP servers and authentication through
[AGX's configuration](https://github.com/tarik02-org/agx/blob/master/docs/configuration.md).

## Update

Let OpenCode update the plugin:

```sh
opencode2 plugin update github:tarik02-org/agx#opencode-plugin
```

Update the AGX binary through your usual installation method. For remote OpenCode,
AGX and the plugin run on the OpenCode server.

[Advanced options and development](https://github.com/tarik02-org/agx/blob/master/docs/development/opencode.md)
