import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { GitBranchPlusIcon, SearchIcon } from "lucide-react";
import { useState } from "react";

import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { Button } from "~/components/ui/button.tsx";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "~/components/ui/card.tsx";
import { Checkbox } from "~/components/ui/checkbox.tsx";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "~/components/ui/field.tsx";
import { Input } from "~/components/ui/input.tsx";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "~/components/ui/input-group.tsx";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemTitle,
} from "~/components/ui/item.tsx";
import { ScrollArea } from "~/components/ui/scroll-area.tsx";
import { Pending } from "~/components/Pending.tsx";
import { Spinner } from "~/components/ui/spinner.tsx";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "~/components/ui/tabs.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { useDaemonMutation, useDaemonQuery } from "~/daemon/query.ts";
import { ScopePicker } from "~/features/config/ScopeDialog.tsx";
import { addPackage, type Scope, select, useStaging } from "~/features/config/staging.ts";
import { FilePreview, SkillDocument } from "~/features/skills/SkillDocument.tsx";
import { SkillFiles } from "~/features/skills/SkillFiles.tsx";
import type { PreviewSkill, PreviewSourceResponse } from "~/gen/agx/v1/agx_pb.ts";
import { pageTitle } from "~/lib/title.ts";

export const Route = createFileRoute("/skills/add")({
  component: AddSkillsPage,
});

function AddSkillsPage() {
  const [source, setSource] = useState("");
  const preview = useDaemonMutation(
    (next: string) => Daemon.use((daemon) => daemon.previewSource(next)),
    [],
  );
  return (
    <div className="flex h-full min-h-0 flex-col gap-4 p-4">
      <title>{pageTitle("Add skills")}</title>
      <h1 className="font-heading font-semibold text-sm">Add skills from Git</h1>
      <form
        className="flex flex-col gap-1"
        onSubmit={(event) => {
          event.preventDefault();
          preview.mutate(source.trim());
        }}
      >
        <InputGroup className="max-w-2xl">
          <InputGroupInput
            aria-label="Source"
            className="font-mono"
            placeholder="owner/repo[@ref][/path] or a Git URL…"
            spellCheck={false}
            value={source}
            onChange={(event) => setSource(event.target.value)}
          />
          <InputGroupAddon align="inline-end">
            <InputGroupButton type="submit" disabled={source.trim() === "" || preview.isPending}>
              {preview.isPending ? <Spinner /> : <SearchIcon />}
              Preview
            </InputGroupButton>
          </InputGroupAddon>
        </InputGroup>
        <p className="text-muted-foreground text-xs">
          The daemon fetches the source and lists its skills; nothing is configured until you stage
          and apply.
        </p>
      </form>
      {preview.isPending && <Pending className="h-64" />}
      {preview.isError && <DaemonProblem error={preview.error} />}
      {preview.isSuccess && <Preview key={preview.data.previewId} preview={preview.data} />}
    </div>
  );
}

