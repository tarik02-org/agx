import { createFileRoute, Link } from "@tanstack/react-router";
import * as Schema from "effect/Schema";
import {
  BlocksIcon,
  FileCode2Icon,
  GitBranchPlusIcon,
  ListChecksIcon,
  PackageIcon,
  PencilIcon,
  PlusIcon,
  RotateCcwIcon,
} from "lucide-react";
import { useState } from "react";

import { ButtonLink } from "~/components/ButtonLink.tsx";
import { Page, PageActions } from "~/components/Page.tsx";
import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemTitle,
} from "~/components/ui/item.tsx";
import { useEnabledHere } from "~/features/scope/scope.ts";
import { UnsavedChanges } from "~/components/UnsavedChanges.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { Button } from "~/components/ui/button.tsx";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card.tsx";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "~/components/ui/empty.tsx";
import { Input } from "~/components/ui/input.tsx";
import { Pending } from "~/components/Pending.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { CodeEditor } from "~/features/config/CodeEditor.tsx";
import { ScopeDialog } from "~/features/config/ScopeDialog.tsx";
import { useStagedFile, useStaging, writeFile } from "~/features/config/staging.ts";
import { FilePreview } from "~/features/skills/SkillDocument.tsx";
import type { Definition } from "~/gen/agx/v1/agx_pb.ts";
import { shortPath } from "~/lib/path.ts";

const SkillsSearch = Schema.Struct({
  skill: Schema.optional(Schema.String),
  /** The open file, relative to the skill's directory; SKILL.md when absent. */
  file: Schema.optional(Schema.String),
});

export const Route = createFileRoute("/skills/")({
  validateSearch: Schema.decodeUnknownSync(SkillsSearch),
  component: SkillsPage,
});

function SkillsPage() {
  const definitions = useDaemonQuery(
    queryKeys.definitions,
    Daemon.use((daemon) => daemon.definitions),
  );
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const search = Route.useSearch();
  const skills = (definitions.data ?? []).filter((definition) => definition.kind === "skills");
  return (
    <Page
      title={[search.file, search.skill, "Skills"]}
      crumbs={[
        { label: "Skills", link: <Link to="/skills" /> },
        ...(search.skill === undefined
          ? []
          : [
              {
                label: search.skill,
                mono: true,
                link: <Link to="/skills" search={{ skill: search.skill }} />,
              },
              { label: search.file ?? "SKILL.md", mono: true },
            ]),
      ]}
      actions={
        <>
          {search.skill !== undefined && (
            <>
              <ButtonLink
                variant="outline"
                size="sm"
                to="/config"
                search={{ kind: "skills", id: search.skill }}
              >
                <FileCode2Icon data-icon="inline-start" />
                Definition
              </ButtonLink>
              <ScopeDialog
                kind="skills"
                id={search.skill}
                trigger={
                  <Button variant="outline" size="sm">
                    <ListChecksIcon data-icon="inline-start" />
                    Enable or disable…
                  </Button>
                }
              />
            </>
          )}
          <ButtonLink
            variant="outline"
            size="sm"
            to="/config"
            search={{ kind: "skills", create: true }}
          >
            <PlusIcon data-icon="inline-start" />
            New skill
          </ButtonLink>
          <ButtonLink size="sm" to="/skills/add">
            <GitBranchPlusIcon data-icon="inline-start" />
            Add from Git
          </ButtonLink>
        </>
      }
    >
      {definitions.isPending && <Pending className="h-64" />}
      {definitions.isError && <DaemonProblem error={definitions.error} />}
      {definitions.isSuccess &&
        (search.skill === undefined ? (
          <SkillOverview skills={skills} />
        ) : (
          <SkillView
            key={search.skill}
            skillId={search.skill}
            home={configuration.data?.home ?? ""}
          />
        ))}
    </Page>
  );
}

/** Every skill with what it does, marking those the scope directory gets. */
function SkillOverview({ skills }: { readonly skills: ReadonlyArray<Definition> }) {
  const enabledHere = useEnabledHere();
  const [filter, setFilter] = useState("");
  const shown = skills.filter(
    (skill) =>
      filter === "" ||
      `${skill.id} ${skill.description}`.toLowerCase().includes(filter.toLowerCase()),
  );
  if (skills.length === 0) {
    return (
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <BlocksIcon />
          </EmptyMedia>
          <EmptyTitle>No skills yet</EmptyTitle>
          <EmptyDescription>Add some from a Git repository.</EmptyDescription>
        </EmptyHeader>
      </Empty>
    );
  }
  return (
    <div className="flex max-w-5xl flex-col gap-3">
      <Input
        type="search"
        aria-label="Filter skills"
        placeholder="Filter skills…"
        autoComplete="off"
        spellCheck={false}
        className="max-w-sm"
        value={filter}
        onChange={(event) => setFilter(event.target.value)}
      />
      <ItemGroup className="grid grid-cols-[repeat(auto-fill,minmax(20rem,1fr))] gap-2">
        {shown.map((skill) => (
          <Item
            key={skill.id}
            variant="outline"
            size="sm"
            render={<Link to="/skills" search={{ skill: skill.id }} />}
          >
            <ItemContent>
              <ItemTitle className="font-mono">
                {skill.id}
                {skill.package !== "" && <PackageIcon className="size-3" />}
              </ItemTitle>
              <ItemDescription>{skill.description}</ItemDescription>
            </ItemContent>
            {enabledHere?.has(`skills/${skill.id}`) === true && (
              <ItemActions>
                <Badge variant="secondary">here</Badge>
              </ItemActions>
            )}
          </Item>
        ))}
      </ItemGroup>
    </div>
  );
}

function SkillView({ skillId, home }: { readonly skillId: string; readonly home: string }) {
  const search = Route.useSearch();
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
          {/* The top bar names the skill and holds its actions. */}
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
      {search.file === undefined || search.file === "SKILL.md" ? (
        <EditableFile
          path={`${directory}/SKILL.md`}
          original={skillMd}
          editable={editable}
          home={home}
        />
      ) : (
        <SkillFile
          key={search.file}
          path={`${directory}/${search.file}`}
          editable={editable}
          home={home}
        />
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
