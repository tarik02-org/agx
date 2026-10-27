import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { cn } from "cn";
import * as Schema from "effect/Schema";
import {
  FileCode2Icon,
  LinkIcon,
  ListChecksIcon,
  LockIcon,
  PackageIcon,
  RotateCcwIcon,
  Trash2Icon,
} from "lucide-react";
import { useState } from "react";

import { ConfirmAction } from "~/components/ConfirmAction.tsx";
import { UnsavedChanges } from "~/components/UnsavedChanges.tsx";
import { type Crumb, Page, PageActions, PageDetails, PageViews } from "~/components/Page.tsx";
import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Alert, AlertDescription, AlertTitle } from "~/components/ui/alert.tsx";
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
import { Field, FieldGroup, FieldLabel } from "~/components/ui/field.tsx";
import { Input } from "~/components/ui/input.tsx";
import { Pending } from "~/components/Pending.tsx";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "~/components/ui/tabs.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { CodeEditor } from "~/features/config/CodeEditor.tsx";
import { DefinitionForm, hasForm } from "~/features/config/DefinitionForm.tsx";
import {
  BundleOverview,
  InstructionsOverview,
  PackageOverview,
  SecretOverview,
} from "~/features/config/DefinitionOverview.tsx";
import { ScopeDialog } from "~/features/config/ScopeDialog.tsx";
import { SettingsTree } from "~/features/explain/SettingsTree.tsx";
import { resourceKey, Traces } from "~/features/explain/Traces.tsx";
import { ServerOverview } from "~/features/mcp/ServerOverview.tsx";
import { useResolution, useScopeDirectory, useScopeHarness } from "~/features/scope/scope.ts";
import { SkillOverview } from "~/features/skills/SkillOverview.tsx";
import {
  define,
  undefine,
  useStagedDefinition,
  useStagedFile,
  useStaging,
  writeFile,
} from "~/features/config/staging.ts";
import { yamlErrors } from "~/features/config/yamlDocument.ts";
import type { ConfigurationFile, Definition } from "~/gen/agx/v1/agx_pb.ts";
import { type ConfigSchema, definitionSchema, fieldDescription } from "~/lib/configSchema.ts";
import { kinds, selectable } from "~/features/config/kinds.ts";
import { shortPath } from "~/lib/path.ts";

const ConfigSearch = Schema.Struct({
  kind: Schema.optional(Schema.String),
  id: Schema.optional(Schema.String),
  /** A new definition of kind is being written. */
  create: Schema.optional(Schema.Boolean),
  file: Schema.optional(Schema.String),
  /** How the definition is edited, so a link opens the same editor. */
  view: Schema.optional(Schema.Literals(["form", "yaml"])),
  /** The item's overview, or its definition to edit. */
  tab: Schema.optional(Schema.Literals(["overview", "definition"])),
  /** A skill's file, relative to its directory; SKILL.md when absent. */
  path: Schema.optional(Schema.String),
  /** An MCP server's tool to call. */
  tool: Schema.optional(Schema.String),
});

export const Route = createFileRoute("/config")({
  validateSearch: Schema.decodeUnknownSync(ConfigSearch),
  component: ConfigPage,
});

/**
 * An item of the configuration: a definition, with an overview of what it is and does here and
 * its definition to edit, or a configuration file.
 */
