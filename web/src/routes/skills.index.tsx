import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import * as Schema from "effect/Schema";
import {
  BlocksIcon,
  FileCode2Icon,
  GitBranchPlusIcon,
  ListChecksIcon,
  PackageIcon,
  PencilIcon,
  RotateCcwIcon,
} from "lucide-react";
import { useState } from "react";

import { ButtonLink } from "~/components/ButtonLink.tsx";
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
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "~/components/ui/card.tsx";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "~/components/ui/empty.tsx";
import { Input } from "~/components/ui/input.tsx";
import { Pending } from "~/components/Pending.tsx";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "~/components/ui/tabs.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { CodeEditor } from "~/features/config/CodeEditor.tsx";
import { ScopeDialog } from "~/features/config/ScopeDialog.tsx";
import { useStagedFile, useStaging, writeFile } from "~/features/config/staging.ts";
import { FilePreview } from "~/features/skills/SkillDocument.tsx";
import { SkillFiles } from "~/features/skills/SkillFiles.tsx";
import type { Definition } from "~/gen/agx/v1/agx_pb.ts";
import { shortPath } from "~/lib/path.ts";
import { pageTitle } from "~/lib/title.ts";

const SkillsSearch = Schema.Struct({
  skill: Schema.optional(Schema.String),
  tab: Schema.optional(Schema.Literals(["skill", "files"])),
  /** The open file, relative to the skill's directory. */
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
    <div className="flex h-full min-h-0 flex-col gap-4 p-4">
      <title>{pageTitle(search.skill, "Skills")}</title>
      <div className="flex items-center gap-2">
        <h1 className="font-heading font-semibold text-sm">Skills</h1>
        <ButtonLink size="sm" className="ml-auto" to="/skills/add">
          <GitBranchPlusIcon data-icon="inline-start" />
          Add from Git
        </ButtonLink>
      </div>
      {definitions.isPending && <Pending className="h-64" />}
      {definitions.isError && <DaemonProblem error={definitions.error} />}
      {definitions.isSuccess &&
        (search.skill === undefined ? (
          <SkillOverview skills={skills} />
        ) : (
          <div className="max-w-5xl">
            <SkillView
              key={search.skill}
              skillId={search.skill}
              home={configuration.data?.home ?? ""}
            />
          </div>
        ))}
    </div>
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
  const navigate = useNavigate({ from: Route.fullPath });
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
  const { definition, skillMd, files } = skill.data;
  // A skill written in configuration has no directory; its definition holds SKILL.md.
  const directory = definition?.path ?? "";
  const editable = definition !== undefined && !definition.readOnly && directory !== "";
  return (
    <div className="flex flex-col gap-4">
      <Card size="sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 font-mono">
            {skillId}
            {definition?.package !== undefined && definition.package !== "" && (
              <Badge variant="secondary">
                <PackageIcon data-icon="inline-start" />
                {definition.package}
              </Badge>
            )}
            {definition?.readOnly === true && <Badge variant="outline">read-only</Badge>}
          </CardTitle>
          <CardDescription>{definition?.description}</CardDescription>
          <CardAction className="flex gap-2">
            <ButtonLink
              variant="outline"
              size="sm"
              to="/config"
              search={{ kind: "skills", id: skillId }}
            >
              <FileCode2Icon data-icon="inline-start" />
              Definition
            </ButtonLink>
            <ScopeDialog
              kind="skills"
              id={skillId}
              trigger={
                <Button variant="outline" size="sm">
                  <ListChecksIcon data-icon="inline-start" />
                  Enable or disable…
                </Button>
              }
            />
          </CardAction>
        </CardHeader>
        {/* A package's skill lives in the cache; only a local skill's directory says something. */}
        {directory !== "" && definition?.package === "" && (
          <CardContent className="break-all font-mono text-muted-foreground text-xs">
            {shortPath(directory, home)}
          </CardContent>
        )}
      </Card>
      <Tabs
        value={search.tab ?? "skill"}
        onValueChange={(next) =>
          void navigate({
            search: (current) => ({ ...current, tab: next === "files" ? "files" : "skill" }),
            replace: true,
          })
        }
      >
        <TabsList>
          <TabsTrigger value="skill">SKILL.md</TabsTrigger>
          <TabsTrigger value="files" disabled={files.length === 0}>
            Files ({files.length})
          </TabsTrigger>
        </TabsList>
        <TabsContent value="skill">
          <EditableFile
            path={`${directory}/SKILL.md`}
            original={skillMd}
            editable={editable}
            home={home}
          />
        </TabsContent>
        <TabsContent value="files">
          <SkillFileBrowser directory={directory} files={files} editable={editable} home={home} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function SkillFileBrowser({
  directory,
  files,
  editable,
  home,
}: {
  readonly directory: string;
  readonly files: ReadonlyArray<string>;
  readonly editable: boolean;
  readonly home: string;
}) {
  const file = Route.useSearch({ select: (search) => search.file });
  const navigate = useNavigate({ from: Route.fullPath });
  return (
    <div className="grid grid-cols-[16rem_minmax(0,1fr)] gap-4">
      <Card size="sm">
        <CardContent>
          <SkillFiles
            files={files}
            selected={file}
            onSelect={(next) =>
              void navigate({ search: (current) => ({ ...current, file: next }) })
            }
          />
        </CardContent>
      </Card>
      {file === undefined ? (
        <p className="text-muted-foreground text-xs">Pick a file.</p>
      ) : (
        <SkillFile key={file} path={`${directory}/${file}`} editable={editable} home={home} />
      )}
    </div>
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
        {editable && !editing && (
          <CardAction>
            <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
              <PencilIcon data-icon="inline-start" />
              Edit
            </Button>
          </CardAction>
        )}
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
      {editing && (
        <CardFooter className="gap-2">
          <Button
            disabled={text === original && staged === undefined}
            onClick={() => {
              stage(writeFile(path, text, home));
              setEditing(false);
              toast.add({ type: "success", title: `Staged ${name}` });
            }}
          >
            Stage
          </Button>
          <Button
            variant="outline"
            onClick={() => {
              unstage(`file/${path}`);
              setText(original);
              setEditing(false);
            }}
          >
            <RotateCcwIcon data-icon="inline-start" />
            Revert
          </Button>
        </CardFooter>
      )}
    </Card>
  );
}
