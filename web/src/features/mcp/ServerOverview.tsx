import { Link } from "@tanstack/react-router";
import { HistoryIcon } from "lucide-react";
import { useState } from "react";

import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Pending } from "~/components/Pending.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "~/components/ui/card.tsx";
import { Item, ItemContent, ItemDescription, ItemGroup, ItemTitle } from "~/components/ui/item.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import type { DaemonError } from "~/daemon/error.ts";
import { queryKeys, useDaemonMutation, useDaemonQuery } from "~/daemon/query.ts";
import { useCallHistory } from "~/features/mcp/history.ts";
import { ToolForm } from "~/features/mcp/ToolForm.tsx";
import { ToolResult } from "~/features/mcp/ToolResult.tsx";
import { useServerTools } from "~/features/mcp/tools.ts";
import { ProcessTable, refreshInterval } from "~/features/runtime/ProcessesCard.tsx";
import { mcpStates } from "~/features/runtime/states.ts";
import {
  type CallToolResponse,
  McpTools_Load,
  type McpTool,
  type McpTools,
} from "~/gen/agx/v1/agx_pb.ts";
import { parseSchema } from "~/lib/jsonSchema.ts";
import { Markdown } from "~/lib/markdown.tsx";

/**
 * An MCP server, started if needed: its overview, or one of its tools with a form to call it.
 */
export function ServerOverview({
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

/**
 * A server at a glance: what it is and its state, what it offers, and the processes running it.
 * Tools are what servers offer today; prompts and resources join them as sections.
 */
function ServerStatus({ server }: { readonly server: McpTools }) {
  const processes = useDaemonQuery(
    queryKeys.processes,
    Daemon.use((daemon) => daemon.processes),
    { refetchInterval: refreshInterval },
  );
  const running = (processes.data ?? []).filter((process) => process.mcpId === server.mcpId);
  return (
    <>
      <Card size="sm">
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center gap-2">
            {server.icon !== "" && <img src={server.icon} alt="" className="size-5" />}
            {server.serverName !== "" && (
              <span className="font-mono">
                {server.serverName}
                {server.serverVersion !== "" && (
                  <span className="text-muted-foreground"> {server.serverVersion}</span>
                )}
              </span>
            )}
            <Badge variant={mcpStates[server.state].variant}>{mcpStates[server.state].label}</Badge>
            {server.load === McpTools_Load.ON_DEMAND && (
              <Badge variant="secondary">on demand</Badge>
            )}
          </CardTitle>
          {server.description !== "" && <CardDescription>{server.description}</CardDescription>}
        </CardHeader>
        {server.message !== "" && (
          <CardContent className="text-destructive text-xs">{server.message}</CardContent>
        )}
      </Card>
      <Card size="sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            Tools <Badge variant="secondary">{server.tools.length}</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {server.tools.length === 0 ? (
            <p className="text-muted-foreground text-xs">It offers no tools.</p>
          ) : (
            <ItemGroup className="gap-1">
              {server.tools.map((tool) => (
                <Item
                  key={tool.name}
                  size="sm"
                  variant="outline"
                  render={
                    <Link
                      to="/config"
                      search={{ kind: "mcp", id: server.mcpId, tool: tool.name }}
                    />
                  }
                >
                  <ItemContent>
                    <ItemTitle className="font-mono">{tool.name}</ItemTitle>
                    {(tool.title !== "" || tool.description !== "") && (
                      <ItemDescription className="line-clamp-1">
                        {tool.title !== "" ? tool.title : tool.description}
                      </ItemDescription>
                    )}
                  </ItemContent>
                </Item>
              ))}
            </ItemGroup>
          )}
        </CardContent>
      </Card>
      <Card size="sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            Processes <Badge variant="secondary">{running.length}</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {processes.isError ? (
            <DaemonProblem error={processes.error} />
          ) : running.length === 0 ? (
            <p className="text-muted-foreground text-xs">
              {server.load === McpTools_Load.ON_DEMAND
                ? "None running; it starts when a session calls it."
                : "None running; a local server starts with the first session that uses it."}
            </p>
          ) : (
            <ProcessTable processes={running} oneServer />
          )}
        </CardContent>
      </Card>
    </>
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
      {/* The page's first section; the header above names the tool. */}
      <Card size="sm">
        {tool.title !== "" && (
          <CardHeader>
            <CardTitle>{tool.title}</CardTitle>
          </CardHeader>
        )}
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
    </>
  );
}
