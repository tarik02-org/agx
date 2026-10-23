import { create } from "zustand";

import type { ConfigChangeInit } from "~/daemon/Daemon.ts";
import { shortPath } from "~/lib/path.ts";

/** A change waiting for the next apply, which writes all staged changes at once. */
export interface StagedChange {
  /** Staging a change with the key of one already staged replaces it. */
  readonly key: string;
  readonly label: string;
  readonly change: ConfigChangeInit;
}

interface Staging {
  readonly changes: ReadonlyArray<StagedChange>;
  readonly reviewing: boolean;
  readonly stage: (...changes: ReadonlyArray<StagedChange>) => void;
  readonly unstage: (key: string) => void;
  readonly clear: () => void;
  readonly review: (open: boolean) => void;
}

// A replaced change keeps its place: adding a package stays ahead of enabling its skills.
const upsert = (
  changes: ReadonlyArray<StagedChange>,
  next: StagedChange,
): ReadonlyArray<StagedChange> =>
  changes.some((change) => change.key === next.key)
    ? changes.map((change) => (change.key === next.key ? next : change))
    : [...changes, next];

/** Staged configuration changes; they live in this tab until applied or discarded. */
export const useStaging = create<Staging>()((set) => ({
  changes: [],
  reviewing: false,
  stage: (...next) => set(({ changes }) => ({ changes: next.reduce(upsert, changes) })),
  unstage: (key) => set(({ changes }) => ({ changes: changes.filter((c) => c.key !== key) })),
  clear: () => set({ changes: [], reviewing: false }),
  review: (open) => set({ reviewing: open }),
}));

const definitionKey = (kind: string, id: string) => `definition/${kind}/${id}`;

/** The staged change of a definition, if any: its new YAML, or its removal. */
export const useStagedDefinition = (kind: string, id: string): StagedChange | undefined =>
  useStaging((state) => state.changes.find((change) => change.key === definitionKey(kind, id)));

/** The staged content of a file, if any. */
export const useStagedFile = (path: string): string | undefined =>
  useStaging((state) => {
    const staged = state.changes.find((change) => change.key === `file/${path}`)?.change.change;
    return staged?.case === "writeFile" ? staged.value.content : undefined;
  });

export const define = (kind: string, id: string, yaml: string, file: string): StagedChange => ({
  key: definitionKey(kind, id),
  label: `Define ${kind} ${id}`,
  change: { change: { case: "define", value: { kind, id, yaml, file } } },
});

export const undefine = (kind: string, id: string): StagedChange => ({
  key: definitionKey(kind, id),
  label: `Remove ${kind} ${id}`,
  change: { change: { case: "undefine", value: { kind, id } } },
});

/** Where a scope rule applies: a directory, or everywhere when empty; for some harnesses, or all. */
export interface Scope {
  readonly directory: string;
  readonly harnesses: ReadonlyArray<string>;
}

export const scopeLabel = ({ directory, harnesses }: Scope): string =>
  `${directory === "" ? "everywhere" : directory}${harnesses.length === 0 ? "" : ` for ${harnesses.join(", ")}`}`;

export const select = (
  action: "enable" | "disable",
  kind: string,
  id: string,
  scope: Scope,
): StagedChange => ({
  // Enabling and then disabling the same thing in the same scope leaves only the last.
  key: `selection/${kind}/${id}/${scope.directory}/${scope.harnesses.join(",")}`,
  label: `${action === "enable" ? "Enable" : "Disable"} ${kind} ${id} ${scopeLabel(scope)}`,
  change: {
    change: {
      case: action,
      value: { kind, id, directory: scope.directory, harnesses: [...scope.harnesses] },
    },
  },
});

export const writeFile = (path: string, content: string, home: string): StagedChange => ({
  key: `file/${path}`,
  label: `Write ${shortPath(path, home)}`,
  change: { change: { case: "writeFile", value: { path, content } } },
});

export interface PackageSource {
  readonly id: string;
  readonly git: string;
  readonly ref: string;
  readonly subdir: string;
}

export const addPackage = (source: PackageSource): StagedChange => ({
  key: `package/${source.id}`,
  label: `Add package ${source.id} from ${source.git}${source.ref === "" ? "" : `@${source.ref}`}`,
  change: { change: { case: "addPackage", value: { ...source } } },
});
