import { createFileRoute, useNavigate } from "@tanstack/react-router";
import {
  ArrowLeftIcon,
  ArrowRightIcon,
  CheckIcon,
  GitBranchPlusIcon,
  SearchIcon,
} from "lucide-react";
import { useState } from "react";

import { Page } from "~/components/Page.tsx";
import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Pending } from "~/components/Pending.tsx";
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
import { Spinner } from "~/components/ui/spinner.tsx";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "~/components/ui/tabs.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonMutation, useDaemonQuery } from "~/daemon/query.ts";
import { ChooseFile, PlannedFiles, useApply, usePlan } from "~/features/config/plan.tsx";
import { ScopePicker } from "~/features/config/ScopeDialog.tsx";
import {
  addPackage,
  type Scope,
  type StagedChange,
  select,
  useStaging,
} from "~/features/config/staging.ts";
import { useScopeDirectory } from "~/features/scope/scope.ts";
import { FilePreview, SkillDocument } from "~/features/skills/SkillDocument.tsx";
import { SkillFiles } from "~/features/skills/SkillFiles.tsx";
import type { PreviewSkill, PreviewSourceResponse } from "~/gen/agx/v1/agx_pb.ts";
import { shortPath } from "~/lib/path.ts";

export const Route = createFileRoute("/skills/add")({
  component: AddSkillsPage,
});

type Step = 0 | 1 | 2 | 3;

const steps: ReadonlyArray<{ readonly step: Step; readonly label: string }> = [
  { step: 0, label: "Source" },
  { step: 1, label: "Skills" },
  { step: 2, label: "Where" },
  { step: 3, label: "Review" },
];

/** Which step the wizard is on; earlier ones can be revisited, later ones wait for this one. */
function Steps({ step, onStep }: { readonly step: Step; readonly onStep: (step: Step) => void }) {
  return (
    <ol className="flex items-center gap-2">
      {steps.map(({ step: index, label }) => (
        <li key={label} className="flex items-center gap-2">
          {index > 0 && <span aria-hidden className="h-px w-6 bg-border" />}
          <Button
            variant={index === step ? "secondary" : "ghost"}
            size="sm"
            disabled={index > step}
            aria-current={index === step ? "step" : undefined}
            onClick={() => onStep(index)}
          >
            <Badge variant={index < step ? "default" : "outline"}>
              {index < step ? <CheckIcon /> : index + 1}
            </Badge>
            {label}
          </Button>
        </li>
      ))}
    </ol>
  );
}

function AddSkillsPage() {
  const [step, setStep] = useState<Step>(0);
  const [source, setSource] = useState("");
  const preview = useDaemonMutation(
    (next: string) => Daemon.use((daemon) => daemon.previewSource(next)),
    [],
  );
  return (
    <Page
      title={["Add from Git", "Skills"]}
      crumbs={[{ label: "Skills" }, { label: "Add from Git" }]}
      className="max-w-3xl"
    >
      <Steps step={step} onStep={setStep} />
      {step === 0 && (
        <Card size="sm">
          <CardHeader>
            <CardTitle>Where are the skills?</CardTitle>
            <CardDescription>
              The daemon fetches the source and lists its skills; nothing is configured until the
              last step.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              onSubmit={(event) => {
                event.preventDefault();
                preview.mutate(source.trim(), { onSuccess: () => setStep(1) });
              }}
            >
              <InputGroup>
                <InputGroupInput
                  aria-label="Source"
                  className="font-mono"
                  placeholder="owner/repo[@ref][/path] or a Git URL…"
                  spellCheck={false}
                  autoComplete="off"
                  autoFocus
                  value={source}
                  onChange={(event) => setSource(event.target.value)}
                />
                <InputGroupAddon align="inline-end">
                  <InputGroupButton
                    type="submit"
                    disabled={source.trim() === "" || preview.isPending}
                  >
                    {preview.isPending ? <Spinner /> : <SearchIcon />}
                    Preview
                  </InputGroupButton>
                </InputGroupAddon>
              </InputGroup>
            </form>
          </CardContent>
          {preview.isError && (
            <CardFooter>
              <DaemonProblem error={preview.error} />
            </CardFooter>
          )}
        </Card>
      )}
      {step > 0 && preview.isSuccess && (
        <Wizard key={preview.data.previewId} preview={preview.data} step={step} onStep={setStep} />
      )}
    </Page>
  );
}

