import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import * as Schema from "effect/Schema";
import {
  FileCode2Icon,
  LinkIcon,
  ListChecksIcon,
  LockIcon,
  PackageIcon,
  PlusIcon,
  RotateCcwIcon,
  Trash2Icon,
} from "lucide-react";
import { useState } from "react";

import { ConfirmAction } from "~/components/ConfirmAction.tsx";
import { UnsavedChanges } from "~/components/UnsavedChanges.tsx";
import { ButtonLink } from "~/components/ButtonLink.tsx";
import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Alert, AlertDescription, AlertTitle } from "~/components/ui/alert.tsx";
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
import { Field, FieldLabel } from "~/components/ui/field.tsx";
import { Input } from "~/components/ui/input.tsx";
import { ScrollArea } from "~/components/ui/scroll-area.tsx";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select.tsx";
import { Pending } from "~/components/Pending.tsx";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "~/components/ui/tabs.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { CodeEditor } from "~/features/config/CodeEditor.tsx";
import { DefinitionForm, hasForm } from "~/features/config/DefinitionForm.tsx";
import { ScopeDialog } from "~/features/config/ScopeDialog.tsx";
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
import { shortPath } from "~/lib/path.ts";
import { pageTitle } from "~/lib/title.ts";

const ConfigSearch = Schema.Struct({
  kind: Schema.optional(Schema.String),
  id: Schema.optional(Schema.String),
  /** A new definition of kind is being written. */
  create: Schema.optional(Schema.Boolean),
  file: Schema.optional(Schema.String),
  /** How the definition is edited, so a link opens the same editor. */
  view: Schema.optional(Schema.Literals(["form", "yaml"])),
});

export const Route = createFileRoute("/config")({
  validateSearch: Schema.decodeUnknownSync(ConfigSearch),
  component: ConfigPage,
});

const kinds: ReadonlyArray<{ readonly kind: string; readonly label: string }> = [
  { kind: "mcp", label: "MCP servers" },
  { kind: "skills", label: "Skills" },
  { kind: "instructions", label: "Instructions" },
  { kind: "secrets", label: "Secrets" },
  { kind: "bundles", label: "Bundles" },
  { kind: "packages", label: "Packages" },
  { kind: "harnesses", label: "Harnesses" },
];

/** Kinds scope rules select, so they can be enabled and disabled. */
const selectable = new Set(["skills", "instructions", "mcp", "bundles"]);

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
  return (
    <div className="flex h-full min-h-0 flex-col gap-4 p-4">
      <title>{pageTitle(search.id ?? search.file?.split("/").at(-1), "Configuration")}</title>
      <h1 className="font-heading font-semibold text-sm">Configuration</h1>
      {(configuration.isPending || definitions.isPending) && <Pending className="h-64" />}
      {configuration.isError && <DaemonProblem error={configuration.error} />}
      {definitions.isError && <DaemonProblem error={definitions.error} />}
      {configuration.isSuccess && definitions.isSuccess && (
        <div className="grid min-h-0 flex-1 grid-cols-[18rem_minmax(0,1fr)] gap-4">
          <Navigator
            definitions={definitions.data}
            files={configuration.data.files}
            home={configuration.data.home}
          />
          <ScrollArea className="min-h-0">
            <div className="pr-3">
              {search.file !== undefined ? (
                <FileEditor
                  key={search.file}
                  path={search.file}
                  file={configuration.data.files.find((file) => file.path === search.file)}
                  home={configuration.data.home}
                  schema={schema.data}
                />
              ) : search.kind !== undefined &&
                (search.id !== undefined || search.create === true) ? (
                <DefinitionEditor
                  key={`${search.kind}/${search.id ?? ""}/${String(search.create)}`}
                  kind={search.kind}
                  definition={definitions.data.find(
                    (definition) => definition.kind === search.kind && definition.id === search.id,
                  )}
                  initialId={search.id ?? ""}
                  files={configuration.data.files}
                  home={configuration.data.home}
                  schema={schema.data}
                />
              ) : (
                <Empty>
                  <EmptyHeader>
                    <EmptyMedia variant="icon">
                      <FileCode2Icon />
                    </EmptyMedia>
                    <EmptyTitle>Pick a definition or file</EmptyTitle>
                    <EmptyDescription>
                      Edits are staged; review and apply them together from Staged changes.
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              )}
            </div>
          </ScrollArea>
        </div>
      )}
    </div>
  );
}

