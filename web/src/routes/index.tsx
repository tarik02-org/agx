import { createFileRoute } from "@tanstack/react-router";

import { Page } from "~/components/Page.tsx";
import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Pending } from "~/components/Pending.tsx";
import { useRuntimeView } from "~/daemon/runtimeView.ts";
import { InstallButton } from "~/features/runtime/InstallButton.tsx";
import { LaunchesCard } from "~/features/runtime/LaunchesCard.tsx";
import { RuntimeOverview } from "~/features/runtime/Overview.tsx";
import { ProcessesCard } from "~/features/runtime/ProcessesCard.tsx";
import { SecretsCard } from "~/features/runtime/SecretsCard.tsx";

export const Route = createFileRoute("/")({
  component: RuntimePage,
});

function RuntimePage() {
  const state = useRuntimeView();
  return (
    <Page
      title={["Runtime"]}
      crumbs={[{ label: "Runtime" }]}
      details={
        state._tag === "Live" && (
          <span className="truncate font-mono text-muted-foreground text-xs">
            daemon {state.view.version} · {state.view.daemonInstanceId.slice(0, 12)}
          </span>
        )
      }
      actions={<InstallButton />}
    >
      {state._tag === "Connecting" && <Pending className="h-40" />}
      {state._tag === "Failed" && <DaemonProblem error={state.error} />}
      {state._tag === "Live" && (
        <>
          <RuntimeOverview view={state.view} />
          <div className="grid gap-2 xl:grid-cols-2">
            <LaunchesCard launches={state.view.launches} />
            <ProcessesCard />
          </div>
          <SecretsCard />
        </>
      )}
    </Page>
  );
}
