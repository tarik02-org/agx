import { PackageIcon, PencilIcon, RotateCcwIcon } from "lucide-react";
import { useState } from "react";

import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { PageActions } from "~/components/Page.tsx";
import { Pending } from "~/components/Pending.tsx";
import { UnsavedChanges } from "~/components/UnsavedChanges.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { Button } from "~/components/ui/button.tsx";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { CodeEditor } from "~/features/config/CodeEditor.tsx";
import { useStagedFile, useStaging, writeFile } from "~/features/config/staging.ts";
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
        <EditableFile
          path={`${directory}/SKILL.md`}
          original={skillMd}
          editable={editable}
          home={home}
        />
      ) : (
        <SkillFile key={file} path={`${directory}/${file}`} editable={editable} home={home} />
      )}
    </>
  );
}

function SkillFile({
  path,
  editable,
  home,
}: {
  readonly path: string;
  readonly editable: boolean;
  readonly home: string;
}) {
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
      home={home}
    />
  );
}

/** A skill file shown rendered, or edited and staged when the skill is a local directory. */
function EditableFile({
  path,
  original,
  editable,
  home,
}: {
  readonly path: string;
  readonly original: string;
  readonly editable: boolean;
  readonly home: string;
}) {
  const staged = useStagedFile(path);
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(staged ?? original);
  const stage = useStaging((state) => state.stage);
  const unstage = useStaging((state) => state.unstage);
  const name = path.slice(path.lastIndexOf("/") + 1);
  return (
    <Card size="sm">
      <UnsavedChanges dirty={editing && text !== (staged ?? original)} />
      <CardHeader>
        <CardTitle className="flex items-center gap-2 font-mono">
          {name}
          {staged !== undefined && <Badge>staged</Badge>}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {editing ? (
          <CodeEditor
            language={name.toLowerCase().endsWith(".md") ? "markdown" : "text"}
            value={text}
            onChange={setText}
          />
        ) : (
          <FilePreview file={name} content={text} binary={false} />
        )}
      </CardContent>
      {editable && (
        <PageActions>
          {editing ? (
            <>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  unstage(`file/${path}`);
                  setText(original);
                  setEditing(false);
                }}
              >
                <RotateCcwIcon data-icon="inline-start" />
                Revert
              </Button>
              <Button
                size="sm"
                disabled={text === original && staged === undefined}
                onClick={() => {
                  stage(writeFile(path, text, home));
                  setEditing(false);
                  toast.add({ type: "success", title: `Staged ${name}` });
                }}
              >
                Stage
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
