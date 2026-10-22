import { FileTree, useFileTree } from "@pierre/trees/react";

interface SkillFilesProps {
  /** Relative, /-separated paths of the skill's files. */
  readonly files: ReadonlyArray<string>;
  readonly onSelect: (file: string) => void;
}

/** A skill's files as a tree; picking a file reports it, picking a directory does not. */
export function SkillFiles({ files, onSelect }: SkillFilesProps) {
  const { model } = useFileTree({
    paths: files,
    initialExpansion: "open",
    onSelectionChange: (selected) => {
      const file = selected.find((path) => files.includes(path));
      if (file !== undefined) {
        onSelect(file);
      }
    },
  });
  return <FileTree model={model} className="file-tree h-64" />;
}

/** The language Shiki highlights a file as, from its extension. */
export const fileLanguage = (file: string): string => {
  const extension = /\.([^./]+)$/.exec(file)?.[1]?.toLowerCase();
  return extension ?? "text";
};
