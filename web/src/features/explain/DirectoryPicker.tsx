import { cn } from "cn";
import { useState } from "react";

import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "~/components/ui/combobox.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";

interface DirectoryPickerProps {
  readonly value: string;
  /** Directories offered before anything is typed, such as those scope rules name. */
  readonly suggestions: ReadonlyArray<string>;
  readonly onChange: (directory: string) => void;
  readonly className?: string;
}

const isPath = (text: string): boolean => text.startsWith("/") || text.startsWith("~/");

/** Picks a directory: a suggestion, a completion of what is typed, or the typed path itself. */
export function DirectoryPicker({ value, suggestions, onChange, className }: DirectoryPickerProps) {
  const [query, setQuery] = useState(value);
  const completions = useDaemonQuery(
    queryKeys.completeDirectory(query),
    Daemon.use((daemon) => daemon.completeDirectory(query)),
    { enabled: isPath(query) && query !== value },
  );
  const items = [
    ...(isPath(query) && query !== value ? [query] : []),
    ...(query === value || !isPath(query) ? suggestions : (completions.data ?? [])),
  ].filter((item, index, all) => all.indexOf(item) === index);

  return (
    <Combobox
      items={items}
      filter={null}
      value={value}
      inputValue={query}
      onInputValueChange={(next) => setQuery(next)}
      onValueChange={(next) => {
        if (typeof next === "string" && next !== "") {
          setQuery(next);
          onChange(next);
        }
      }}
    >
      <ComboboxInput
        aria-label="Directory"
        placeholder="~/work/project…"
        className={cn("w-[32rem] font-mono", className)}
      />
      <ComboboxContent>
        <ComboboxEmpty>No matching directories.</ComboboxEmpty>
        <ComboboxList>
          {(item: string) => (
            <ComboboxItem key={item} value={item} className="font-mono">
              {item}
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  );
}
