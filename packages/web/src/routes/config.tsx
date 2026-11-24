import type { JsonObject } from "@bufbuild/protobuf";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import type { Definition } from "agx-client/protocol";
import { cn } from "cn";
import * as Schema from "effect/Schema";
import {
  FileCode2Icon,
  ListChecksIcon,
  LockIcon,
  PackageIcon,
  RotateCcwIcon,
  Trash2Icon,
} from "lucide-react";
import { useState } from "react";

import { ConfirmAction } from "~/components/ConfirmAction.tsx";
import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { type Crumb, Page, PageActions, PageViews } from "~/components/Page.tsx";
import { Pending } from "~/components/Pending.tsx";
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "~/components/ui/tabs.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { UnsavedChanges } from "~/components/UnsavedChanges.tsx";
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
import { definitionValue, isJsonObject, sameValue } from "~/features/config/definitionValue.ts";
import { kinds, selectable } from "~/features/config/kinds.ts";
import { ScopeDialog } from "~/features/config/ScopeDialog.tsx";
import {
  define,
  definitionKey,
  undefine,
  useStagedDefinition,
  useStaging,
} from "~/features/config/staging.ts";
import { SettingsTree } from "~/features/explain/SettingsTree.tsx";
import { resourceKey, Traces } from "~/features/explain/Traces.tsx";
import { ServerOverview } from "~/features/mcp/ServerOverview.tsx";
import { useResolution, useScopeDirectory, useScopeHarness } from "~/features/scope/scope.ts";
import { SkillOverview } from "~/features/skills/SkillOverview.tsx";
import { type ConfigSchema, definitionSchema, fieldDescription } from "~/lib/configSchema.ts";
import { shortPath } from "~/lib/path.ts";

