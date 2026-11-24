import { Code, Markdown } from "~/lib/markdown.tsx";
import { fileLanguage } from "~/features/skills/SkillFiles.tsx";

/** SKILL.md split into its YAML front matter and Markdown body. */
export const splitFrontmatter = (
  skillMd: string,
): { readonly frontmatter: string; readonly body: string } => {
  // The block may be empty, and its closing --- ends its line.
  const match = /^---\r?\n(?:([\s\S]*?)\r?\n)?---(?:\r?\n|$)/.exec(skillMd);
  return match === null
    ? { frontmatter: "", body: skillMd }
    : { frontmatter: match[1] ?? "", body: skillMd.slice(match[0].length) };
};

/** SKILL.md as agents read it: front matter, then the rendered body. */
export function SkillDocument({ skillMd }: { readonly skillMd: string }) {
  const { frontmatter, body } = splitFrontmatter(skillMd);
  return (
    <div className="flex flex-col gap-3">
      {frontmatter !== "" && <Code code={frontmatter} language="yaml" />}
      <Markdown>{body}</Markdown>
    </div>
  );
}

/** A file of a skill, highlighted; binary and oversized files are not shown. */
export function FilePreview({
  file,
  content,
  binary,
}: {
  readonly file: string;
  readonly content: string;
  readonly binary: boolean;
}) {
  if (binary) {
    return <p className="text-muted-foreground text-xs">Binary or larger than 1 MiB; not shown.</p>;
  }
  return file.toLowerCase().endsWith(".md") ? (
    <SkillDocument skillMd={content} />
  ) : (
    <Code code={content} language={fileLanguage(file)} />
  );
}
