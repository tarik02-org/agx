import { useQueryClient } from "@tanstack/react-query";
import { lazy, Suspense } from "react";
import { CheckIcon, GitCompareArrowsIcon, Trash2Icon, XIcon } from "lucide-react";

import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { Button } from "~/components/ui/button.tsx";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "~/components/ui/empty.tsx";
import { Item, ItemActions, ItemContent, ItemGroup, ItemTitle } from "~/components/ui/item.tsx";
import { ScrollArea } from "~/components/ui/scroll-area.tsx";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "~/components/ui/sheet.tsx";
import { Pending } from "~/components/Pending.tsx";
import { Spinner } from "~/components/ui/spinner.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonMutation, useDaemonQuery } from "~/daemon/query.ts";
import { useStaging } from "~/features/config/staging.ts";
import { shortPath } from "~/lib/path.ts";
import { ConfirmAction } from "~/components/ConfirmAction.tsx";

const PlannedDiff = lazy(() => import("~/features/config/PlannedDiff.tsx"));

/** The staged changes, the diff they make, and the apply that writes them at once. */
export function ReviewChanges() {
  const { changes, reviewing, review, unstage, clear } = useStaging();
  const client = useQueryClient();
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const home = configuration.data?.home ?? "";
  const plan = useDaemonQuery(
    ["plan", changes],
    Daemon.use((daemon) => daemon.planChanges(changes.map((change) => change.change))),
    { enabled: reviewing && changes.length > 0 },
  );
  const apply = useDaemonMutation(
    (expected: NonNullable<typeof plan.data>) =>
      Daemon.use((daemon) =>
        daemon.applyChanges(
          changes.map((change) => change.change),
          expected,
        ),
      ),
    [],
  );
  return (
    <Sheet open={reviewing} onOpenChange={review}>
      <SheetContent className="data-[side=right]:sm:max-w-3xl">
        <SheetHeader>
          <SheetTitle>Staged changes</SheetTitle>
          <SheetDescription>
            Applied together: every file is written, or none when one changed since this diff.
          </SheetDescription>
        </SheetHeader>
        <ScrollArea className="min-h-0 flex-1">
          <div className="flex flex-col gap-4 px-4">
            {changes.length === 0 ? (
              <Empty>
                <EmptyHeader>
                  <EmptyMedia variant="icon">
                    <GitCompareArrowsIcon />
                  </EmptyMedia>
                  <EmptyTitle>Nothing staged</EmptyTitle>
                  <EmptyDescription>
                    Edits from the Skills and Configuration pages wait here until you apply them.
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : (
              <ItemGroup>
                {changes.map((change) => (
                  <Item key={change.key} size="sm" variant="outline">
                    <ItemContent>
                      <ItemTitle className="font-mono">{change.label}</ItemTitle>
                    </ItemContent>
                    <ItemActions>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={`Unstage ${change.label}`}
                        onClick={() => unstage(change.key)}
                      >
                        <XIcon />
                      </Button>
                    </ItemActions>
                  </Item>
                ))}
              </ItemGroup>
            )}
            {plan.isPending && changes.length > 0 && <Pending className="h-40" />}
            {plan.isError && <DaemonProblem error={plan.error} />}
            {apply.isError && <DaemonProblem error={apply.error} />}
            {plan.isSuccess &&
              changes.length > 0 &&
              plan.data.map((file) => (
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
            {plan.isSuccess && changes.length > 0 && plan.data.length === 0 && (
              <p className="text-muted-foreground text-xs">
                These changes leave every file as it is.
              </p>
            )}
          </div>
        </ScrollArea>
        <SheetFooter className="flex-row justify-end">
          <ConfirmAction
            trigger={
              <Button variant="outline" disabled={changes.length === 0}>
                <Trash2Icon data-icon="inline-start" />
                Discard all
              </Button>
            }
            title="Discard every staged change?"
            description="Nothing was written yet; the edits are lost."
            action="Discard all"
            onConfirm={clear}
          />
          <Button
            disabled={!plan.isSuccess || changes.length === 0 || apply.isPending}
            onClick={() => {
              if (!plan.isSuccess) {
                return;
              }
              apply.mutate(plan.data, {
                onSuccess: (written) => {
                  clear();
                  toast.add({
                    type: "success",
                    title: `Applied; wrote ${written.length} ${written.length === 1 ? "file" : "files"}`,
                  });
                  // Every read may show configuration the apply changed.
                  void client.invalidateQueries();
                },
                onError: (error) => {
                  if (error.reason === "conflict" || error.reason === "failed_precondition") {
                    void plan.refetch();
                  }
                },
              });
            }}
          >
            {apply.isPending ? (
              <Spinner data-icon="inline-start" />
            ) : (
              <CheckIcon data-icon="inline-start" />
            )}
            Apply
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
