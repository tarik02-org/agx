import { Link } from "@tanstack/react-router";
import * as Option from "effect/Option";
import * as Schema from "effect/Schema";
import { parseDocument } from "yaml";

import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Pending } from "~/components/Pending.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card.tsx";
import { Item, ItemContent, ItemDescription, ItemGroup, ItemTitle } from "~/components/ui/item.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { kinds } from "~/features/config/kinds.ts";
import { SecretActions } from "~/features/runtime/SecretsCard.tsx";
import { secretStates } from "~/features/runtime/states.ts";
import type { Definition } from "~/gen/agx/v1/agx_pb.ts";
import { Markdown } from "~/lib/markdown.tsx";
import { shortPath } from "~/lib/path.ts";

/** A definition's value as written, decoded with a schema; none when it does not match. */
const decodeYaml = <A,>(schema: Schema.Decoder<A>, yaml: string): Option.Option<A> =>
  Schema.decodeUnknownOption(schema)(parseDocument(yaml).toJS());

/** Links to definitions, each with its description. */
function DefinitionLinks({
  title,
  definitions,
  empty,
}: {
  readonly title: string;
  readonly definitions: ReadonlyArray<{
    readonly kind: string;
    readonly id: string;
    /** Set when the ID names no definition, so there is nothing to open. */
    readonly missing?: boolean;
    readonly description: string;
  }>;
  readonly empty: string;
}) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {title} <Badge variant="secondary">{definitions.length}</Badge>
        </CardTitle>
      </CardHeader>
      <CardContent>
        {definitions.length === 0 ? (
          <p className="text-muted-foreground text-xs">{empty}</p>
        ) : (
          <ItemGroup className="gap-1">
            {definitions.map((definition) => (
              <Item
                key={`${definition.kind}/${definition.id}`}
                size="sm"
                variant="outline"
                {...(definition.missing === true
                  ? {}
                  : {
                      render: (
                        <Link to="/config" search={{ kind: definition.kind, id: definition.id }} />
                      ),
                    })}
              >
                <ItemContent>
                  <ItemTitle className="font-mono">{definition.id}</ItemTitle>
                  {definition.description !== "" && (
                    <ItemDescription className="line-clamp-1">
                      {definition.description}
                    </ItemDescription>
                  )}
                </ItemContent>
              </Item>
            ))}
          </ItemGroup>
        )}
      </CardContent>
    </Card>
  );
}

const kindLabel = (kind: string) => kinds.find((k) => k.kind === kind)?.one ?? kind;

const PackageSpec = Schema.Struct({
  git: Schema.optional(Schema.String),
  path: Schema.optional(Schema.String),
  ref: Schema.optional(Schema.String),
  hash: Schema.optional(Schema.String),
  subdir: Schema.optional(Schema.String),
});

/** Where a package comes from, and the definitions it provides. */
export function PackageOverview({
  definition,
  definitions,
}: {
  readonly definition: Definition | undefined;
  readonly definitions: ReadonlyArray<Definition>;
}) {
  const id = definition?.id ?? "";
  const spec = decodeYaml(PackageSpec, definition?.yaml ?? "").pipe(Option.getOrUndefined);
  const source: ReadonlyArray<readonly [string, string | undefined]> = [
    ["Git", spec?.git],
    ["Path", spec?.path],
    ["Ref", spec?.ref],
    ["Hash", spec?.hash],
    ["Subdirectory", spec?.subdir],
  ];
  const provided = definitions
    .filter((d) => d.package === id && d.kind !== "packages")
    .map((d) => ({
      kind: d.kind,
      id: d.id,
      description: [kindLabel(d.kind), d.description].filter((part) => part !== "").join(" · "),
    }));
  return (
    <>
      {spec !== undefined && (
        <Card size="sm">
          <CardHeader>
            <CardTitle>Source</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-xs">
              {source
                .filter(([, value]) => value !== undefined && value !== "")
                .map(([label, value]) => (
                  <div key={label} className="contents">
                    <dt className="text-muted-foreground">{label}</dt>
                    <dd className="break-all font-mono">{value}</dd>
                  </div>
                ))}
            </dl>
          </CardContent>
        </Card>
      )}
      <DefinitionLinks
        title="Provides"
        definitions={provided}
        empty="Nothing yet: install the configuration's packages from the runtime page."
      />
    </>
  );
}

