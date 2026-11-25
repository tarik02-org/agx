import { PackageIcon, PencilIcon, RotateCcwIcon } from "lucide-react";
import { useState } from "react";

import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { PageActions } from "~/components/Page.tsx";
import { Pending } from "~/components/Pending.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { Button } from "~/components/ui/button.tsx";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card.tsx";
import { Spinner } from "~/components/ui/spinner.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { UnsavedChanges } from "~/components/UnsavedChanges.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonMutation, useDaemonQuery } from "~/daemon/query.ts";
import { CodeEditor } from "~/features/config/CodeEditor.tsx";
import { FilePreview } from "~/features/skills/SkillDocument.tsx";
import { shortPath } from "~/lib/path.ts";

/**
 * A skill at a glance: what it does, where it lives, and SKILL.md or another of its files, shown
 * rendered and edited in place when the skill is a local directory.
 */
export function SkillOverview({
  skillId,
  file,
  home,
}: {
  readonly skillId: string;
  /** A file relative to the skill's directory; SKILL.md when absent. */
  readonly file: string | undefined;
  readonly home: string;
}) {
  const skill = useDaemonQuery(
    queryKeys.skill(skillId),
    Daemon.use((daemon) => daemon.skill(skillId)),
  );
  if (skill.isPending) {
    return <Pending className="h-64" />;
  }
  if (skill.isError) {
    return <DaemonProblem error={skill.error} />;
  }
  const { definition, skillMd } = skill.data;
  // A skill written in configuration has no directory; its definition holds SKILL.md.
  const directory = definition?.path ?? "";
  const editable = definition !== undefined && !definition.readOnly && directory !== "";
  return (
    <>
      <Card size="sm">
        <CardHeader>
          {/* The page header names the skill and holds its actions. */}
          <CardTitle className="flex items-center gap-2">
            {definition?.description}
            {definition?.package !== undefined && definition.package !== "" && (
              <Badge variant="secondary">
                <PackageIcon data-icon="inline-start" />
                {definition.package}
              </Badge>
            )}
            {definition?.readOnly === true && <Badge variant="outline">read-only</Badge>}
          </CardTitle>
        </CardHeader>
        {/* A package's skill lives in the cache; only a local skill's directory says something. */}
        {directory !== "" && definition?.package === "" && (
          <CardContent className="break-all font-mono text-muted-foreground text-xs">
            {shortPath(directory, home)}
          </CardContent>
        )}
      </Card>
      {file === undefined || file === "SKILL.md" ? (
        <EditableFile path={`${directory}/SKILL.md`} original={skillMd} editable={editable} />
      ) : (
        <SkillFile key={file} path={`${directory}/${file}`} editable={editable} />
      )}
    </>
  );
}

function SkillFile({ path, editable }: { readonly path: string; readonly editable: boolean }) {
  const content = useDaemonQuery(
    queryKeys.file(path),
    Daemon.use((daemon) => daemon.readFile(path)),
  );
  if (content.isPending) {
    return <Pending className="h-40" />;
  }
  if (content.isError) {
    return <DaemonProblem error={content.error} />;
  }
  if (content.data.binary) {
    return <FilePreview file={path} content="" binary />;
  }
  return (
    <EditableFile
      path={path}
      original={content.data.content}
      editable={editable && !content.data.readOnly}
    />
  );
}

/**
 * A skill file shown rendered, or edited and saved when the skill is a local directory. Files
 * are written at once: they are not configuration, so staged changes do not hold them.
 */
function EditableFile({
  path,
  original,
  editable,
}: {
  readonly path: string;
  readonly original: string;
  readonly editable: boolean;
}) {
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(original);
  const save = useDaemonMutation(
    (content: string) => Daemon.use((daemon) => daemon.writeResourceFile(path, content)),
    // SKILL.md comes with its skill.
    [queryKeys.file(path), ["skill"]],
  );
  const name = path.slice(path.lastIndexOf("/") + 1);
  return (
    <Card size="sm">
      <UnsavedChanges dirty={editing && text !== original} />
      <CardHeader>
        <CardTitle className="font-mono">{name}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {editing ? (
          <CodeEditor
            language={name.toLowerCase().endsWith(".md") ? "markdown" : "text"}
            value={text}
            onChange={setText}
          />
        ) : (
          <FilePreview file={name} content={text} binary={false} />
        )}
        {save.error !== null && <DaemonProblem error={save.error} />}
      </CardContent>
      {editable && (
        <PageActions>
          {editing ? (
            <>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setText(original);
                  setEditing(false);
                }}
              >
                <RotateCcwIcon data-icon="inline-start" />
                Revert
              </Button>
              <Button
                size="sm"
                disabled={text === original || save.isPending}
                onClick={() =>
                  save.mutate(text, {
                    onSuccess: () => {
                      setEditing(false);
                      toast.add({ type: "success", title: `Saved ${name}` });
                    },
                  })
                }
              >
                {save.isPending && <Spinner data-icon="inline-start" />}
                Save
              </Button>
            </>
          ) : (
            <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
              <PencilIcon data-icon="inline-start" />
              Edit
            </Button>
          )}
        </PageActions>
      )}
    </Card>
  );
}
