import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { cn } from "cn";
import * as Schema from "effect/Schema";
import {
  ChevronDownIcon,
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
import { Page, PageActions } from "~/components/Page.tsx";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu.tsx";
import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Alert, AlertDescription, AlertTitle } from "~/components/ui/alert.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { Button } from "~/components/ui/button.tsx";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "~/components/ui/empty.tsx";
import { Field, FieldGroup, FieldLabel } from "~/components/ui/field.tsx";
import { Input } from "~/components/ui/input.tsx";
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
});

export const Route = createFileRoute("/config")({
  validateSearch: Schema.decodeUnknownSync(ConfigSearch),
  component: ConfigPage,
});

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
  const home = configuration.data?.home ?? "";
  const label = kinds.find((k) => k.kind === search.kind)?.label ?? search.kind;
  const definition = definitions.data?.find(
    (candidate) => candidate.kind === search.kind && candidate.id === search.id,
  );
  return (
    <Page
      title={[search.id ?? search.file?.split("/").at(-1), "Configuration"]}
      crumbs={[
        { label: "Configuration" },
        ...(search.file !== undefined
          ? [{ label: shortPath(search.file, home), mono: true }]
          : label !== undefined
            ? [{ label }, { label: search.id ?? "new", mono: true }]
            : []),
      ]}
      actions={<NewDefinitionMenu />}
      fill={
        search.file !== undefined ||
        (search.kind !== undefined && (search.view === "yaml" || !hasForm(search.kind)))
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
        ) : search.kind !== undefined && (search.id !== undefined || search.create === true) ? (
          <DefinitionEditor
            key={`${search.kind}/${search.id ?? ""}/${String(search.create)}`}
            kind={search.kind}
            definition={definition}
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
              <EmptyTitle>Pick a definition or file in the sidebar</EmptyTitle>
              <EmptyDescription>
                Edits are staged; review and apply them together from Staged changes.
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ))}
    </Page>
  );
}

/** Starts a new definition of any kind in the editor. */
function NewDefinitionMenu() {
  const navigate = useNavigate({ from: Route.fullPath });
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button variant="outline" size="sm">
            <PlusIcon data-icon="inline-start" />
            New
            <ChevronDownIcon data-icon="inline-end" />
          </Button>
        }
      />
      <DropdownMenuContent align="end">
        <DropdownMenuGroup>
          {kinds.map(({ kind, label }) => (
            <DropdownMenuItem
              key={kind}
              onClick={() =>
                void navigate({ search: (current) => ({ dir: current.dir, kind, create: true }) })
              }
            >
              {label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

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
  const writable = files.filter((file) => !file.readOnly);
  const [id, setId] = useState(initialId);
  const [file, setFile] = useState(writable[0]?.path ?? "");
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
      <PageActions>
        <span className="flex items-center gap-2">
          {definition?.package !== undefined && definition.package !== "" && (
            <Badge variant="secondary">
              <PackageIcon data-icon="inline-start" />
              {definition.package}
            </Badge>
          )}
          {readOnly && <Badge variant="outline">read-only</Badge>}
          {staged !== undefined && <Badge>{removing ? "removal staged" : "staged"}</Badge>}
          {location !== undefined && (
            <span className="font-mono text-muted-foreground text-xs">
              {`${shortPath(location.path, home)}${location.line > 0 ? `:${location.line}` : ""}`}
            </span>
          )}
        </span>
        {editable && (
          <TabsList>
            <TabsTrigger value="form" disabled={!formAvailable}>
              Form
            </TabsTrigger>
            <TabsTrigger value="yaml">YAML</TabsTrigger>
          </TabsList>
        )}
        {definition !== undefined && selectable.has(kind) && (
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
        )}
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
                stage(define(kind, id, text, definition === undefined ? file : ""));
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
            <Field>
              <FieldLabel>Write to</FieldLabel>
              <Select
                items={writable.map((f) => ({ value: f.path, label: shortPath(f.path, home) }))}
                value={file}
                onValueChange={(next) => setFile(next ?? "")}
              >
                <SelectTrigger className="font-mono">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
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
        <PageActions>
          <span className="flex items-center gap-1 font-mono text-muted-foreground text-xs">
            <LinkIcon className="size-3" />
            {linkTarget}
          </span>
        </PageActions>
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
