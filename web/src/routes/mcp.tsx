import { createFileRoute } from "@tanstack/react-router";
import * as Schema from "effect/Schema";
import { FileCode2Icon, HistoryIcon, WrenchIcon } from "lucide-react";
import { useState } from "react";

import { ButtonLink } from "~/components/ButtonLink.tsx";
import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "~/components/ui/card.tsx";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "~/components/ui/empty.tsx";
import { Pending } from "~/components/Pending.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import type { DaemonError } from "~/daemon/error.ts";
import { useDaemonMutation } from "~/daemon/query.ts";
import { mcpStates } from "~/features/runtime/states.ts";
import { useCallHistory } from "~/features/mcp/history.ts";
import { ToolForm } from "~/features/mcp/ToolForm.tsx";
import { ToolResult } from "~/features/mcp/ToolResult.tsx";
import { useServerTools } from "~/features/mcp/tools.ts";
import {
  type CallToolResponse,
  McpTools_Load,
  type McpTool,
  type McpTools,
} from "~/gen/agx/v1/agx_pb.ts";
import { parseSchema } from "~/lib/jsonSchema.ts";
import { Markdown } from "~/lib/markdown.tsx";
import { pageTitle } from "~/lib/title.ts";

const InspectorSearch = Schema.Struct({
  server: Schema.optional(Schema.String),
  tool: Schema.optional(Schema.String),
});

export const Route = createFileRoute("/mcp")({
  validateSearch: Schema.decodeUnknownSync(InspectorSearch),
  component: InspectorPage,
});

function InspectorPage() {
  const search = Route.useSearch();
  return (
    <div className="flex max-w-5xl flex-col gap-2 px-4 py-3">
      <title>
        {pageTitle(
          search.server === undefined
            ? undefined
            : search.tool === undefined
              ? search.server
              : `${search.server}.${search.tool}`,
          "MCP inspector",
        )}
      </title>
      <h1 className="font-heading font-semibold text-sm">MCP inspector</h1>
      {search.server === undefined ? (
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <WrenchIcon />
            </EmptyMedia>
            <EmptyTitle>Pick a tool in the sidebar</EmptyTitle>
            <EmptyDescription>
              Expand an MCP server to see its tools, and call them with a form built from their
              schema.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <ServerView key={search.server} mcpId={search.server} toolName={search.tool} />
      )}
    </div>
  );
}

function ServerView({
  mcpId,
  toolName,
}: {
  readonly mcpId: string;
  readonly toolName: string | undefined;
}) {
  const { tools, directory } = useServerTools(mcpId);
  if (tools.isPending) {
    return <Pending className="h-64" />;
  }
  if (tools.isError) {
    return <DaemonProblem error={tools.error} />;
  }
  const tool = tools.data.tools.find((t) => t.name === toolName);
  return tool === undefined ? (
    <ServerStatus server={tools.data} />
  ) : (
    <ToolPanel key={tool.name} server={tools.data} tool={tool} directory={directory} />
  );
}

/** A server's state, and why it has no tools to call when it has none. */
function ServerStatus({ server }: { readonly server: McpTools }) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 font-mono">
          {server.mcpId}
          <Badge variant={mcpStates[server.state].variant}>{mcpStates[server.state].label}</Badge>
          {server.load === McpTools_Load.ON_DEMAND && <Badge variant="secondary">on demand</Badge>}
        </CardTitle>
        <CardDescription>{server.description}</CardDescription>
        <CardAction>
          <ButtonLink
            variant="outline"
            size="sm"
            to="/config"
            search={{ kind: "mcp", id: server.mcpId }}
          >
            <FileCode2Icon data-icon="inline-start" />
            Definition
          </ButtonLink>
        </CardAction>
      </CardHeader>
      <CardContent className="text-xs">
        {server.message === "" ? (
          <span className="text-muted-foreground">
            {server.tools.length === 0 ? "It offers no tools." : "Pick one of its tools."}
          </span>
        ) : (
          <span className="text-destructive">{server.message}</span>
        )}
      </CardContent>
    </Card>
  );
}

function ToolPanel({
  server,
  tool,
  directory,
}: {
  readonly server: McpTools;
  readonly tool: McpTool;
  /** Where stdio servers without a cwd run. */
  readonly directory: string;
}) {
  const history = useCallHistory();
  const [result, setResult] = useState<CallToolResponse | null>(null);
  const [failure, setFailure] = useState<DaemonError | null>(null);
  const call = useDaemonMutation(
    (argumentsJson: string) =>
      Daemon.use((daemon) => daemon.callTool(directory, server.mcpId, tool.name, argumentsJson)),
    [],
  );
  const own = history.entries.filter(
    (entry) => entry.server === server.mcpId && entry.tool === tool.name,
  );
  return (
    <>
      <div className="flex flex-col gap-4 pr-3">
        <Card size="sm">
          <CardHeader>
            <CardTitle className="font-mono">
              {server.mcpId}.{tool.name}
            </CardTitle>
            {tool.title !== "" && <CardDescription>{tool.title}</CardDescription>}
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            {tool.description !== "" && <Markdown>{tool.description}</Markdown>}
            <ToolForm
              schema={parseSchema(tool.inputSchemaJson)}
              pending={call.isPending}
              onCall={(argumentsJson) => {
                setFailure(null);
                call.mutate(argumentsJson, {
                  onSuccess: (response) => {
                    setResult(response);
                    history.record({
                      server: server.mcpId,
                      tool: tool.name,
                      argumentsJson,
                      isError: response.isError,
                      at: Date.now(),
                    });
                  },
                  onError: (error) => setFailure(error),
                });
              }}
            />
          </CardContent>
        </Card>
        {failure !== null && <DaemonProblem error={failure} />}
        {result !== null && (
          <Card size="sm">
            <CardHeader>
              <CardTitle>Result</CardTitle>
            </CardHeader>
            <CardContent>
              <ToolResult response={result} />
            </CardContent>
          </Card>
        )}
        {own.length > 0 && (
          <Card size="sm">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <HistoryIcon />
                Earlier calls
              </CardTitle>
            </CardHeader>
            <CardContent>
              <ul className="flex flex-col gap-1">
                {own.map((entry) => (
                  <li key={entry.at} className="flex items-center gap-2 font-mono text-xs">
                    <Badge variant={entry.isError ? "destructive" : "secondary"}>
                      {new Date(entry.at).toLocaleTimeString()}
                    </Badge>
                    <span className="truncate text-muted-foreground">{entry.argumentsJson}</span>
                  </li>
                ))}
              </ul>
            </CardContent>
          </Card>
        )}
      </div>
    </>
  );
}
