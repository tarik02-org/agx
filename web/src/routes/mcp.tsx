import { createFileRoute, useNavigate } from "@tanstack/react-router";
import * as Schema from "effect/Schema";
import { HistoryIcon, WrenchIcon } from "lucide-react";
import { useState } from "react";

import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { Button } from "~/components/ui/button.tsx";
import {
  Card,
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
import { Input } from "~/components/ui/input.tsx";
import { ScrollArea } from "~/components/ui/scroll-area.tsx";
import { Skeleton } from "~/components/ui/skeleton.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import type { DaemonError } from "~/daemon/error.ts";
import { queryKeys, useDaemonMutation, useDaemonQuery } from "~/daemon/query.ts";
import { mcpStates } from "~/features/runtime/states.ts";
import { useCallHistory } from "~/features/mcp/history.ts";
import { ToolForm } from "~/features/mcp/ToolForm.tsx";
import { ToolResult } from "~/features/mcp/ToolResult.tsx";
import {
  type CallToolResponse,
  McpTools_Load,
  type McpTool,
  type McpTools,
} from "~/gen/agx/v1/agx_pb.ts";
import { parseSchema } from "~/lib/jsonSchema.ts";
import { Markdown } from "~/lib/markdown.tsx";

const InspectorSearch = Schema.Struct({
  server: Schema.optional(Schema.String),
  tool: Schema.optional(Schema.String),
});

export const Route = createFileRoute("/mcp")({
  validateSearch: Schema.decodeUnknownSync(InspectorSearch),
  component: InspectorPage,
});

function InspectorPage() {
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const home = configuration.data?.home ?? "";
  const tools = useDaemonQuery(
    queryKeys.tools,
    Daemon.use((daemon) => daemon.tools(home)),
    {
      enabled: configuration.isSuccess,
    },
  );
  return (
    <div className="flex h-full flex-col gap-4 p-4">
      <h1 className="font-heading font-semibold text-sm">MCP inspector</h1>
      {(configuration.isPending || tools.isPending) && <Skeleton className="h-64" />}
      {configuration.isError && <DaemonProblem error={configuration.error} />}
      {tools.isError && <DaemonProblem error={tools.error} />}
      {tools.isSuccess && <Inspector servers={tools.data} home={home} />}
    </div>
  );
}

function Inspector({
  servers,
  home,
}: {
  readonly servers: ReadonlyArray<McpTools>;
  readonly home: string;
}) {
  const search = Route.useSearch();
  const navigate = useNavigate({ from: Route.fullPath });
  const [filter, setFilter] = useState("");
  const server = servers.find((s) => s.mcpId === search.server);
  const tool = server?.tools.find((t) => t.name === search.tool);
  const matches = (name: string) => name.toLowerCase().includes(filter.toLowerCase());
  return (
    <div className="grid min-h-0 flex-1 grid-cols-[18rem_minmax(0,1fr)] gap-4">
      <Card size="sm" className="min-h-0">
        <CardHeader>
          <Input
            placeholder="Filter tools"
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
          />
        </CardHeader>
        <ScrollArea className="min-h-0 flex-1">
          <ul className="flex flex-col gap-3 px-3 pb-3">
            {servers.map((s) => (
              <li key={s.mcpId} className="flex flex-col gap-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium font-mono">{s.mcpId}</span>
                  <Badge variant={mcpStates[s.state].variant}>{mcpStates[s.state].label}</Badge>
                  {s.load === McpTools_Load.ON_DEMAND && (
                    <Badge variant="secondary">on demand</Badge>
                  )}
                </div>
                {s.message !== "" && <span className="text-destructive text-xs">{s.message}</span>}
                <ul className="flex flex-col">
                  {s.tools
                    .filter((t) => filter === "" || matches(t.name) || matches(s.mcpId))
                    .map((t) => (
                      <li key={t.name}>
                        <Button
                          variant={
                            s.mcpId === search.server && t.name === search.tool
                              ? "secondary"
                              : "ghost"
                          }
                          size="sm"
                          className="w-full justify-start font-mono"
                          onClick={() =>
                            void navigate({ search: { server: s.mcpId, tool: t.name } })
                          }
                        >
                          {t.name}
                        </Button>
                      </li>
                    ))}
                </ul>
              </li>
            ))}
          </ul>
        </ScrollArea>
      </Card>
      {server === undefined || tool === undefined ? (
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <WrenchIcon />
            </EmptyMedia>
            <EmptyTitle>Pick a tool</EmptyTitle>
            <EmptyDescription>
              Call any configured MCP server's tools, with a form built from their schema.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <ToolPanel key={`${server.mcpId}/${tool.name}`} server={server} tool={tool} home={home} />
      )}
    </div>
  );
}

function ToolPanel({
  server,
  tool,
  home,
}: {
  readonly server: McpTools;
  readonly tool: McpTool;
  readonly home: string;
}) {
  const history = useCallHistory();
  const [result, setResult] = useState<CallToolResponse | null>(null);
  const [failure, setFailure] = useState<DaemonError | null>(null);
  const call = useDaemonMutation(
    (argumentsJson: string) =>
      Daemon.use((daemon) => daemon.callTool(home, server.mcpId, tool.name, argumentsJson)),
    [],
  );
  const own = history.entries.filter(
    (entry) => entry.server === server.mcpId && entry.tool === tool.name,
  );
  return (
    <ScrollArea className="min-h-0">
      <div className="flex flex-col gap-4 pr-3">
        <Card size="sm">
          <CardHeader>
            <CardTitle className="font-mono">
              {server.mcpId}.{tool.name}
            </CardTitle>
            {tool.title !== "" && <CardDescription>{tool.title}</CardDescription>}
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
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
    </ScrollArea>
  );
}
