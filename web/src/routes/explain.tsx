import { createFileRoute, useNavigate } from "@tanstack/react-router";
import * as Schema from "effect/Schema";
import { ScanSearchIcon, TriangleAlertIcon } from "lucide-react";

import { Page } from "~/components/Page.tsx";
import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Alert, AlertDescription, AlertTitle } from "~/components/ui/alert.tsx";
import {
  Card,
  CardContent,
  CardDescription,
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select.tsx";
import { Pending } from "~/components/Pending.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { useScopeDirectory } from "~/features/scope/scope.ts";
import { SettingsTree } from "~/features/explain/SettingsTree.tsx";
import { resourceKey, Traces } from "~/features/explain/Traces.tsx";
import {
  Diagnostic_Severity,
  type GetConfigurationResponse,
  type ResolveResponse,
} from "~/gen/agx/v1/agx_pb.ts";

// The address bar is a boundary: what it holds is decoded, not trusted.
const ExplainSearch = Schema.Struct({
  harness: Schema.optional(Schema.String),
});

export const Route = createFileRoute("/explain")({
  validateSearch: Schema.decodeUnknownSync(ExplainSearch),
  component: ExplainPage,
});

function ExplainPage() {
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const search = Route.useSearch();
  const navigate = useNavigate({ from: Route.fullPath });
  const harnesses = [
    { label: "No harness", value: "" },
    ...(configuration.data?.harnesses ?? []).map((h) => ({ label: h.name, value: h.name })),
  ];
  return (
    <Page
      title={[search.harness, "Explain"]}
      crumbs={[{ label: "Explain" }]}
      actions={
        <Select
          items={harnesses}
          value={search.harness ?? ""}
          onValueChange={(next) =>
            void navigate({
              search: { ...search, harness: next === null || next === "" ? undefined : next },
            })
          }
        >
          <SelectTrigger size="sm" className="w-44" aria-label="Harness">
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {harnesses.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      }
    >
      {configuration.isPending && <Pending className="h-40" />}
      {configuration.isError && <DaemonProblem error={configuration.error} />}
      {configuration.isSuccess && <Explain configuration={configuration.data} />}
    </Page>
  );
}

function Explain({ configuration }: { readonly configuration: GetConfigurationResponse }) {
  const search = Route.useSearch();
  // The directory is the UI's scope, picked in the top bar.
  const directory = useScopeDirectory();
  const harness = search.harness ?? "";
  return (
    <>
      {directory === "" ? (
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <ScanSearchIcon />
            </EmptyMedia>
            <EmptyTitle>Scope the UI to a directory</EmptyTitle>
            <EmptyDescription>
              Pick one in the top bar to see every skill, instruction, and MCP server it gets, and
              the rule that decided each.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <Resolution directory={directory} harness={harness} home={configuration.home} />
      )}
    </>
  );
}

function Resolution({
  directory,
  harness,
  home,
}: {
  readonly directory: string;
  readonly harness: string;
  readonly home: string;
}) {
  const absolute =
    directory.startsWith("~/") && home !== "" ? `${home}${directory.slice(1)}` : directory;
  const resolved = useDaemonQuery(
    queryKeys.resolve(absolute, harness),
    Daemon.use((daemon) => daemon.resolve(absolute, harness)),
  );
  if (resolved.isPending) {
    return <Pending className="h-40" />;
  }
  if (resolved.isError) {
    return <DaemonProblem error={resolved.error} />;
  }
  return <ResolutionView response={resolved.data} home={home} />;
}

function ResolutionView({
  response,
  home,
}: {
  readonly response: ResolveResponse;
  readonly home: string;
}) {
  const selected = new Set(
    (response.snapshot?.resources ?? []).map((ref) => {
      const { kind, id } = resourceKey(ref);
      return `${kind}/${id}`;
    }),
  );
  const warnings = response.diagnostics.filter((d) => d.severity === Diagnostic_Severity.WARNING);
  return (
    <>
      {warnings.map((warning) => (
        <Alert key={warning.summary}>
          <TriangleAlertIcon />
          <AlertTitle>{warning.summary}</AlertTitle>
          <AlertDescription>
            Launches here go without it until you run agx install.
          </AlertDescription>
        </Alert>
      ))}
      <Card size="sm">
        <CardHeader>
          <CardTitle>Resources</CardTitle>
          <CardDescription>
            {selected.size} selected
            {(response.snapshot?.requiredEnvironment.length ?? 0) > 0 &&
              ` · reads ${response.snapshot?.requiredEnvironment.join(", ")} from the shell`}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {response.traces.length === 0 ? (
            <p className="text-muted-foreground">No rule applies to this directory.</p>
          ) : (
            <Traces traces={response.traces} selected={selected} home={home} />
          )}
        </CardContent>
      </Card>
      {response.settings.length > 0 && (
        <Card size="sm">
          <CardHeader>
            <CardTitle>Harness settings</CardTitle>
          </CardHeader>
          <CardContent>
            <SettingsTree settings={response.settings} home={home} />
          </CardContent>
        </Card>
      )}
    </>
  );
}
