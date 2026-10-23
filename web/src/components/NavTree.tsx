import type {
  FileTreeBatchOperation,
  FileTreeDirectoryHandle,
  FileTreeItemHandle,
  FileTreeRowDecoration,
  FileTreeSortEntry,
} from "@pierre/trees";
import { FileTree, useFileTree } from "@pierre/trees/react";
import { type ReactNode, useEffect, useLayoutEffect, useRef } from "react";

interface NavTreeProps {
  /** /-separated paths; a path's leading segments are its folders. */
  readonly paths: ReadonlyArray<string>;
  /**
   * Folders whose children load when they are first expanded, by whether they have loaded. Until
   * then each holds a placeholder row.
   */
  readonly lazy?: ReadonlyMap<string, boolean>;
  readonly onExpand?: (folder: string) => void;
  /** The path of what the page shows, selected in the tree. */
  readonly selected: string | undefined;
  /** Picking a row: a leaf, or a folder that stands for something, such as a skill. */
  readonly onSelect: (path: string) => void;
  /** Short notes after rows' names, such as a state, by path. */
  readonly decorations?: ReadonlyMap<string, string>;
  readonly header?: ReactNode;
  /** Shows only rows matching this, and their folders; empty shows every row. */
  readonly query?: string;
  /** CSS mask images, from iconMask, drawn before folders' names, by folder path. */
  readonly folderIcons?: ReadonlyMap<string, string>;
  /** Top-level folders in the order to show them, such as the configuration's sections. */
  readonly order?: ReadonlyArray<string>;
}

const placeholder = "loading…";

// Top-level folders are sections, such as the configuration's kinds: set as small headings so
// what they hold stands out. The tree draws in a shadow root; custom properties reach it.
const rootRowsAsHeadings = `
  [data-type="item"][aria-level="1"] {
    color: var(--muted-foreground);
    font-family: var(--font-sans);
    font-size: 10.5px;
    font-weight: 500;
    letter-spacing: 0.05em;
    text-transform: uppercase;
  }
`;
const noDecorations: ReadonlyMap<string, string> = new Map();
const noFolders: ReadonlyMap<string, boolean> = new Map();
const noOrder: ReadonlyArray<string> = [];
const noIcons: ReadonlyMap<string, string> = new Map();

/** Draws each folder's icon before its name; the tree reports folders' paths with a slash. */
const folderIconRules = (icons: ReadonlyMap<string, string>): string =>
  [...icons]
    .map(
      ([folder, mask]) => `
  [data-item-path="${folder}/"] [data-item-section="content"] {
    display: flex;
    align-items: center;
  }
  [data-item-path="${folder}/"] [data-item-section="content"]::before {
    content: "";
    flex-shrink: 0;
    width: 12px;
    height: 12px;
    margin-right: 6px;
    background-color: currentColor;
    mask: ${mask} center / contain no-repeat;
  }`,
    )
    .join("");

// The tree reports folders with a trailing slash.
const bare = (path: string) => (path.endsWith("/") ? path.slice(0, -1) : path);

const rank = (order: ReadonlyArray<string>, name: string | undefined) => {
  const index = name === undefined ? -1 : order.indexOf(name);
  return index === -1 ? order.length : index;
};

/** Top-level folders in the given order; below them, folders first, then by name. */
const compareIn =
  (order: ReadonlyArray<string>) =>
  (left: FileTreeSortEntry, right: FileTreeSortEntry): number => {
    const byOrder = rank(order, left.segments[0]) - rank(order, right.segments[0]);
    if (byOrder !== 0) {
      return byOrder;
    }
    // The tree compares whole paths, so the comparison walks to where they part.
    const length = Math.min(left.segments.length, right.segments.length);
    for (let index = 0; index < length; index++) {
      const a = left.segments[index] ?? "";
      const b = right.segments[index] ?? "";
      if (a !== b) {
        const aIsFolder = index < left.segments.length - 1 || left.isDirectory;
        const bIsFolder = index < right.segments.length - 1 || right.isDirectory;
        if (aIsFolder !== bIsFolder) {
          return aIsFolder ? -1 : 1;
        }
        return a.localeCompare(b);
      }
    }
    return left.segments.length - right.segments.length;
  };

const isFolder = (item: FileTreeItemHandle | null): item is FileTreeDirectoryHandle =>
  item?.isDirectory() === true;

/** The folders above a path, outermost first. */
const ancestors = (path: string) => {
  const segments = path.split("/");
  return segments.slice(1).map((_, index) => segments.slice(0, index + 1).join("/"));
};

/**
 * Navigation as a tree with a built-in filter. Rows are added and removed as the paths change, so
 * what is expanded stays expanded.
 */