function ConfigPage() {
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const definitions = useDaemonQuery(
    queryKeys.definitions,
    Daemon.use((daemon) => daemon.definitions),
  );
  const schema = useDaemonQuery(
    queryKeys.configSchema,
    Daemon.use((daemon) => daemon.configSchema),
    { staleTime: Number.POSITIVE_INFINITY },
  );
  const search = Route.useSearch();
  const navigate = useNavigate({ from: Route.fullPath });
  const home = configuration.data?.home ?? "";
  const label = kinds.find((k) => k.kind === search.kind)?.label ?? search.kind;
  const definition = definitions.data?.find(
    (candidate) => candidate.kind === search.kind && candidate.id === search.id,
  );
  const creating = search.create === true;
  const tab = creating ? "definition" : (search.tab ?? "overview");
  const crumbs: Array<Crumb> =
    search.file !== undefined
      ? [{ label: "Files" }, { label: shortPath(search.file, home), mono: true }]
      : label === undefined
        ? [{ label: "Configuration" }]
        : [
            { label },
            {
              label: search.id ?? "new",
              mono: true,
              link: <Link to="/config" search={{ kind: search.kind, id: search.id }} />,
            },
            ...(search.tool === undefined ? [] : [{ label: search.tool, mono: true }]),
            ...(search.path === undefined ? [] : [{ label: search.path, mono: true }]),
          ];
  return (
    <Page
      title={[search.tool ?? search.id ?? search.file?.split("/").at(-1), label]}
      crumbs={crumbs}
      {...(search.file === undefined && tab === "overview" ? { className: "max-w-3xl" } : {})}
      actions={
        definition !== undefined &&
        selectable.has(definition.kind) && (
          <ScopeDialog
            kind={definition.kind}
            id={definition.id}
            trigger={
              <Button variant="outline" size="sm">
                <ListChecksIcon data-icon="inline-start" />
                Enable or disable…
              </Button>
            }
          />
        )
      }
      fill={
        search.file !== undefined ||
        (tab === "definition" &&
          search.kind !== undefined &&
          (search.view === "yaml" || !hasForm(search.kind)))
      }
    >
      {(configuration.isPending || definitions.isPending) && <Pending className="h-64" />}
      {configuration.isError && <DaemonProblem error={configuration.error} />}
      {definitions.isError && <DaemonProblem error={definitions.error} />}
      {configuration.isSuccess &&
        definitions.isSuccess &&
        (search.file !== undefined ? (
          <FileEditor
            key={search.file}
            path={search.file}
            file={configuration.data.files.find((file) => file.path === search.file)}
            home={configuration.data.home}
            schema={schema.data}
          />
        ) : search.kind !== undefined && (search.id !== undefined || creating) ? (
          <Tabs
            value={tab}
            onValueChange={(next) =>
              void navigate({
                search: (current) => ({
                  ...current,
                  tab: next === "definition" ? "definition" : undefined,
                }),
                replace: true,
              })
            }
            className="flex min-h-0 flex-1 flex-col gap-3"
          >
            {!creating && (
              <PageViews>
                <TabsList>
                  <TabsTrigger value="overview">Overview</TabsTrigger>
                  <TabsTrigger value="definition">Definition</TabsTrigger>
                </TabsList>
              </PageViews>
            )}
            <TabsContent value="overview" className="flex flex-col gap-2">
              {search.id !== undefined && (
                <ItemOverview
                  kind={search.kind}
                  id={search.id}
                  definition={definition}
                  path={search.path}
                  tool={search.tool}
                  home={home}
                  definitions={definitions.data}
                />
              )}
            </TabsContent>
            <TabsContent value="definition" className="flex min-h-0 flex-1 flex-col">
              <DefinitionEditor
                key={`${search.kind}/${search.id ?? ""}/${String(creating)}`}
                kind={search.kind}
                definition={definition}
                initialId={search.id ?? ""}
                home={configuration.data.home}
                schema={schema.data}
              />
            </TabsContent>
          </Tabs>
        ) : (
          <Empty>
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <FileCode2Icon />
              </EmptyMedia>
              <EmptyTitle>Pick something in the sidebar</EmptyTitle>
              <EmptyDescription>
                Edits are staged; review and apply them together from Staged changes.
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ))}
    </Page>
  );
}

