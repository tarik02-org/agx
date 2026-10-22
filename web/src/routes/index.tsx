import { createFileRoute } from "@tanstack/react-router";

import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Pending } from "~/components/Pending.tsx";
import { useRuntimeView } from "~/daemon/runtimeView.ts";
import { InstallButton } from "~/features/runtime/InstallButton.tsx";
import { LaunchesCard } from "~/features/runtime/LaunchesCard.tsx";
import { ProcessesCard } from "~/features/runtime/ProcessesCard.tsx";
import { SecretsCard } from "~/features/runtime/SecretsCard.tsx";
import { pageTitle } from "~/lib/title.ts";

export const Route = createFileRoute("/")({
  component: RuntimePage,
});

function RuntimePage() {
  const state = useRuntimeView();
  return (
    <div className="flex flex-col gap-4 p-4">
      <title>{pageTitle("Runtime")}</title>
      <header className="flex items-center gap-3">
        <h1 className="font-heading font-semibold text-sm">Runtime</h1>
        {state._tag === "Live" && (
          <span className="font-mono text-muted-foreground text-xs">
            daemon {state.view.version} · {state.view.daemonInstanceId.slice(0, 12)}
          </span>
        )}
        <div className="ml-auto">
          <InstallButton />
        </div>
      </header>
      {state._tag === "Connecting" && <Pending className="h-40" />}
      {state._tag === "Failed" && <DaemonProblem error={state.error} />}
      {state._tag === "Live" && (
        <>
          <LaunchesCard launches={state.view.launches} />
          <ProcessesCard />
          <SecretsCard />
        </>
      )}
    </div>
  );
}