export function NavTree({
  paths,
  lazy = noFolders,
  onExpand,
  selected,
  onSelect,
  decorations = noDecorations,
  header,
  query = "",
  folderIcons = noIcons,
  order = noOrder,
}: NavTreeProps) {
  // Lazy folders are listed themselves, so one that loads empty stays.
  const rows = [
    ...[...lazy].flatMap(([folder, loaded]) =>
      loaded ? [`${folder}/`] : [`${folder}/`, `${folder}/${placeholder}`],
    ),
    ...paths,
  ];
  // The tree is created once; its callbacks read the latest props through this.
  const latest = useRef({ rows, lazy, onExpand, selected, onSelect, decorations });
  useLayoutEffect(() => {
    latest.current = { rows, lazy, onExpand, selected, onSelect, decorations };
  });
  const { model } = useFileTree({
    paths: rows,
    // Kinds open; what they hold, such as a skill's files, closed.
    initialExpansion: 1,
    initialSelectedPaths: selected === undefined ? [] : [selected],
    flattenEmptyDirectories: false,
    // The caller draws the search field, in its own header.
    search: false,
    fileTreeSearchMode: "hide-non-matches",
    density: "compact",
    unsafeCSS: rootRowsAsHeadings + folderIconRules(folderIcons),
    sort: compareIn(order),
    onSelectionChange: (chosen) => {
      const path = chosen.map(bare).find((p) => !p.endsWith(`/${placeholder}`));
      if (path !== undefined && path !== latest.current.selected) {
        latest.current.onSelect(path);
      }
    },
    renderRowDecoration: ({ item }): FileTreeRowDecoration | null => {
      const text = latest.current.decorations.get(bare(item.path));
      return text === undefined ? null : { text };
    },
  });

  // Rows follow the paths without rebuilding the tree.
  const content = rows.join("\n");
  const shown = useRef<ReadonlyArray<string>>(rows);
  useEffect(() => {
    const next = new Set(content === "" ? [] : content.split("\n"));
    const previous = new Set(shown.current);
    shown.current = [...next];
    const removed = [...previous].filter((path) => !next.has(path));
    const operations: Array<FileTreeBatchOperation> = [
      ...removed
        // Removing a folder removes what it holds.
        .filter((path) => !ancestors(bare(path)).some((folder) => removed.includes(`${folder}/`)))
        .filter((path) => model.getItem(path) !== null)
        .map((path) => ({ type: "remove" as const, path, recursive: true })),
      // Folders first: adding a path makes its folders, and a folder added twice collides.
      ...[...next]
        .filter((path) => !previous.has(path) && model.getItem(path) === null)
        .toSorted((a, b) => Number(b.endsWith("/")) - Number(a.endsWith("/")))
        .map((path) => ({ type: "add" as const, path })),
    ];
    if (operations.length > 0) {
      model.batch(operations);
    }
  }, [model, content]);

  // Notes are drawn with the rows; redraw them when they change.
  const notes = [...decorations].map(([path, note]) => `${path}\t${note}`).join("\n");
  const drawn = useRef(notes);
  useEffect(() => {
    if (drawn.current !== notes) {
      drawn.current = notes;
      model.setComposition(model.getComposition());
    }
  }, [model, notes]);

  // A lazy folder loads its children the first time it is expanded.
  useEffect(() => {
    const requested = new Set<string>();
    const check = () => {
      for (const [folder, loaded] of latest.current.lazy) {
        const item = model.getItem(folder);
        if (!loaded && !requested.has(folder) && isFolder(item) && item.isExpanded()) {
          requested.add(folder);
          latest.current.onExpand?.(folder);
        }
      }
    };
    check();
    return model.subscribe(check);
  }, [model]);

  // Only the page's item is selected, and its folders open once each time it changes, so a
  // folder collapsed by hand stays collapsed.
  const revealed = useRef<string | undefined>(undefined);
  useEffect(() => {
    for (const path of model.getSelectedPaths()) {
      if (bare(path) !== selected) {
        model.getItem(path)?.deselect();
      }
    }
    if (selected === undefined || revealed.current === selected) {
      revealed.current = selected;
      return;
    }
    for (const folder of ancestors(selected)) {
      const handle = model.getItem(folder);
      if (isFolder(handle)) {
        handle.expand();
      }
    }
    // What a lazy folder holds appears later; until then its folders stay open for it.
    const item = model.getItem(selected);
    if (item !== null) {
      if (!item.isSelected()) {
        item.select();
      }
      revealed.current = selected;
    }
  });

  useEffect(() => {
    model.setSearch(query === "" ? null : query);
  }, [model, query]);

  return <FileTree model={model} header={header} className="file-tree h-full" />;
}
