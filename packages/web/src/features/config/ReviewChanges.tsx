import { CheckIcon, GitCompareArrowsIcon, RefreshCwIcon, Trash2Icon, XIcon } from "lucide-react";
import { useState } from "react";

import { ConfirmAction } from "~/components/ConfirmAction.tsx";
import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Pending } from "~/components/Pending.tsx";
import { Alert, AlertDescription, AlertTitle } from "~/components/ui/alert.tsx";
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
import { Spinner } from "~/components/ui/spinner.tsx";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "~/components/ui/tabs.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { StagedDiffs, useApply } from "~/features/config/apply.tsx";
import { useStaging } from "~/features/config/staging.ts";

/** The staged changes, the diff they make, and the apply that commits them at once. */
export function ReviewChanges() {
  const { changes, reviewing, review, unstage, clear } = useStaging();
  const [tab, setTab] = useState<"changes" | "diff">("changes");
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const definitions = useDaemonQuery(
    queryKeys.definitions,
    Daemon.use((daemon) => daemon.definitions),
  );
  const apply = useApply();
  const ready = configuration.isSuccess && definitions.isSuccess;
  return (
    <Sheet open={reviewing} onOpenChange={review}>
      <SheetContent className="data-[side=right]:sm:max-w-3xl">
        <SheetHeader>
          <SheetTitle>Staged changes</SheetTitle>
          <SheetDescription>
            Applied together: every change is committed, or none when the configuration changed
            since this diff.
          </SheetDescription>
        </SheetHeader>
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
          <Tabs
            value={tab}
            onValueChange={(next) => setTab(next === "diff" ? "diff" : "changes")}
            className="min-h-0 flex-1 px-4"
          >
            <TabsList>
              <TabsTrigger value="changes">Changes ({changes.length})</TabsTrigger>
              <TabsTrigger value="diff">Diff</TabsTrigger>
            </TabsList>
            <TabsContent value="changes" className="min-h-0">
              <ScrollArea className="h-full">
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
              </ScrollArea>
            </TabsContent>
            <TabsContent value="diff" className="min-h-0">
              <ScrollArea className="h-full">
                {definitions.isPending && <Pending className="h-40" />}
                {definitions.isError && <DaemonProblem error={definitions.error} />}
                {definitions.isSuccess && (
                  <StagedDiffs changes={changes} definitions={definitions.data} />
                )}
              </ScrollArea>
            </TabsContent>
          </Tabs>
        )}
        {apply.error !== null && (
          <div className="px-4">
            {apply.error.reason === "conflict" ? (
              <Alert>
                <RefreshCwIcon />
                <AlertTitle>Changed meanwhile</AlertTitle>
                <AlertDescription>
                  The configuration changed since this diff, which now shows the latest. Review it
                  and apply again.
                </AlertDescription>
              </Alert>
            ) : (
              <DaemonProblem error={apply.error} />
            )}
          </div>
        )}
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
            disabled={!ready || changes.length === 0 || apply.pending}
            onClick={() => {
              if (configuration.isSuccess) {
                apply.run(changes, configuration.data.revision, clear);
              }
            }}
          >
            {apply.pending ? (
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

/** Opens the staged changes; shown while there are any. */
export function StagedButton() {
  const count = useStaging((state) => state.changes.length);
  const review = useStaging((state) => state.review);
  if (count === 0) {
    return null;
  }
  return (
    <Button variant="outline" size="sm" onClick={() => review(true)}>
      <GitCompareArrowsIcon data-icon="inline-start" />
      Staged
      <Badge className="h-4 min-w-4 px-1">{count}</Badge>
    </Button>
  );
}
