import { useQueryClient } from "@tanstack/react-query";
import { lazy, Suspense } from "react";

import { Pending } from "~/components/Pending.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { type ConfigChangeInit, Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonMutation, useDaemonQuery } from "~/daemon/query.ts";
import type { PlannedFile } from "~/gen/agx/v1/agx_pb.ts";
import { shortPath } from "~/lib/path.ts";

const PlannedDiff = lazy(() => import("~/features/config/PlannedDiff.tsx"));

/** What changes would write, file by file, without writing anything. */
export const usePlan = (changes: ReadonlyArray<ConfigChangeInit>, enabled: boolean) =>
  useDaemonQuery(
    ["plan", changes],
    Daemon.use((daemon) => daemon.planChanges(changes)),
    { enabled: enabled && changes.length > 0 },
  );

/**
 * Writes changes as planned: every file, or none when one changed since the plan. Every read is
 * refreshed afterwards, since any page may show what changed.
 */
export const useApply = (changes: ReadonlyArray<ConfigChangeInit>) => {
  const client = useQueryClient();
  const apply = useDaemonMutation(
    (expected: ReadonlyArray<PlannedFile>) =>
      Daemon.use((daemon) => daemon.applyChanges(changes, expected)),
    [],
  );
  const run = (plan: ReadonlyArray<PlannedFile>, onApplied: () => void) =>
    apply.mutate(plan, {
      onSuccess: (written) => {
        onApplied();
        toast.add({
          type: "success",
          title: `Applied; wrote ${written.length} ${written.length === 1 ? "file" : "files"}`,
        });
        void client.invalidateQueries();
      },
      onError: (error) => {
        if (error.reason === "conflict" || error.reason === "failed_precondition") {
          void client.invalidateQueries({ queryKey: ["plan"] });
        }
      },
    });
  return { run, pending: apply.isPending, error: apply.error };
};

/** The diff of every planned file. */
export function PlannedFiles({ files }: { readonly files: ReadonlyArray<PlannedFile> }) {
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const home = configuration.data?.home ?? "";
  if (files.length === 0) {
    return (
      <p className="text-muted-foreground text-xs">These changes leave every file as it is.</p>
    );
  }
  return (
    <div className="flex flex-col gap-4">
      {files.map((file) => (
        <div key={file.path} className="flex flex-col gap-1">
          <div className="flex items-center gap-2 font-mono text-xs">
            <span className="truncate">{shortPath(file.path, home)}</span>
            {file.beforeSha256 === "" && <Badge variant="secondary">new</Badge>}
          </div>
          <Suspense fallback={<Pending className="h-24" />}>
            <PlannedDiff diff={file.diff} />
          </Suspense>
        </div>
      ))}
    </div>
  );
}
