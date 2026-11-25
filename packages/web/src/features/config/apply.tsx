import { useQueryClient } from "@tanstack/react-query";
import type { Definition } from "agx-client/protocol";
import { lazy, Suspense } from "react";

import { Pending } from "~/components/Pending.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { useDaemonMutation } from "~/daemon/query.ts";
import { definitionValue, valueText } from "~/features/config/definitionValue.ts";
import type { StagedChange } from "~/features/config/staging.ts";

const ValueDiff = lazy(() => import("~/features/config/ValueDiff.tsx"));

/**
 * Commits changes at once, or none when the local configuration moved past revision, the one
 * the reviewed definitions came with. Every read is refreshed afterwards, since any page may
 * show what changed; after a conflict that shows the diff against what is there now.
 */
export const useApply = () => {
  const client = useQueryClient();
  const apply = useDaemonMutation(
    ({ changes, revision }: { changes: ReadonlyArray<StagedChange>; revision: bigint }) =>
      Daemon.use((daemon) =>
        daemon.updateConfiguration(
          changes.map((change) => change.change),
          revision,
        ),
      ),
    [],
  );
  const run = (changes: ReadonlyArray<StagedChange>, revision: bigint, onApplied: () => void) =>
    apply.mutate(
      { changes, revision },
      {
        onSuccess: () => {
          onApplied();
          toast.add({
            type: "success",
            title: `Applied ${changes.length} ${changes.length === 1 ? "change" : "changes"}`,
          });
        },
        onSettled: () => void client.invalidateQueries(),
      },
    );
  return { run, pending: apply.isPending, error: apply.error };
};

/** What each staged change does to a definition, with its value's diff where it has one. */
export function StagedDiffs({
  changes,
  definitions,
}: {
  readonly changes: ReadonlyArray<StagedChange>;
  readonly definitions: ReadonlyArray<Definition>;
}) {
  return (
    <div className="flex flex-col gap-4">
      {changes.map((staged) => {
        const change = staged.change.change;
        const target =
          change?.case === "define" || change?.case === "undefine"
            ? definitions.find((d) => d.kind === change.value.kind && d.id === change.value.id)
            : undefined;
        const before = target === undefined ? "" : valueText(definitionValue(target));
        const after = change?.case === "define" ? valueText(staged.value) : "";
        return (
          <div key={staged.key} className="flex flex-col gap-1">
            <div className="flex items-center gap-2 font-mono text-xs">
              <span className="truncate">{staged.label}</span>
              {change?.case === "define" && target === undefined && (
                <Badge variant="secondary">new</Badge>
              )}
            </div>
            {(change?.case === "define" || change?.case === "undefine") && (
              <Suspense fallback={<Pending className="h-24" />}>
                <ValueDiff name={staged.key} before={before} after={after} />
              </Suspense>
            )}
          </div>
        );
      })}
    </div>
  );
}
