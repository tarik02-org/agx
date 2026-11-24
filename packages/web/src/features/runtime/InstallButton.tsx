import { useQueryClient } from "@tanstack/react-query";
import * as Effect from "effect/Effect";
import * as Stream from "effect/Stream";
import { PackageIcon } from "lucide-react";

import { Button } from "~/components/ui/button.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { useEffectCallback } from "~/effect/react.tsx";

/** Runs agx install in the daemon, with its progress in one toast. */
export function InstallButton() {
  const client = useQueryClient();
  const install = useEffectCallback(
    () =>
      Effect.gen(function* () {
        const id = toast.add({ type: "loading", title: "Installing packages", timeout: 0 });
        yield* Daemon.use((daemon) => Effect.succeed(daemon.install)).pipe(
          Stream.unwrap,
          Stream.runForEach((event) =>
            Effect.sync(() => {
              switch (event.state.case) {
                case "progress":
                  toast.update(id, {
                    description: `${event.state.value.packageId} (${event.state.value.completed}/${event.state.value.total})`,
                  });
                  return;
                case "result":
                  toast.update(id, {
                    type: "success",
                    title: "Packages installed",
                    description: `${event.state.value.packageIds.length} packages`,
                    timeout: 5000,
                  });
                  return;
                case "failure":
                  toast.update(id, {
                    type: "error",
                    title: "Installation failed",
                    description: event.state.value.summary,
                    timeout: 0,
                  });
                  return;
                case undefined:
                  return;
              }
            }),
          ),
          Effect.catchTag("DaemonError", (error) =>
            Effect.sync(() =>
              toast.update(id, {
                type: "error",
                title: "Installation failed",
                description: error.message,
                timeout: 0,
              }),
            ),
          ),
          Effect.ensuring(Effect.promise(() => client.invalidateQueries())),
        );
      }),
    [client],
  );
  return (
    <Button variant="outline" size="sm" onClick={() => install()}>
      <PackageIcon data-icon="inline-start" />
      Install packages
    </Button>
  );
}
