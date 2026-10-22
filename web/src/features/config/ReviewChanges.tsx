import { CheckIcon, GitCompareArrowsIcon, Trash2Icon, XIcon } from "lucide-react";
import { useState } from "react";

import { ConfirmAction } from "~/components/ConfirmAction.tsx";
import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Pending } from "~/components/Pending.tsx";
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
import { PlannedFiles, useApply, usePlan } from "~/features/config/plan.tsx";
import { useStaging } from "~/features/config/staging.ts";

/** The staged changes, the diff they make, and the apply that writes them at once. */
export function ReviewChanges() {
  const { changes, reviewing, review, unstage, clear } = useStaging();
  const [tab, setTab] = useState<"changes" | "files">("changes");
  const plan = usePlan(
    changes.map((change) => change.change),
    reviewing,
  );
  const apply = useApply(changes.map((change) => change.change));
  return (
    <Sheet open={reviewing} onOpenChange={review}>
      <SheetContent className="data-[side=right]:sm:max-w-3xl">
        <SheetHeader>
          <SheetTitle>Staged changes</SheetTitle>
          <SheetDescription>
            Applied together: every file is written, or none when one changed since this diff.
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
            onValueChange={(next) => setTab(next === "files" ? "files" : "changes")}
            className="min-h-0 flex-1 px-4"
          >
            <TabsList>
              <TabsTrigger value="changes">Changes ({changes.length})</TabsTrigger>
              <TabsTrigger value="files">
                Files{plan.isSuccess && ` (${plan.data.length})`}
              </TabsTrigger>
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
            <TabsContent value="files" className="min-h-0">
              <ScrollArea className="h-full">
                {plan.isPending && <Pending className="h-40" />}
                {plan.isError && <DaemonProblem error={plan.error} />}
                {plan.isSuccess && <PlannedFiles files={plan.data} />}
              </ScrollArea>
            </TabsContent>
          </Tabs>
        )}
        {apply.error !== null && (
          <div className="px-4">
            <DaemonProblem error={apply.error} />
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
            disabled={!plan.isSuccess || changes.length === 0 || apply.pending}
            onClick={() => {
              if (plan.isSuccess) {
                apply.run(plan.data, clear);
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
