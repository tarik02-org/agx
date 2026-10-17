# Centralize MCP execution behind separate HTTP endpoints

The local daemon owns upstream MCP connections, local MCP child processes,
authentication refresh, routing, and runtime telemetry. Harnesses connect to
the daemon using native HTTP MCP support; there are no per-server stdio
connector processes in the initial design.

Generate one named MCP configuration per selected connection, all served by the
same daemon, for example `/launch/<id>/mcp/<connection>`. Separate endpoints
preserve server identity, guidance, permissions, and connection controls without
merging catalogs. Expose native tools and schemas; a generic
`search_tools`/`call_tool` interface is outside the design. Preserve applicable
MCP capabilities, including resources, prompts, notifications, and cancellation.

Keep upstream sessions separate per launch initially. Central ownership does not
require pooling; sharing can be introduced when server behavior, identity, and
working-directory requirements permit it. This concentrates MCP complexity but
makes daemon failures affect all launches, so reconnection needs explicit handling.
