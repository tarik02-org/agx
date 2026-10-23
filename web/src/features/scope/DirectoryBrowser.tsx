import { cn } from "cn";
import {
  ArrowUpIcon,
  CheckIcon,
  ChevronRightIcon,
  FolderIcon,
  GlobeIcon,
  HouseIcon,
} from "lucide-react";
import { type ReactNode, useState } from "react";

import { Button } from "~/components/ui/button.tsx";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "~/components/ui/input-group.tsx";
import { ScrollArea } from "~/components/ui/scroll-area.tsx";
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
  /** Shortcuts in a sidebar beside the folders, for a dialog; above them otherwise. */
  readonly wide?: boolean;
  readonly onPick: (directory: string) => void;
}

const withSlash = (directory: string): string =>
  directory.endsWith("/") ? directory : `${directory}/`;

const withoutSlash = (path: string): string => path.replace(/(?<=.)\/+$/, "");

const parentOf = (directory: string): string => {
  const parent = directory.slice(0, directory.lastIndexOf("/"));
  return parent === "" ? "/" : parent;
};

/**
 * Picks a directory: one of the shortcuts, or one reached by browsing. The address bar is the
 * folder being browsed, editable: typing narrows the folders to those starting with what follows
 * the last slash, and a pasted path goes straight there.
 */
export function DirectoryBrowser({
  value,
  home,
  shortcuts,
  everywhere = false,
  wide = false,
  onPick,
}: DirectoryBrowserProps) {
  // The address, as typed or browsed to, ~/… under home.
  const [address, setAddress] = useState(withSlash(shortPath(value === "" ? home : value, home)));
  const isPath = address.startsWith("/") || address.startsWith("~/");
  const listing = useDaemonQuery(
    queryKeys.completeDirectory(address),
    Daemon.use((daemon) => daemon.completeDirectory(address)),
    { enabled: isPath },
  );
  const typed = absoluteDirectory(withoutSlash(address), home);
  // A browsed folder ends in a slash; a typed one counts once the daemon lists it.
  const folder =
    isPath && (address.endsWith("/") || listing.data?.includes(typed) === true) ? typed : undefined;
  const browse = (directory: string) => setAddress(withSlash(shortPath(directory, home)));
  const places = [
    ...new Set([home, ...shortcuts.map((directory) => absoluteDirectory(directory, home))]),
  ].filter((directory) => directory !== "");

  const placeList = (
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
          onClick={() => (wide ? browse(directory) : onPick(directory))}
        >
          {shortPath(directory, home)}
        </Place>
      ))}
    </div>
  );

  return (
    <div className={cn("flex min-h-0", wide ? "h-[28rem] flex-row" : "flex-col")}>
      <nav
        aria-label="Shortcuts"
        className={cn(
          "shrink-0",
          wide ? "w-48 overflow-y-auto border-r bg-muted/30 py-1" : "border-b",
        )}
      >
        {wide && (
          <p className="px-3 pt-1.5 pb-1 text-[0.625rem] text-muted-foreground uppercase tracking-wider">
            Scopes
          </p>
        )}
        {placeList}
      </nav>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <div className="p-2">
          <InputGroup>
            <InputGroupAddon>
              <InputGroupButton
                size="icon-xs"
                aria-label="Parent folder"
                disabled={folder === "/" || !isPath}
                onClick={() => browse(parentOf(typed))}
              >
                <ArrowUpIcon />
              </InputGroupButton>
            </InputGroupAddon>
            <InputGroupInput
              aria-label="Path"
              placeholder="/path/to/directory"
              autoComplete="off"
              spellCheck={false}
              className="font-mono"
              value={address}
              onChange={(event) => setAddress(event.target.value)}
              onKeyDown={(event) => {
                // Enter goes into the typed folder, or the only one matching it.
                const only = listing.data?.length === 1 ? listing.data[0] : undefined;
                if (event.key === "Enter" && folder !== undefined) {
                  browse(folder);
                } else if (event.key === "Enter" && only !== undefined) {
                  browse(only);
                }
              }}
            />
            {listing.isFetching && (
              <InputGroupAddon align="inline-end">
                <Spinner className="size-3" />
              </InputGroupAddon>
            )}
          </InputGroup>
        </div>
        <ScrollArea className={cn("min-h-0", wide ? "flex-1" : "h-56")}>
          <div className="flex flex-col gap-px px-1 pb-1">
            {(listing.data ?? [])
              .filter((child) => child !== typed)
              .map((child) => (
                <button
                  key={child}
                  type="button"
                  className="flex items-center gap-2 rounded-sm px-2 py-1 text-left font-mono text-xs hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
                  onClick={() => browse(child)}
                >
                  <FolderIcon className="size-3.5 shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1 truncate">
                    {child.slice(child.lastIndexOf("/") + 1)}
                  </span>
                  <ChevronRightIcon className="size-3.5 shrink-0 text-muted-foreground" />
                </button>
              ))}
            {listing.isSuccess && listing.data.filter((child) => child !== typed).length === 0 && (
              <p className="px-2 py-6 text-center text-muted-foreground text-xs">
                {address.endsWith("/") ? "No folders here." : "No matching folders."}
              </p>
            )}
            {!isPath && (
              <p className="px-2 py-6 text-center text-muted-foreground text-xs">
                Paths start with / or ~/.
              </p>
            )}
          </div>
        </ScrollArea>
        <div className="flex items-center gap-2 border-t p-2">
          <Button
            size="sm"
            className={cn("min-w-0", !wide && "w-full", wide && "ml-auto")}
            disabled={folder === undefined}
            onClick={() => folder !== undefined && onPick(folder)}
          >
            <span className="min-w-0 truncate">
              Use{" "}
              <span className="font-mono">
                {folder === undefined ? "…" : shortPath(folder, home)}
              </span>
            </span>
          </Button>
        </div>
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
