import { useNavigate } from "@tanstack/react-router";
import { FolderIcon, GlobeIcon } from "lucide-react";
import { useState } from "react";

import { Button } from "~/components/ui/button.tsx";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "~/components/ui/dialog.tsx";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { DirectoryBrowser } from "~/features/scope/DirectoryBrowser.tsx";
import { useScopeDirectory, useScopeHarness } from "~/features/scope/scope.ts";
import { shortPath } from "~/lib/path.ts";

/**
 * The scope every page looks at: a directory, or everywhere, and the harness launching there,
 * which removes what it removes. A harness only matters within a directory.
 */
export function ScopeControl() {
  const directory = useScopeDirectory();
  const harness = useScopeHarness();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const home = configuration.data?.home ?? "";
  const harnesses = [
    { value: "", label: "Any harness" },
    ...(configuration.data?.harnesses ?? [])
      .filter((h) => !h.hidden)
      .map((h) => ({ value: h.name, label: h.name })),
  ];
  const scopeTo = (next: string) => {
    setOpen(false);
    void navigate({
      to: ".",
      search: (current) => ({
        ...current,
        dir: next === "" ? undefined : next,
        harness: next === "" ? undefined : current.harness,
      }),
    });
  };
  return (
    <div className="flex min-w-0 items-center rounded-md border">
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogTrigger
          render={
            <Button
              variant="ghost"
              size="sm"
              className="min-w-0 max-w-72 justify-start rounded-r-none font-mono"
            >
              {directory === "" ? (
                <GlobeIcon data-icon="inline-start" />
              ) : (
                <FolderIcon data-icon="inline-start" />
              )}
              <span className="truncate">
                {directory === "" ? "Everywhere" : shortPath(directory, home)}
              </span>
            </Button>
          }
        />
        <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-3xl">
          <DialogHeader className="border-b px-4 py-3">
            <DialogTitle>Scope</DialogTitle>
          </DialogHeader>
          <DirectoryBrowser
            key={String(open)}
            value={directory}
            home={home}
            shortcuts={configuration.data?.scopeDirectories ?? []}
            everywhere
            wide
            onPick={scopeTo}
          />
        </DialogContent>
      </Dialog>
      {directory !== "" && (
        <Select
          items={harnesses}
          value={harness}
          onValueChange={(next) =>
            void navigate({
              to: ".",
              search: (current) => ({
                ...current,
                harness: next === null || next === "" ? undefined : next,
              }),
            })
          }
        >
          <SelectTrigger
            size="sm"
            aria-label="Harness"
            className="rounded-l-none border-0 border-l bg-transparent dark:bg-transparent"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {harnesses.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      )}
    </div>
  );
}
