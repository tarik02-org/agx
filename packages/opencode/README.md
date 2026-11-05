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
keeps these summaries when a location goes offline. Resume runs the session's
harness in its directory with `--session <id>`, including the harness's configured
arguments and environment. The plugin does not need to be connected for resume.

To use a different OpenCode profile or server, define an AGX harness:

```yaml
harnesses:
  opencode-work:
    base: opencode
    args: [--server, https://opencode.example.com]
    environment:
      OPENCODE_PASSWORD: !env OPENCODE_WORK_PASSWORD
```

Set `options.harness` to `opencode-work` in that OpenCode server's AGX plugin
configuration. Its published sessions will open with this harness. The default
is `opencode`, which uses the client's normal OpenCode profile. Keep native
profile environment settings in the harness consistent with the server loading
the plugin. AGX requires the selected session directory to exist locally.

AGX starts the native client; the plugin supplies resources for its location.
The plugin's harness also selects its AGX resources. Client arguments and
environment apply at launch; changing them does not reconfigure an existing
OpenCode service.

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