/** What an item is and does, and why a launch in the scope directory gets it or not. */
function ItemOverview({
  kind,
  id,
  definition,
  path,
  tool,
  home,
  definitions,
}: {
  readonly kind: string;
  readonly id: string;
  readonly definition: Definition | undefined;
  readonly path: string | undefined;
  readonly tool: string | undefined;
  readonly home: string;
  readonly definitions: ReadonlyArray<Definition>;
}) {
  if (kind === "mcp" && tool !== undefined) {
    return <ServerOverview key={tool} mcpId={id} toolName={tool} />;
  }
  return (
    <>
      <WhyHere kind={kind} id={id} home={home} />
      {kind === "skills" ? (
        <SkillOverview key={`${id}/${path ?? ""}`} skillId={id} file={path} home={home} />
      ) : kind === "mcp" ? (
        <ServerOverview key={id} mcpId={id} toolName={undefined} />
      ) : (
        <>
          {definition?.description !== undefined && definition.description !== "" && (
            <p className="text-muted-foreground text-xs">{definition.description}</p>
          )}
          {kind === "harnesses" && <HarnessSettings harness={id} home={home} />}
          {kind === "packages" && (
            <PackageOverview definition={definition} definitions={definitions} />
          )}
          {kind === "bundles" && (
            <BundleOverview definition={definition} definitions={definitions} />
          )}
          {kind === "secrets" && <SecretOverview id={id} definitions={definitions} />}
          {kind === "instructions" && <InstructionsOverview definition={definition} home={home} />}
        </>
      )}
    </>
  );
}

// The resolution's names for the kinds a scope selects.
const resolvedKinds: Readonly<Record<string, string>> = {
  skills: "skill",
  instructions: "instructions",
  mcp: "mcp",
};