function Preview({ preview }: { readonly preview: PreviewSourceResponse }) {
  const navigate = useNavigate();
  const stage = useStaging((state) => state.stage);
  const review = useStaging((state) => state.review);
  const source = preview.package;
  const [id, setId] = useState(source?.id ?? "");
  const [chosen, setChosen] = useState<ReadonlyArray<string>>(() =>
    preview.skills.map((skill) => skill.name),
  );
  const [shown, setShown] = useState(preview.skills[0]?.name ?? "");
  const [scope, setScope] = useState<Scope>({ directory: "", harnesses: [] });
  const skill = preview.skills.find((s) => s.name === shown);
  return (
    <div className="grid min-h-0 flex-1 grid-cols-[24rem_minmax(0,1fr)] gap-4">
      <ScrollArea className="min-h-0">
        <div className="flex flex-col gap-4 pr-3">
          <Card size="sm">
            <CardHeader>
              <CardTitle>Package</CardTitle>
              <CardDescription className="font-mono">
                {source?.git}
                {source !== undefined && source.ref !== "" && `@${source.ref}`}
                {source !== undefined && source.subdir !== "" && ` /${source.subdir}`}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="package-id">ID</FieldLabel>
                  <Input
                    id="package-id"
                    className="font-mono"
                    spellCheck={false}
                    value={id}
                    onChange={(event) => setId(event.target.value)}
                  />
                  <FieldDescription>
                    Its skills are named {id === "" ? "<id>" : id}/&lt;skill&gt;.
                  </FieldDescription>
                </Field>
              </FieldGroup>
            </CardContent>
          </Card>
          <Card size="sm">
            <CardHeader>
              <CardTitle>Skills ({preview.skills.length})</CardTitle>
              <CardDescription>Checked skills are enabled where you choose below.</CardDescription>
            </CardHeader>
            <CardContent>
              <ItemGroup>
                {preview.skills.map((s) => (
                  <Item
                    key={s.name}
                    size="sm"
                    variant={s.name === shown ? "muted" : "outline"}
                    onClick={() => setShown(s.name)}
                  >
                    <ItemActions>
                      <Checkbox
                        aria-label={`Enable ${s.name}`}
                        checked={chosen.includes(s.name)}
                        onClick={(event) => event.stopPropagation()}
                        onCheckedChange={(checked) =>
                          setChosen(
                            checked ? [...chosen, s.name] : chosen.filter((n) => n !== s.name),
                          )
                        }
                      />
                    </ItemActions>
                    <ItemContent>
                      <ItemTitle className="font-mono">{s.name}</ItemTitle>
                      <ItemDescription>{s.description}</ItemDescription>
                    </ItemContent>
                  </Item>
                ))}
              </ItemGroup>
            </CardContent>
          </Card>
          <Card size="sm">
            <CardHeader>
              <CardTitle>Enable</CardTitle>
            </CardHeader>
            <CardContent>
              <ScopePicker scope={scope} onChange={setScope} />
            </CardContent>
            <CardFooter>
              <Button
                disabled={id === "" || source === undefined}
                onClick={() => {
                  if (source === undefined) {
                    return;
                  }
                  stage(
                    addPackage({ id, git: source.git, ref: source.ref, subdir: source.subdir }),
                    ...chosen.map((name) => select("enable", "skills", `${id}/${name}`, scope)),
                  );
                  toast.add({ type: "success", title: `Staged package ${id}` });
                  review(true);
                  void navigate({ to: "/skills" });
                }}
              >
                <GitBranchPlusIcon data-icon="inline-start" />
                Stage package
                {chosen.length > 0 &&
                  ` and ${chosen.length} ${chosen.length === 1 ? "skill" : "skills"}`}
              </Button>
            </CardFooter>
          </Card>
        </div>
      </ScrollArea>
      <ScrollArea className="min-h-0">
        <div className="pr-3">
          {skill !== undefined && (
            <PreviewedSkill key={skill.name} previewId={preview.previewId} skill={skill} />
          )}
        </div>
      </ScrollArea>
    </div>
  );
}

function PreviewedSkill({
  previewId,
  skill,
}: {
  readonly previewId: string;
  readonly skill: PreviewSkill;
}) {
  const [file, setFile] = useState<string | null>(null);
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 font-mono">
          {skill.name}
          <Badge variant="secondary">
            {skill.files.length} {skill.files.length === 1 ? "file" : "files"}
          </Badge>
        </CardTitle>
        <CardDescription>{skill.description}</CardDescription>
      </CardHeader>
      <CardContent>
        <Tabs defaultValue="skill">
          <TabsList>
            <TabsTrigger value="skill">SKILL.md</TabsTrigger>
            <TabsTrigger value="files" disabled={skill.files.length === 0}>
              Files
            </TabsTrigger>
          </TabsList>
          <TabsContent value="skill">
            <SkillDocument skillMd={skill.skillMd} />
          </TabsContent>
          <TabsContent value="files">
            <div className="grid grid-cols-[14rem_minmax(0,1fr)] gap-4">
              <SkillFiles files={skill.files} onSelect={setFile} />
              {file === null ? (
                <p className="text-muted-foreground text-xs">Pick a file.</p>
              ) : (
                <PreviewFile key={file} previewId={previewId} skill={skill.name} file={file} />
              )}
            </div>
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}

function PreviewFile({
  previewId,
  skill,
  file,
}: {
  readonly previewId: string;
  readonly skill: string;
  readonly file: string;
}) {
  const content = useDaemonQuery(
    ["preview-file", previewId, skill, file],
    Daemon.use((daemon) => daemon.readPreviewFile(previewId, skill, file)),
  );
  if (content.isPending) {
    return <Pending className="h-40" />;
  }
  if (content.isError) {
    return <DaemonProblem error={content.error} />;
  }
  return <FilePreview file={file} content={content.data.content} binary={content.data.binary} />;
}