// A bundle lists definitions by ID, or by path or inline, which name nothing to open.
const Member = Schema.Union([Schema.String, Schema.Struct({ id: Schema.optional(Schema.String) })]);
const BundleSpec = Schema.Struct({
  skills: Schema.optional(Schema.Array(Member)),
  instructions: Schema.optional(Schema.Array(Member)),
  mcp: Schema.optional(Schema.Array(Member)),
  bundles: Schema.optional(Schema.Array(Schema.String)),
});

/** What a bundle adds, by kind. */
export function BundleOverview({
  definition,
  definitions,
}: {
  readonly definition: Definition | undefined;
  readonly definitions: ReadonlyArray<Definition>;
}) {
  const spec = decodeYaml(BundleSpec, definition?.yaml ?? "").pipe(Option.getOrUndefined);
  const members = (["skills", "instructions", "mcp", "bundles"] as const).flatMap((kind) =>
    (spec?.[kind] ?? []).map((member) => {
      const id = typeof member === "string" ? member : (member.id ?? "inline");
      const known = definitions.find((d) => d.kind === kind && d.id === id);
      return {
        kind,
        id,
        missing: known === undefined,
        description: [kindLabel(kind), known?.description ?? ""]
          .filter((part) => part !== "")
          .join(" · "),
      };
    }),
  );
  return <DefinitionLinks title="Adds" definitions={members} empty="It adds nothing." />;
}

const escapeRegExp = (text: string) => text.replaceAll(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`);

/** A secret's state, what can be done about it, and the MCP servers that use it. */
export function SecretOverview({
  id,
  definitions,
}: {
  readonly id: string;
  readonly definitions: ReadonlyArray<Definition>;
}) {
  const secrets = useDaemonQuery(
    queryKeys.secrets,
    Daemon.use((daemon) => daemon.secrets),
  );
  const secret = secrets.data?.find((s) => s.secretId === id);
  // Servers name a secret with the !secret tag in a value, or as their auth's secret.
  const reference = new RegExp(String.raw`(?:!secret\s+|secret:\s*)${escapeRegExp(id)}(?![\w-])`);
  const users = definitions
    .filter((d) => d.kind === "mcp" && (reference.test(d.yaml) || d.id === secret?.mcpId))
    .map((d) => ({ kind: d.kind, id: d.id, description: d.description }));
  return (
    <>
      <Card size="sm">
        <CardHeader>
          <CardTitle>State</CardTitle>
        </CardHeader>
        <CardContent>
          {secrets.isPending && <Pending className="h-8" />}
          {secrets.isError && <DaemonProblem error={secrets.error} />}
          {secrets.isSuccess && secret === undefined && (
            <p className="text-muted-foreground text-xs">The daemon does not know it yet.</p>
          )}
          {secret !== undefined && (
            <div className="flex flex-wrap items-center gap-2 text-xs">
              <Badge variant={secretStates[secret.state].variant}>
                {secretStates[secret.state].label}
              </Badge>
              <span className="text-muted-foreground">{secret.kind}</span>
              {secret.diagnostics.length > 0 && (
                <span className="text-muted-foreground">
                  {secret.diagnostics.map((d) => d.summary).join("; ")}
                </span>
              )}
              <div className="ml-auto">
                <SecretActions secret={secret} />
              </div>
            </div>
          )}
        </CardContent>
      </Card>
      <DefinitionLinks title="Used by" definitions={users} empty="No MCP server uses it." />
    </>
  );
}

const InstructionsSpec = Schema.Struct({ text: Schema.optional(Schema.String) });

/** The instructions a launch gets: the text written inline, or the file they come from. */
export function InstructionsOverview({
  definition,
  home,
}: {
  readonly definition: Definition | undefined;
  readonly home: string;
}) {
  const text = decodeYaml(InstructionsSpec, definition?.yaml ?? "").pipe(
    Option.flatMap((spec) => Option.fromUndefinedOr(spec.text)),
    Option.getOrUndefined,
  );
  if (text === undefined && (definition?.path ?? "") === "") {
    return null;
  }
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Instructions</CardTitle>
      </CardHeader>
      <CardContent>
        {text === undefined ? (
          <p className="break-all font-mono text-muted-foreground text-xs">
            {shortPath(definition?.path ?? "", home)}
          </p>
        ) : (
          <Markdown>{text}</Markdown>
        )}
      </CardContent>
    </Card>
  );
}
