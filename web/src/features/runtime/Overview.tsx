import { cn } from "cn";
import { KeyRoundIcon, ServerIcon } from "lucide-react";

import { ButtonLink } from "~/components/ButtonLink.tsx";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "~/components/ui/card.tsx";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from "~/components/ui/item.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { refreshInterval } from "~/features/runtime/ProcessesCard.tsx";
import { SecretActions } from "~/features/runtime/SecretsCard.tsx";
import { isActiveLaunch, mcpStates, secretStates } from "~/features/runtime/states.ts";
import {
  McpRuntime_State,
  type RuntimeView,
  type SecretStatus,
  SecretStatus_State,
} from "~/gen/agx/v1/agx_pb.ts";

/** Counts at a glance, and what needs a hand before launches work as configured. */
export function RuntimeOverview({ view }: { readonly view: RuntimeView }) {
  const processes = useDaemonQuery(
    queryKeys.processes,
    Daemon.use((daemon) => daemon.processes),
    { refetchInterval: refreshInterval },
  );
  const secrets = useDaemonQuery(
    queryKeys.secrets,
    Daemon.use((daemon) => daemon.secrets),
  );
  const active = view.launches.filter((launch) => isActiveLaunch(launch.state));
  const harnesses = [...new Set(active.map((launch) => launch.harness))];
  const shared = (processes.data ?? []).filter((p) => p.share !== "none").length;
  // A server fails once per launch using it; one row per server says enough.
  const failing = [
    ...new Map(
      view.mcpServers
        .filter(
          (server) =>
            server.state === McpRuntime_State.FAILED ||
            server.state === McpRuntime_State.NEEDS_AUTH,
        )
        .map((server) => [server.mcpId, server]),
    ).values(),
  ];
  const unready = (secrets.data ?? []).filter(
    (secret) =>
      secret.state === SecretStatus_State.NEEDS_LOGIN ||
      secret.state === SecretStatus_State.UNCONFIGURED ||
      secret.state === SecretStatus_State.UNAVAILABLE,
  );
  const ready = (secrets.data ?? []).filter((s) => s.state === SecretStatus_State.READY).length;
  const attention = failing.length + unready.length;
  return (
    <>
      <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
        <Stat
          label="Launches"
          value={active.length}
          detail={harnesses.length === 0 ? "none running" : harnesses.join(" · ")}
        />
        <Stat
          label="MCP processes"
          value={processes.data?.length ?? "–"}
          detail={`${shared} shared`}
        />
        <Stat
          label="Needs attention"
          value={attention}
          detail={
            attention === 0 ? "all good" : `${failing.length} servers · ${unready.length} secrets`
          }
          warn={attention > 0}
        />
        <Stat label="Secrets" value={secrets.data?.length ?? "–"} detail={`${ready} ready`} />
      </div>
      {attention > 0 && (
        <Card size="sm">
          <CardHeader>
            <CardTitle>Needs attention</CardTitle>
            <CardDescription>Launches go without these until they are fixed.</CardDescription>
          </CardHeader>
          <CardContent>
            <ItemGroup>
              {failing.map((server) => (
                <Item key={server.mcpId} size="sm" variant="outline">
                  <ItemMedia variant="icon">
                    <ServerIcon />
                  </ItemMedia>
                  <ItemContent>
                    <ItemTitle className="font-mono">{server.mcpId}</ItemTitle>
                    <ItemDescription>
                      {mcpStates[server.state].label}
                      {server.diagnostics.length > 0 &&
                        `: ${server.diagnostics.map((d) => d.summary).join("; ")}`}
                    </ItemDescription>
                  </ItemContent>
                  <ItemActions>
                    <ButtonLink
                      variant="outline"
                      size="sm"
                      to="/config"
                      search={(current) => ({ dir: current.dir, kind: "mcp", id: server.mcpId })}
                    >
                      Open
                    </ButtonLink>
                  </ItemActions>
                </Item>
              ))}
              {unready.map((secret) => (
                <SecretItem key={secret.secretId} secret={secret} />
              ))}
            </ItemGroup>
          </CardContent>
        </Card>
      )}
    </>
  );
}

function SecretItem({ secret }: { readonly secret: SecretStatus }) {
  return (
    <Item size="sm" variant="outline">
      <ItemMedia variant="icon">
        <KeyRoundIcon />
      </ItemMedia>
      <ItemContent>
        <ItemTitle className="font-mono">{secret.secretId}</ItemTitle>
        <ItemDescription>
          {secretStates[secret.state].label}
          {secret.mcpId !== "" && ` · ${secret.mcpId} waits for it`}
          {secret.diagnostics.length > 0 &&
            `: ${secret.diagnostics.map((d) => d.summary).join("; ")}`}
        </ItemDescription>
      </ItemContent>
      <ItemActions>
        <SecretActions secret={secret} />
      </ItemActions>
    </Item>
  );
}

function Stat({
  label,
  value,
  detail,
  warn = false,
}: {
  readonly label: string;
  readonly value: number | string;
  readonly detail: string;
  readonly warn?: boolean;
}) {
  return (
    <Card size="sm">
      <CardContent className="flex flex-col gap-0.5">
        <span className="text-muted-foreground text-xs">{label}</span>
        <span className={cn("font-semibold text-2xl", warn && "text-warning")}>{value}</span>
        <span className="truncate text-muted-foreground text-xs">{detail}</span>
      </CardContent>
    </Card>
  );
}