function Wizard({
  preview,
  step,
  onStep,
}: {
  readonly preview: PreviewSourceResponse;
  readonly step: Step;
  readonly onStep: (step: Step) => void;
}) {
  const source = preview.package;
  const [id, setId] = useState(source?.id ?? "");
  const [chosen, setChosen] = useState<ReadonlyArray<string>>(() =>
    preview.skills.map((skill) => skill.name),
  );
  const [shown, setShown] = useState(preview.skills[0]?.name ?? "");
  const [scope, setScope] = useState<Scope>({ directory: useScopeDirectory(), harnesses: [] });
  const skill = preview.skills.find((s) => s.name === shown);
  const staged: ReadonlyArray<StagedChange> =
    source === undefined
      ? []
      : [
          addPackage({ id, git: source.git, ref: source.ref, subdir: source.subdir }),
          ...chosen.map((name) => select("enable", "skills", `${id}/${name}`, scope)),
        ];
  if (step === 1) {
    return (
      <div className="grid grid-cols-[24rem_minmax(0,1fr)] gap-4">
        <Card size="sm">
          <CardHeader>
            <CardTitle>
              {preview.skills.length} {preview.skills.length === 1 ? "skill" : "skills"} in{" "}
              <span className="font-mono">{source?.git}</span>
            </CardTitle>
            <CardDescription>Checked skills are enabled in the next step.</CardDescription>
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
          <CardFooter className="gap-2">
            <Button variant="outline" onClick={() => onStep(0)}>
              <ArrowLeftIcon data-icon="inline-start" />
              Back
            </Button>
            <Button className="ml-auto" onClick={() => onStep(2)}>
              Next
              <ArrowRightIcon data-icon="inline-end" />
            </Button>
          </CardFooter>
        </Card>
        {skill !== undefined && (
          <PreviewedSkill key={skill.name} previewId={preview.previewId} skill={skill} />
        )}
      </div>
    );
  }
  if (step === 2) {
    return (
      <Card size="sm">
        <CardHeader>
          <CardTitle>Where should they apply?</CardTitle>
          <CardDescription>
            {chosen.length === 0
              ? "No skill is checked, so only the package is added."
              : `Enables ${chosen.length} ${chosen.length === 1 ? "skill" : "skills"} here.`}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="package-id">Package ID</FieldLabel>
              <Input
                id="package-id"
                className="font-mono"
                spellCheck={false}
                autoComplete="off"
                value={id}
                onChange={(event) => setId(event.target.value)}
              />
              <FieldDescription>
                Its skills are named {id === "" ? "<id>" : id}/&lt;skill&gt;.
              </FieldDescription>
            </Field>
            {chosen.length > 0 && <ScopePicker scope={scope} onChange={setScope} />}
          </FieldGroup>
        </CardContent>
        <CardFooter className="gap-2">
          <Button variant="outline" onClick={() => onStep(1)}>
            <ArrowLeftIcon data-icon="inline-start" />
            Back
          </Button>
          <Button className="ml-auto" disabled={id === ""} onClick={() => onStep(3)}>
            Review
            <ArrowRightIcon data-icon="inline-end" />
          </Button>
        </CardFooter>
      </Card>
    );
  }
  return (
    <Review
      changes={staged}
      packageId={id}
      summary={`Package ${id}${
        chosen.length === 0
          ? ""
          : ` · ${chosen.length} ${chosen.length === 1 ? "skill" : "skills"} enabled`
      }`}
      scope={scope}
      onBack={() => onStep(2)}
    />
  );
}

/** The diff the package and its enabled skills make, applied now or staged with other edits. */
function Review({
  changes,
  packageId,
  summary,
  scope,
  onBack,
}: {
  readonly changes: ReadonlyArray<StagedChange>;
  readonly packageId: string;
  readonly summary: string;
  readonly scope: Scope;
  readonly onBack: () => void;
}) {
  const navigate = useNavigate();
  const stage = useStaging((state) => state.stage);
  const target = useStaging((state) => state.target);
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const home = configuration.data?.home ?? "";
  const [file, setFile] = useState("");
  const plan = usePlan(
    changes.map((change) => change.change),
    file,
    true,
  );
  const apply = useApply(
    changes.map((change) => change.change),
    file,
  );
  const done = () => void navigate({ to: "/config", search: { kind: "packages", id: packageId } });
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Review</CardTitle>
        <CardDescription>
          {summary}
          {changes.length > 1 &&
            ` ${scope.directory === "" ? "everywhere" : `in ${shortPath(scope.directory, home)}`}`}
          {scope.harnesses.length > 0 && ` for ${scope.harnesses.join(", ")}`}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {plan.isPending && <Pending className="h-40" />}
        {plan.error?.reason === "choose_file" ? (
          <ChooseFile files={plan.error.files} home={home} onChoose={setFile} />
        ) : (
          plan.isError && <DaemonProblem error={plan.error} />
        )}
        {apply.error !== null && <DaemonProblem error={apply.error} />}
        {plan.isSuccess && <PlannedFiles files={plan.data} />}
      </CardContent>
      <CardFooter className="gap-2">
        <Button variant="outline" onClick={onBack}>
          <ArrowLeftIcon data-icon="inline-start" />
          Back
        </Button>
        <Button
          variant="outline"
          className="ml-auto"
          onClick={() => {
            stage(...changes);
            if (file !== "") {
              target(file);
            }
            toast.add({ type: "success", title: "Added to staged changes" });
            done();
          }}
        >
          <GitBranchPlusIcon data-icon="inline-start" />
          Add to staged changes
        </Button>
        <Button
          disabled={!plan.isSuccess || apply.pending}
          onClick={() => {
            if (plan.isSuccess) {
              apply.run(plan.data, done);
            }
          }}
        >
          {apply.pending ? (
            <Spinner data-icon="inline-start" />
          ) : (
            <CheckIcon data-icon="inline-start" />
          )}
          Apply
        </Button>
      </CardFooter>
    </Card>
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
