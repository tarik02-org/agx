# Generate native launch artifacts without custom homes

Initially support Codex, Claude Code, and Pi through harness-specific adapters.
Each adapter turns a resolved snapshot into native arguments, configuration,
and skill/plugin artifacts. Preserve the harness protocol, standard streams,
exit behavior, and native account state. Managed artifacts live in Weft's cache;
manager-owned harness homes and isolation from existing global skills are not
requirements.

Use Claude's session plugin loading and Codex's local marketplace and launch
configuration mechanisms. Pi supports explicit skill paths and MCP configuration.
Generate filtered plugin contents when exclusions remove individual resources.
Inject always-on instructions separately: merely placing a guidance file in a
plugin does not guarantee that the harness loads it.

T3 Code starts one app-server per thread, so each thread can receive its own
snapshot and launch grant. Native plugins are output formats, not Weft's package
or bundle model. Adapters must support macOS, Linux, and Windows.
