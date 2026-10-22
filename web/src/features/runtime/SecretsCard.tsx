import { useQueryClient } from "@tanstack/react-query";
import * as Effect from "effect/Effect";
import * as Stream from "effect/Stream";
import { KeyRoundIcon, LogInIcon, LogOutIcon } from "lucide-react";

import { ConfirmAction } from "~/components/ConfirmAction.tsx";
import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { Button } from "~/components/ui/button.tsx";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card.tsx";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "~/components/ui/empty.tsx";
import { Skeleton } from "~/components/ui/skeleton.tsx";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "~/components/ui/table.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonMutation, useDaemonQuery } from "~/daemon/query.ts";
import { useEffectCallback } from "~/effect/react.tsx";
import { secretStates } from "~/features/runtime/states.ts";
import {
  LoginFinished_Outcome,
  type SecretStatus,
  SecretStatus_State,
} from "~/gen/agx/v1/agx_pb.ts";

export function SecretsCard() {
  const secrets = useDaemonQuery(
    queryKeys.secrets,
    Daemon.use((daemon) => daemon.secrets),
  );
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Secrets</CardTitle>
      </CardHeader>
      <CardContent>
        {secrets.isPending && <Skeleton className="h-16" />}
        {secrets.isError && <DaemonProblem error={secrets.error} />}
        {secrets.isSuccess && <SecretTable secrets={secrets.data} />}
      </CardContent>
    </Card>
  );
}

const loginOutcomes: Record<
  LoginFinished_Outcome,
  { readonly type: "success" | "error"; readonly title: string }
> = {
  [LoginFinished_Outcome.UNSPECIFIED]: { type: "error", title: "Login ended" },
  [LoginFinished_Outcome.SUCCEEDED]: { type: "success", title: "Logged in" },
  [LoginFinished_Outcome.FAILED]: { type: "error", title: "Login failed" },
  [LoginFinished_Outcome.EXPIRED]: { type: "error", title: "Login expired" },
};

function SecretTable({ secrets }: { readonly secrets: ReadonlyArray<SecretStatus> }) {
  const client = useQueryClient();
  const logout = useDaemonMutation(
    (secretId: string) => Daemon.use((daemon) => daemon.logout(secretId)),
    [queryKeys.secrets],
  );
  // The login page opens in a window the click itself opened, since browsers block windows
  // opened later; the daemon's authorization URL then replaces its blank page.
  const login = useEffectCallback(
    (secretId: string, popup: Window | null) =>
      Daemon.use((daemon) => Effect.succeed(daemon.login(secretId))).pipe(
        Stream.unwrap,
        Stream.runForEach((event) =>
          Effect.sync(() => {
            switch (event.event.case) {
              case "started":
                if (popup === null) {
                  toast.add({
                    type: "info",
                    title: "Open the login page",
                    description: event.event.value.authorizationUrl,
                  });
                } else {
                  popup.location.href = event.event.value.authorizationUrl;
                }
                return;
              case "finished": {
                const outcome = loginOutcomes[event.event.value.outcome];
                toast.add({
                  type: outcome.type,
                  title: `${outcome.title}: ${secretId}`,
                  description: event.event.value.message,
                });
                return;
              }
              case undefined:
                return;
            }
          }),
        ),
        Effect.catchTag("DaemonError", (error) =>
          Effect.sync(() => {
            popup?.close();
            toast.add({
              type: "error",
              title: `Login failed: ${secretId}`,
              description: error.message,
            });
          }),
        ),
        Effect.ensuring(
          Effect.promise(() => client.invalidateQueries({ queryKey: queryKeys.secrets })),
        ),
      ),
    [client],
  );

  if (secrets.length === 0) {
    return (
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <KeyRoundIcon />
          </EmptyMedia>
          <EmptyTitle>No secrets</EmptyTitle>
          <EmptyDescription>Secrets the configuration defines show up here.</EmptyDescription>
        </EmptyHeader>
      </Empty>
    );
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Secret</TableHead>
          <TableHead>Kind</TableHead>
          <TableHead>State</TableHead>
          <TableHead>Problem</TableHead>
          <TableHead />
        </TableRow>
      </TableHeader>
      <TableBody>
        {secrets.map((secret) => (
          <TableRow key={secret.secretId}>
            <TableCell>{secret.secretId}</TableCell>
            <TableCell className="text-muted-foreground">
              {secret.kind}
              {secret.mcpId !== "" && ` · ${secret.mcpId}`}
            </TableCell>
            <TableCell>
              <Badge variant={secretStates[secret.state].variant}>
                {secretStates[secret.state].label}
              </Badge>
            </TableCell>
            <TableCell className="text-muted-foreground">
              {secret.diagnostics.map((d) => d.summary).join("; ")}
            </TableCell>
            <TableCell className="text-right">
              {secret.kind === "oauth" && (
                <div className="flex justify-end gap-1">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => login(secret.secretId, globalThis.open("about:blank", "_blank"))}
                  >
                    <LogInIcon data-icon="inline-start" />
                    Log in
                  </Button>
                  {secret.state === SecretStatus_State.READY && (
                    <ConfirmAction
                      trigger={
                        <Button variant="ghost" size="sm">
                          <LogOutIcon data-icon="inline-start" />
                          Log out
                        </Button>
                      }
                      title={`Log out of ${secret.secretId}?`}
                      description="agx forgets the stored tokens; MCP servers using them need a new login."
                      action="Log out"
                      onConfirm={() =>
                        logout.mutate(secret.secretId, {
                          onError: (error) =>
                            toast.add({
                              type: "error",
                              title: "Logout failed",
                              description: error.message,
                            }),
                        })
                      }
                    />
                  )}
                </div>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
