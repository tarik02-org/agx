import { useQueryClient } from "@tanstack/react-query";
import { lazy, Suspense, useState } from "react";

import { Pending } from "~/components/Pending.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { Button } from "~/components/ui/button.tsx";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "~/components/ui/field.tsx";
import { Input } from "~/components/ui/input.tsx";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { type ConfigChangeInit, Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonMutation, useDaemonQuery } from "~/daemon/query.ts";
import type { PlannedFile } from "~/gen/agx/v1/agx_pb.ts";
import { shortPath } from "~/lib/path.ts";

const PlannedDiff = lazy(() => import("~/features/config/PlannedDiff.tsx"));

/**
 * What changes would write, file by file, without writing anything. New definitions, rules, and
 * packages go to file; when it is empty and several fragments could take them, the plan fails
 * with choose_file.
 */
export const usePlan = (changes: ReadonlyArray<ConfigChangeInit>, file: string, enabled: boolean) =>
  useDaemonQuery(
    ["plan", changes, file],
    Daemon.use((daemon) => daemon.planChanges(changes, file)),
    { enabled: enabled && changes.length > 0 },
  );

/**
 * Writes changes as planned: every file, or none when one changed since the plan. Every read is
 * refreshed afterwards, since any page may show what changed.
 */
export const useApply = (changes: ReadonlyArray<ConfigChangeInit>, file: string) => {
  const client = useQueryClient();
  const apply = useDaemonMutation(
    (expected: ReadonlyArray<PlannedFile>) =>
      Daemon.use((daemon) => daemon.applyChanges(changes, file, expected)),
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

const newFile = "new";

/**
 * Picks the fragment new definitions, rules, and packages go to: one the daemon offered, or a new
 * file, relative to the root configuration's directory, that the root's globs match.
 */
export function ChooseFile({
  files,
  home,
  onChoose,
}: {
  readonly files: ReadonlyArray<string>;
  readonly home: string;
  readonly onChoose: (file: string) => void;
}) {
  const [choice, setChoice] = useState(files[0] ?? newFile);
  const [name, setName] = useState("agx.d/main.yaml");
  const items = [
    ...files.map((file) => ({ value: file, label: shortPath(file, home) })),
    { value: newFile, label: "New file…" },
  ];
  const file = choice === newFile ? name.trim() : choice;
  return (
    <FieldGroup>
      <Field>
        <FieldLabel>Write new definitions to</FieldLabel>
        <Select items={items} value={choice} onValueChange={(next) => setChoice(next ?? newFile)}>
          <SelectTrigger className="font-mono">
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {items.map((item) => (
                <SelectItem key={item.value} value={item.value} className="font-mono">
                  {item.label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </Field>
      {choice === newFile && (
        <Field>
          <FieldLabel htmlFor="new-fragment">New file</FieldLabel>
          <Input
            id="new-fragment"
            className="font-mono"
            spellCheck={false}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
          <FieldDescription>
            Relative to the root configuration; the root must list it or a glob matching it.
          </FieldDescription>
        </Field>
      )}
      <Field orientation="horizontal">
        <Button size="sm" disabled={file === ""} onClick={() => onChoose(file)}>
          Use this file
        </Button>
      </Field>
    </FieldGroup>
  );
}