function Navigator({
  definitions,
  files,
  home,
}: {
  readonly definitions: ReadonlyArray<Definition>;
  readonly files: ReadonlyArray<ConfigurationFile>;
  readonly home: string;
}) {
  const search = Route.useSearch();
  const [filter, setFilter] = useState("");
  const matches = (text: string) => text.toLowerCase().includes(filter.toLowerCase());
  return (
    <Card size="sm" className="min-h-0">
      <CardHeader>
        <Input
          type="search"
          aria-label="Filter definitions"
          placeholder="Filter definitions…"
          autoComplete="off"
          spellCheck={false}
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
        />
      </CardHeader>
      <ScrollArea className="min-h-0 flex-1">
        <div className="flex flex-col gap-3 px-3 pb-3">
          {kinds.map(({ kind, label }) => {
            const own = definitions.filter(
              (d) => d.kind === kind && (filter === "" || matches(d.id)),
            );
            return (
              <section key={kind} className="flex flex-col gap-0.5">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-muted-foreground text-xs">{label}</span>
                  <Badge variant="secondary">{own.length}</Badge>
                  <ButtonLink
                    variant="ghost"
                    size="icon-sm"
                    className="ml-auto"
                    aria-label={`New ${label}`}
                    to="/config"
                    search={{ kind, create: true }}
                  >
                    <PlusIcon />
                  </ButtonLink>
                </div>
                {own.map((definition) => (
                  <ButtonLink
                    key={definition.id}
                    variant={
                      search.kind === kind && search.id === definition.id ? "secondary" : "ghost"
                    }
                    size="sm"
                    className="w-full justify-start font-mono"
                    to="/config"
                    search={{ kind, id: definition.id }}
                  >
                    <span className="truncate">{definition.id}</span>
                    {definition.package !== "" && <PackageIcon data-icon="inline-end" />}
                    {definition.readOnly && definition.package === "" && (
                      <LockIcon data-icon="inline-end" />
                    )}
                  </ButtonLink>
                ))}
              </section>
            );
          })}
          <section className="flex flex-col gap-0.5">
            <span className="font-medium text-muted-foreground text-xs">Files</span>
            {files.map((file) => (
              <ButtonLink
                key={file.path}
                variant={search.file === file.path ? "secondary" : "ghost"}
                size="sm"
                className="w-full justify-start font-mono"
                to="/config"
                search={{ file: file.path }}
              >
                <span className="truncate">{shortPath(file.path, home)}</span>
                {file.readOnly && <LockIcon data-icon="inline-end" />}
              </ButtonLink>
            ))}
          </section>
        </div>
      </ScrollArea>
    </Card>
  );
}

interface DefinitionEditorProps {
  readonly kind: string;
  /** Undefined while writing a new definition. */
  readonly definition: Definition | undefined;
  readonly initialId: string;
  readonly files: ReadonlyArray<ConfigurationFile>;
  readonly home: string;
  readonly schema: ConfigSchema | undefined;
}

