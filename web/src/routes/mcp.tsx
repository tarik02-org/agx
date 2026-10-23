import { createFileRoute, redirect } from "@tanstack/react-router";
import * as Schema from "effect/Schema";

const InspectorSearch = Schema.Struct({
  server: Schema.optional(Schema.String),
  tool: Schema.optional(Schema.String),
});

// An MCP server and its tools are an item of the configuration; their page is the item page.
// Links from before keep working.
export const Route = createFileRoute("/mcp")({
  beforeLoad: ({ location }) => {
    const search = Schema.decodeUnknownSync(InspectorSearch)(location.search);
    throw search.server === undefined
      ? redirect({ to: "/" })
      : redirect({
          to: "/config",
          search: {
            kind: "mcp",
            id: search.server,
            ...(search.tool === undefined ? {} : { tool: search.tool }),
          },
        });
  },
});