/** Whether a launch in the scope directory gets the item, and the rules that decided it. */
function WhyHere({
  kind,
  id,
  home,
}: {
  readonly kind: string;
  readonly id: string;
  readonly home: string;
}) {
  const directory = useScopeDirectory();
  const harness = useScopeHarness();
  const resolved = useResolution();
  const resolvedKind = resolvedKinds[kind];
  if (resolvedKind === undefined || directory === "") {
    return null;
  }
  const trace = resolved.data?.traces.find((candidate) => {
    const key = resourceKey(candidate.resource);
    return key.kind === resolvedKind && key.id === id;
  });
  const selected = new Set(
    (resolved.data?.snapshot?.resources ?? []).map((ref) => {
      const key = resourceKey(ref);
      return `${key.kind}/${key.id}`;
    }),
  );
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>
          In <span className="font-mono">{shortPath(directory, home)}</span>
          {harness !== "" && (
            <>
              {" "}
              with <span className="font-mono">{harness}</span>
            </>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {resolved.isPending ? (
          <Pending className="h-10" />
        ) : resolved.isError ? (
          <DaemonProblem error={resolved.error} />
        ) : trace === undefined ? (
          <p className="text-muted-foreground text-xs">
            No rule names it here, so launches go without it.
          </p>
        ) : (
          <Traces traces={[trace]} selected={selected} home={home} />
        )}
      </CardContent>
    </Card>
  );
}

/** A harness's settings as a launch gets them, in the scope directory or else at home. */
function HarnessSettings({ harness, home }: { readonly harness: string; readonly home: string }) {
  const scope = useScopeDirectory();
  const directory = scope === "" ? home : scope;
  const resolved = useDaemonQuery(
    queryKeys.resolve(directory, harness),
    Daemon.use((daemon) => daemon.resolve(directory, harness)),
    { enabled: directory !== "" },
  );
  if (resolved.isPending) {
    return <Pending className="h-16" />;
  }
  if (resolved.isError) {
    return <DaemonProblem error={resolved.error} />;
  }
  return resolved.data.settings.length === 0 ? null : (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Settings</CardTitle>
      </CardHeader>
      <CardContent>
        <SettingsTree settings={resolved.data.settings} home={home} />
      </CardContent>
    </Card>
  );
}

/**
 * Where a definition is written, for showing: under its package for one a package defines, whose
 * files live in the cache under the package's hash, and as ~/… otherwise.
 */
const definedAt = (path: string, home: string): string =>
  /\/packages\/[0-9a-f]{64}\/(.+)$/.exec(path)?.[1] ?? shortPath(path, home);

// What a new definition starts as: skills and instructions written inline, the usual way to
// start one; the form's source switch takes them elsewhere.
const newDefinitions: Readonly<Record<string, string>> = {
  skills: 'description: ""\ncontent: ""\n',
  instructions: 'text: ""\n',
  secrets: "kind: keychain\n",
  mcp: "command: []\n",
};

interface DefinitionEditorProps {
  readonly kind: string;
  /** Undefined while writing a new definition. */
  readonly definition: Definition | undefined;
  readonly initialId: string;
  readonly home: string;
  readonly schema: ConfigSchema | undefined;
}

function DefinitionEditor({ kind, definition, initialId, home, schema }: DefinitionEditorProps) {
  const [id, setId] = useState(initialId);
  const staged = useStagedDefinition(kind, id);
  const stagedChange = staged?.change.change;
  const original = definition?.yaml ?? newDefinitions[kind] ?? "";
  const [text, setText] = useState(
    stagedChange?.case === "define" ? (stagedChange.value.yaml ?? original) : original,
  );
  const stage = useStaging((state) => state.stage);
  const unstage = useStaging((state) => state.unstage);
  const readOnly = definition?.readOnly ?? false;
  const errors = yamlErrors(text);
  const removing = stagedChange?.case === "undefine";
  const formAvailable = hasForm(kind) && errors.length === 0;
  const search = Route.useSearch();
  const navigate = useNavigate({ from: Route.fullPath });
  const tab = formAvailable ? (search.view ?? "form") : "yaml";
  const saved = stagedChange?.case === "define" ? (stagedChange.value.yaml ?? original) : original;
  const location = definition?.source;
  const editable = !(readOnly && original === "");
  return (
    <Tabs
      value={tab}
      onValueChange={(next) =>
        void navigate({
          search: (current) => ({ ...current, view: next === "yaml" ? "yaml" : "form" }),
          replace: true,
        })
      }
      className="flex min-h-0 flex-1 flex-col gap-3"
    >
      <UnsavedChanges dirty={!readOnly && text !== saved} ignore={["view"]} />
      <PageDetails>
        {editable && (
          <TabsList>
            <TabsTrigger value="form" disabled={!formAvailable}>
              Form
            </TabsTrigger>
            <TabsTrigger value="yaml">YAML</TabsTrigger>
          </TabsList>
        )}
        {definition?.package !== undefined && definition.package !== "" && (
          <Badge variant="secondary">
            <PackageIcon data-icon="inline-start" />
            {definition.package}
          </Badge>
        )}
        {readOnly && <Badge variant="outline">read-only</Badge>}
        {staged !== undefined && <Badge>{removing ? "removal staged" : "staged"}</Badge>}
        {location !== undefined && (
          <span
            className="min-w-0 truncate font-mono text-muted-foreground text-xs"
            title={location.path}
          >
            {`${definedAt(location.path, home)}${location.line > 0 ? `:${location.line}` : ""}`}
          </span>
        )}
      </PageDetails>
      <PageActions>
        {!readOnly && (
          <>
            {definition !== undefined && (
              <ConfirmAction
                trigger={
                  <Button variant="destructive" size="sm">
                    <Trash2Icon data-icon="inline-start" />
                    Remove
                  </Button>
                }
                title={`Remove ${definition.id}?`}
                description="Stages removing the definition and every scope rule naming it."
                action="Stage removal"
                onConfirm={() => stage(undefine(kind, definition.id))}
              />
            )}
            <Button
              variant="outline"
              size="sm"
              disabled={staged === undefined && text === original}
              onClick={() => {
                unstage(`definition/${kind}/${id}`);
                setText(original);
              }}
            >
              <RotateCcwIcon data-icon="inline-start" />
              Revert
            </Button>
            <Button
              size="sm"
              disabled={
                id === "" || errors.length > 0 || (text === original && staged === undefined)
              }
              onClick={() => {
                stage(define(kind, id, text));
                toast.add({ type: "success", title: `Staged ${id}` });
              }}
            >
              Stage
            </Button>
          </>
        )}
      </PageActions>
      {/* A form reads best narrow and centered; YAML takes the page. */}
      <div className={cn("flex w-full flex-col gap-3", tab === "form" && "mx-auto max-w-xl")}>
        {readOnly && (
          <Alert>
            <LockIcon />
            <AlertTitle>
              {definition?.package === "" ? "Its file is read-only" : "A package defines this"}
            </AlertTitle>
            <AlertDescription>
              {definition?.package === ""
                ? "The file is a link, such as one Home Manager manages, or agx cannot write it. Change it where it comes from."
                : "Override it in your own configuration, or change the package."}
            </AlertDescription>
          </Alert>
        )}
        {/* An editable definition's form shows its description itself. */}
        {readOnly && definition?.description !== undefined && definition.description !== "" && (
          <p className="text-muted-foreground text-xs">{definition.description}</p>
        )}
        {definition === undefined && (
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="definition-id">ID</FieldLabel>
              <Input
                id="definition-id"
                className="font-mono"
                placeholder="id"
                spellCheck={false}
                value={id}
                onChange={(event) => setId(event.target.value)}
              />
            </Field>
          </FieldGroup>
        )}
        {editable && (
          <TabsContent value="form">
            <DefinitionForm
              kind={kind}
              form={{
                text,
                onChange: setText,
                readOnly,
                existing: definition !== undefined,
                describe: (field) => fieldDescription(schema, kind, field),
              }}
            />
          </TabsContent>
        )}
      </div>
      {editable && (
        <>
          <TabsContent value="yaml" className="flex min-h-0 flex-1 flex-col gap-2">
            <CodeEditor
              language="yaml"
              fill
              value={text}
              readOnly={readOnly}
              schema={schema === undefined ? undefined : definitionSchema(schema, kind)}
              onChange={setText}
            />
            {errors.length > 0 && <p className="text-destructive text-xs">{errors[0]}</p>}
          </TabsContent>
        </>
      )}
    </Tabs>
  );
}

function FileEditor({
  path,
  file,
  home,
  schema,
}: {
  readonly path: string;
  /** Undefined for a path that is not one of the configuration's files. */
  readonly file: ConfigurationFile | undefined;
  readonly home: string;
  readonly schema: ConfigSchema | undefined;
}) {
  const content = useDaemonQuery(
    queryKeys.file(path),
    Daemon.use((daemon) => daemon.readFile(path)),
  );
  const linkTarget = file?.linkTarget ?? "";
  return (
    <>
      {linkTarget !== "" && (
        <PageDetails>
          <span className="flex min-w-0 items-center gap-1 font-mono text-muted-foreground text-xs">
            <LinkIcon className="size-3 shrink-0" />
            <span className="truncate">{linkTarget}</span>
          </span>
        </PageDetails>
      )}
      {content.isPending && <Pending className="h-64" />}
      {content.isError && <DaemonProblem error={content.error} />}
      {content.isSuccess && (
        <FileText
          path={path}
          original={content.data.content}
          readOnly={file === undefined || file.readOnly || content.data.readOnly}
          home={home}
          schema={schema}
        />
      )}
    </>
  );
}

function FileText({
  path,
  original,
  readOnly,
  home,
  schema,
}: {
  readonly path: string;
  readonly original: string;
  readonly readOnly: boolean;
  readonly home: string;
  readonly schema: ConfigSchema | undefined;
}) {
  const staged = useStagedFile(path);
  const [text, setText] = useState(staged ?? original);
  const stage = useStaging((state) => state.stage);
  const unstage = useStaging((state) => state.unstage);
  const errors = yamlErrors(text);
  return (
    <>
      <UnsavedChanges dirty={!readOnly && text !== (staged ?? original)} />
      {readOnly && (
        <Alert>
          <LockIcon />
          <AlertTitle>Read-only</AlertTitle>
          <AlertDescription>
            A link, such as one Home Manager manages, or a file agx cannot write.
          </AlertDescription>
        </Alert>
      )}
      <CodeEditor
        language="yaml"
        fill
        value={text}
        readOnly={readOnly}
        schema={schema}
        onChange={setText}
      />
      {errors.length > 0 && <p className="text-destructive text-xs">{errors[0]}</p>}
      {!readOnly && (
        <PageActions>
          <Button
            variant="outline"
            size="sm"
            disabled={staged === undefined && text === original}
            onClick={() => {
              unstage(`file/${path}`);
              setText(original);
            }}
          >
            <RotateCcwIcon data-icon="inline-start" />
            Revert
          </Button>
          <Button
            size="sm"
            disabled={errors.length > 0 || (text === original && staged === undefined)}
            onClick={() => {
              stage(writeFile(path, text, home));
              toast.add({ type: "success", title: `Staged ${shortPath(path, home)}` });
            }}
          >
            Stage
          </Button>
        </PageActions>
      )}
    </>
  );
}