function DefinitionEditor({
  kind,
  definition,
  initialId,
  files,
  home,
  schema,
}: DefinitionEditorProps) {
  const label = kinds.find((k) => k.kind === kind)?.label ?? kind;
  const writable = files.filter((file) => !file.readOnly);
  const [id, setId] = useState(initialId);
  const [file, setFile] = useState(writable[0]?.path ?? "");
  const staged = useStagedDefinition(kind, id);
  const stagedChange = staged?.change.change;
  const original = definition?.yaml ?? "";
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
  return (
    <Card size="sm">
      <UnsavedChanges dirty={!readOnly && text !== saved} ignore={["view"]} />
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <span className="text-muted-foreground">{label}</span>
          {definition === undefined ? (
            <Input
              aria-label="ID"
              className="w-72 font-mono"
              placeholder="id"
              spellCheck={false}
              value={id}
              onChange={(event) => setId(event.target.value)}
            />
          ) : (
            <span className="font-mono">{definition.id}</span>
          )}
          {definition?.package !== undefined && definition.package !== "" && (
            <Badge variant="secondary">
              <PackageIcon data-icon="inline-start" />
              {definition.package}
            </Badge>
          )}
          {readOnly && <Badge variant="outline">read-only</Badge>}
          {staged !== undefined && <Badge>{removing ? "removal staged" : "staged"}</Badge>}
        </CardTitle>
        <CardDescription className="font-mono">
          {location === undefined
            ? "Not written yet"
            : `${shortPath(location.path, home)}${location.line > 0 ? `:${location.line}` : ""}`}
        </CardDescription>
        {definition !== undefined && selectable.has(kind) && (
          <CardAction>
            <ScopeDialog
              kind={kind}
              id={definition.id}
              trigger={
                <Button variant="outline" size="sm">
                  <ListChecksIcon data-icon="inline-start" />
                  Enable or disable…
                </Button>
              }
            />
          </CardAction>
        )}
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
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
        {definition?.description !== undefined && definition.description !== "" && (
          <p className="text-muted-foreground text-xs">{definition.description}</p>
        )}
        {kind === "skills" && definition !== undefined && (
          <Link
            to="/skills"
            search={{ skill: definition.id }}
            className="text-primary text-xs underline"
          >
            Open in Skills
          </Link>
        )}
        {definition === undefined && (
          <Field>
            <FieldLabel>Write to</FieldLabel>
            <Select
              items={writable.map((f) => ({ value: f.path, label: shortPath(f.path, home) }))}
              value={file}
              onValueChange={(next) => setFile(next ?? "")}
            >
              <SelectTrigger className="w-96 font-mono">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {writable.map((f) => (
                    <SelectItem key={f.path} value={f.path} className="font-mono">
                      {shortPath(f.path, home)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
        )}
        {!(readOnly && original === "") && (
          <Tabs
            value={tab}
            onValueChange={(next) =>
              void navigate({
                search: (current) => ({ ...current, view: next === "yaml" ? "yaml" : "form" }),
                replace: true,
              })
            }
          >
            <TabsList>
              <TabsTrigger value="form" disabled={!formAvailable}>
                Form
              </TabsTrigger>
              <TabsTrigger value="yaml">YAML</TabsTrigger>
            </TabsList>
            <TabsContent value="form">
              <DefinitionForm
                kind={kind}
                form={{
                  text,
                  onChange: setText,
                  readOnly,
                  describe: (field) => fieldDescription(schema, kind, field),
                }}
              />
            </TabsContent>
            <TabsContent value="yaml">
              <CodeEditor
                language="yaml"
                className="min-h-48"
                value={text}
                readOnly={readOnly}
                schema={schema === undefined ? undefined : definitionSchema(schema, kind)}
                onChange={setText}
              />
              {errors.length > 0 && <p className="mt-2 text-destructive text-xs">{errors[0]}</p>}
            </TabsContent>
          </Tabs>
        )}
      </CardContent>
      {!readOnly && (
        <CardFooter className="gap-2">
          <Button
            disabled={id === "" || errors.length > 0 || (text === original && staged === undefined)}
            onClick={() => {
              stage(define(kind, id, text, definition === undefined ? file : ""));
              toast.add({ type: "success", title: `Staged ${id}` });
            }}
          >
            Stage
          </Button>
          <Button
            variant="outline"
            disabled={staged === undefined && text === original}
            onClick={() => {
              unstage(`definition/${kind}/${id}`);
              setText(original);
            }}
          >
            <RotateCcwIcon data-icon="inline-start" />
            Revert
          </Button>
          {definition !== undefined && (
            <ConfirmAction
              trigger={
                <Button variant="destructive" className="ml-auto">
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
        </CardFooter>
      )}
    </Card>
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
    <Card size="sm">
      <CardHeader>
        <CardTitle className="font-mono">{shortPath(path, home)}</CardTitle>
        {linkTarget !== "" && (
          <CardDescription className="flex items-center gap-1 font-mono">
            <LinkIcon />
            {linkTarget}
          </CardDescription>
        )}
      </CardHeader>
      {content.isPending && (
        <CardContent>
          <Pending className="h-64" />
        </CardContent>
      )}
      {content.isError && (
        <CardContent>
          <DaemonProblem error={content.error} />
        </CardContent>
      )}
      {content.isSuccess && (
        <FileText
          path={path}
          original={content.data.content}
          readOnly={file === undefined || file.readOnly || content.data.readOnly}
          home={home}
          schema={schema}
        />
      )}
    </Card>
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
      <CardContent className="flex flex-col gap-2">
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
          value={text}
          readOnly={readOnly}
          schema={schema}
          onChange={setText}
        />
        {errors.length > 0 && <p className="text-destructive text-xs">{errors[0]}</p>}
      </CardContent>
      {!readOnly && (
        <CardFooter className="gap-2">
          <Button
            disabled={errors.length > 0 || (text === original && staged === undefined)}
            onClick={() => {
              stage(writeFile(path, text, home));
              toast.add({ type: "success", title: `Staged ${shortPath(path, home)}` });
            }}
          >
            Stage
          </Button>
          <Button
            variant="outline"
            disabled={staged === undefined && text === original}
            onClick={() => {
              unstage(`file/${path}`);
              setText(original);
            }}
          >
            <RotateCcwIcon data-icon="inline-start" />
            Revert
          </Button>
        </CardFooter>
      )}
    </>
  );
}