const ConfigSearch = Schema.Struct({
  kind: Schema.optional(Schema.String),
  id: Schema.optional(Schema.String),
  /** A new definition of kind is being written. */
  create: Schema.optional(Schema.Boolean),
  /** How the definition is edited, so a link opens the same editor. */
  view: Schema.optional(Schema.Literals(["form", "json"])),
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
 * its definition to edit.
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
    label === undefined
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
      title={[search.tool ?? search.id, label]}
      crumbs={crumbs}
      {...(tab === "overview" ? { className: "max-w-3xl" } : {})}
      actions={
        definition !== undefined &&
        selectable.has(definition.kind) && (
          <ScopeDialog
            kind={definition.kind}
            id={definition.id}
            trigger={
              <Button
                variant="outline"
                size="icon-sm"
                aria-label="Enable or disable…"
                title="Enable or disable…"
              >
                <ListChecksIcon />
              </Button>
            }
          />
        )
      }
      fill={
        tab === "definition" &&
        search.kind !== undefined &&
        (search.view === "json" || !hasForm(search.kind))
      }
    >
      {(configuration.isPending || definitions.isPending) && <Pending className="h-64" />}
      {configuration.isError && <DaemonProblem error={configuration.error} />}
      {definitions.isError && <DaemonProblem error={definitions.error} />}
      {configuration.isSuccess &&
        definitions.isSuccess &&
        (search.kind !== undefined && (search.id !== undefined || creating) ? (
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
const newDefinitions: Readonly<Record<string, JsonObject>> = {
  skills: { description: "", content: "" },
  instructions: { text: "" },
  secrets: { kind: "keychain" },
  mcp: { command: [] },
};

const jsonText = (value: JsonObject) => `${JSON.stringify(value, null, 2)}\n`;

/** The JSON text's value, or why it is not one a definition can have. */
const parseJson = (text: string): JsonObject | string => {
  try {
    const parsed: unknown = JSON.parse(text);
    return isJsonObject(parsed) ? parsed : "A definition is a JSON object.";
  } catch (error) {
    return error instanceof Error ? error.message : String(error);
  }
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
  const original =
    definition === undefined ? (newDefinitions[kind] ?? {}) : definitionValue(definition);
  const saved = staged?.value ?? original;
  const [value, setValue] = useState(saved);
  // The JSON tab's text, kept apart from the value while it does not parse.
  const [text, setText] = useState(() => jsonText(saved));
  const stage = useStaging((state) => state.stage);
  const unstage = useStaging((state) => state.unstage);
  const readOnly = definition?.readOnly ?? false;
  const parsed = parseJson(text);
  const error = typeof parsed === "string" ? parsed : undefined;
  const removing = staged?.change.change?.case === "undefine";
  const formAvailable = hasForm(kind) && error === undefined;
  const search = Route.useSearch();
  const navigate = useNavigate({ from: Route.fullPath });
  const tab = formAvailable ? (search.view ?? "form") : "json";
  const location = definition?.source;
  const change = (next: JsonObject) => {
    setValue(next);
    setText(jsonText(next));
  };
  const dirty = !sameValue(value, saved);
  if (readOnly) {
    return (
      <div className="mx-auto flex w-full max-w-xl flex-col gap-3">
        <div className="flex min-w-0 items-center gap-2">
          {definition?.package !== undefined && definition.package !== "" && (
            <Badge variant="secondary">
              <PackageIcon data-icon="inline-start" />
              {definition.package}
            </Badge>
          )}
          <Badge variant="outline">read-only</Badge>
          {location !== undefined && location.path !== "" && (
            <span
              className="min-w-0 truncate font-mono text-muted-foreground text-xs"
              title={location.path}
            >
              {`${definedAt(location.path, home)}${location.line > 0 ? `:${location.line}` : ""}`}
            </span>
          )}
        </div>
        <Alert>
          <LockIcon />
          <AlertTitle>
            {definition?.package === ""
              ? "A configuration file defines this"
              : "A package defines this"}
          </AlertTitle>
          <AlertDescription>
            {definition?.package === ""
              ? "Change it in its file. Removing it there brings back a local definition of the same name, if there is one."
              : "Change the package, or define one of another name here."}
          </AlertDescription>
        </Alert>
        {definition?.description !== undefined && definition.description !== "" && (
          <p className="text-muted-foreground text-xs">{definition.description}</p>
        )}
      </div>
    );
  }
  return (
    <Tabs
      value={tab}
      onValueChange={(next) =>
        void navigate({
          search: (current) => ({ ...current, view: next === "json" ? "json" : "form" }),
          replace: true,
        })
      }
      className="flex min-h-0 flex-1 flex-col gap-3"
    >
      <UnsavedChanges dirty={dirty || text !== jsonText(value)} ignore={["view"]} />
      <PageActions>
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
          disabled={staged === undefined && sameValue(value, original) && error === undefined}
          onClick={() => {
            unstage(definitionKey(kind, id));
            change(original);
          }}
        >
          <RotateCcwIcon data-icon="inline-start" />
          Revert
        </Button>
        <Button
          size="sm"
          disabled={
            id === "" || error !== undefined || (sameValue(value, original) && staged === undefined)
          }
          onClick={() => {
            stage(define(kind, id, value));
            toast.add({ type: "success", title: `Staged ${id}` });
          }}
        >
          Stage
        </Button>
      </PageActions>
      {/* A form reads best narrow and centered; JSON takes the page. */}
      <div className={cn("flex w-full flex-col gap-3", tab === "form" && "mx-auto max-w-xl")}>
        <div className="flex min-w-0 items-center gap-2">
          <TabsList>
            <TabsTrigger value="form" disabled={!formAvailable}>
              Form
            </TabsTrigger>
            <TabsTrigger value="json">JSON</TabsTrigger>
          </TabsList>
          {staged !== undefined && <Badge>{removing ? "removal staged" : "staged"}</Badge>}
        </div>
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
        <TabsContent value="form">
          <DefinitionForm
            kind={kind}
            form={{
              value,
              onChange: change,
              readOnly,
              existing: definition !== undefined,
              describe: (field) => fieldDescription(schema, kind, field),
            }}
          />
        </TabsContent>
      </div>
      <TabsContent value="json" className="flex min-h-0 flex-1 flex-col gap-2">
        <CodeEditor
          language="json"
          fill
          value={text}
          schema={schema === undefined ? undefined : definitionSchema(schema, kind)}
          onChange={(next) => {
            setText(next);
            const nextValue = parseJson(next);
            if (typeof nextValue !== "string") {
              setValue(nextValue);
            }
          }}
        />
        {error !== undefined && <p className="text-destructive text-xs">{error}</p>}
      </TabsContent>
    </Tabs>
  );
}
