import { cn } from "cn";
import {
  ArrowUpIcon,
  CheckIcon,
  ChevronRightIcon,
  FolderIcon,
  GlobeIcon,
  HouseIcon,
  SearchIcon,
} from "lucide-react";
import { type ReactNode, useState } from "react";

import { Button } from "~/components/ui/button.tsx";
import { InputGroup, InputGroupAddon, InputGroupInput } from "~/components/ui/input-group.tsx";
import { ScrollArea } from "~/components/ui/scroll-area.tsx";
import { Separator } from "~/components/ui/separator.tsx";
import { Spinner } from "~/components/ui/spinner.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { absoluteDirectory } from "~/features/scope/scope.ts";
import { shortPath } from "~/lib/path.ts";

interface DirectoryBrowserProps {
  /** The picked directory, absolute, or empty for everywhere. */
  readonly value: string;
  readonly home: string;
  /** Directories offered at once, such as those scope rules name; absolute or ~/…. */
  readonly shortcuts: ReadonlyArray<string>;
  /** Offers everywhere, the empty directory, among the shortcuts. */
  readonly everywhere?: boolean;
  readonly onPick: (directory: string) => void;
}

const parentOf = (directory: string): string => {
  const parent = directory.slice(0, directory.lastIndexOf("/"));
  return parent === "" ? "/" : parent;
};

const isPath = (text: string): boolean => text.startsWith("/") || text.startsWith("~/");

/**
 * Picks a directory: one of the shortcuts, or one found by browsing folders from the current one.
 * The filter narrows the folder list; a path typed there jumps to it.
 */
export function DirectoryBrowser({
  value,
  home,
  shortcuts,
  everywhere = false,
  onPick,
}: DirectoryBrowserProps) {
  const [folder, setFolder] = useState(value === "" ? home : value);
  const [filter, setFilter] = useState("");
  // A typed path lists that path's parent, narrowed to its last segment.
  const prefix = isPath(filter) ? filter : `${folder === "/" ? "" : folder}/${filter}`;
  const children = useDaemonQuery(
    queryKeys.completeDirectory(prefix),
    Daemon.use((daemon) => daemon.completeDirectory(prefix)),
    { enabled: folder !== "" },
  );
  const open = (next: string) => {
    setFolder(next);
    setFilter("");
  };
  const places = [
    ...new Set([home, ...shortcuts.map((directory) => absoluteDirectory(directory, home))]),
  ].filter((directory) => directory !== "");

  return (
    <div className="flex flex-col">
      <div className="flex flex-col gap-px p-1">
        {everywhere && (
          <Place icon={<GlobeIcon />} picked={value === ""} onClick={() => onPick("")}>
            Everywhere
          </Place>
        )}
        {places.map((directory) => (
          <Place
            key={directory}
            icon={directory === home ? <HouseIcon /> : <FolderIcon />}
            picked={value === directory}
            mono
            onClick={() => onPick(directory)}
          >
            {shortPath(directory, home)}
          </Place>
        ))}
      </div>
      <Separator />
      <div className="flex items-center gap-1 px-2 pt-2">
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Parent folder"
          disabled={folder === "/"}
          onClick={() => open(parentOf(folder))}
        >
          <ArrowUpIcon />
        </Button>
        <span className="min-w-0 flex-1 truncate font-mono text-xs" title={folder}>
          {shortPath(folder, home)}
        </span>
      </div>
      <div className="px-2 pt-1.5 pb-1">
        <InputGroup>
          <InputGroupAddon>
            <SearchIcon />
          </InputGroupAddon>
          <InputGroupInput
            aria-label="Filter folders, or type a path"
            placeholder="Filter, or type a path…"
            autoComplete="off"
            spellCheck={false}
            className="font-mono"
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
            onKeyDown={(event) => {
              // Enter opens the only match, or the typed path.
              const only = children.data?.length === 1 ? children.data[0] : undefined;
              if (event.key === "Enter" && only !== undefined) {
                open(only);
              } else if (event.key === "Enter" && isPath(filter)) {
                open(absoluteDirectory(filter.replace(/\/+$/, ""), home) || "/");
              }
            }}
          />
          {children.isFetching && (
            <InputGroupAddon align="inline-end">
              <Spinner className="size-3" />
            </InputGroupAddon>
          )}
        </InputGroup>
      </div>
      <ScrollArea className="h-56">
        <div className="flex flex-col gap-px p-1">
          {(children.data ?? []).map((child) => (
            <button
              key={child}
              type="button"
              className="flex items-center gap-2 rounded-sm px-2 py-1 text-left font-mono text-xs hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
              onClick={() => open(child)}
            >
              <FolderIcon className="size-3.5 shrink-0 text-muted-foreground" />
              <span className="min-w-0 flex-1 truncate">
                {child.slice(child.lastIndexOf("/") + 1)}
              </span>
              <ChevronRightIcon className="size-3.5 shrink-0 text-muted-foreground" />
            </button>
          ))}
          {children.isSuccess && children.data.length === 0 && (
            <p className="px-2 py-6 text-center text-muted-foreground text-xs">No folders here.</p>
          )}
        </div>
      </ScrollArea>
      <Separator />
      <div className="p-2">
        <Button size="sm" className="w-full" onClick={() => onPick(folder)}>
          <span className="min-w-0 truncate">
            Use <span className="font-mono">{shortPath(folder, home)}</span>
          </span>
        </Button>
      </div>
    </div>
  );
}

function Place({
  icon,
  picked,
  mono = false,
  onClick,
  children,
}: {
  readonly icon: ReactNode;
  readonly picked: boolean;
  readonly mono?: boolean;
  readonly onClick: () => void;
  readonly children: ReactNode;
}) {
  return (
    <button
      type="button"
      aria-pressed={picked}
      className={cn(
        "flex items-center gap-2 rounded-sm px-2 py-1 text-left text-xs hover:bg-accent focus-visible:bg-accent focus-visible:outline-none [&_svg]:size-3.5 [&_svg]:shrink-0 [&_svg]:text-muted-foreground",
        mono && "font-mono",
      )}
      onClick={onClick}
    >
      {icon}
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {picked && <CheckIcon />}
    </button>
  );
}
