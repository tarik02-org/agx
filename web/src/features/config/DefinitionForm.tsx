import * as Effect from "effect/Effect";
import { FolderIcon, HashIcon } from "lucide-react";
import { type ReactNode, useId, useRef, useState } from "react";

import { Badge } from "~/components/ui/badge.tsx";
import { InputGroupButton } from "~/components/ui/input-group.tsx";
import { Popover, PopoverContent, PopoverTrigger } from "~/components/ui/popover.tsx";
import { Spinner } from "~/components/ui/spinner.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Tooltip, TooltipContent, TooltipTrigger } from "~/components/ui/tooltip.tsx";
import { Field, FieldGroup, FieldSeparator } from "~/components/ui/field.tsx";
import { Input } from "~/components/ui/input.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonMutation, useDaemonQuery } from "~/daemon/query.ts";
import type { SourceRef } from "~/gen/agx/v1/agx_pb.ts";
import { CodeEditor } from "~/features/config/CodeEditor.tsx";
import { shortPath } from "~/lib/path.ts";
import { DirectoryBrowser } from "~/features/scope/DirectoryBrowser.tsx";
import { absoluteDirectory } from "~/features/scope/scope.ts";
import {
  BooleanField,
  ChoiceField,
  type DefinitionText,
  HintedLabel,
  ListField,
  type Option,
  OptionSelect,
  SuggestField,
  TagsField,
  MapField,
  NumberField,
  TextField,
  ValueField,
  VariantField,
} from "~/features/config/fields.tsx";
import {
  ensureField,
  type FieldPath,
  putFields,
  type TakenFields,
  takeFields,
  hasField,
  isStructured,
  readString,
  removeFields,
  writeField,
} from "~/features/config/yamlDocument.ts";

/** The kinds with a form; the others are edited as YAML. */
export const hasForm = (kind: string): boolean =>
  kind === "mcp" ||
  kind === "secrets" ||
  kind === "packages" ||
  kind === "skills" ||
  kind === "instructions";

export function DefinitionForm({
  kind,
  form,
}: {
  readonly kind: string;
  readonly form: DefinitionText;
}) {
  switch (kind) {
    case "mcp":
      return <McpForm form={form} />;
    case "secrets":
      return <SecretForm form={form} />;
    case "packages":
      return <PackageForm form={form} />;
    case "skills":
      return <SkillForm form={form} />;
    case "instructions":
      return <InstructionsForm form={form} />;
    default:
      return null;
  }
}

// A field written at all marks the source, even empty while it is being filled in.
const has = (form: DefinitionText, field: string) => hasField(form.text, [field]);

// Where a configured definition comes from is fixed: a new source makes a different thing, so
// it is removed and added again rather than edited. Its pins stay editable.
const sourceForm = (form: DefinitionText): DefinitionText =>
  form.existing ? { ...form, readOnly: true } : form;

const sourceNote = (form: DefinitionText) =>
  form.existing
    ? "Fixed once configured: remove it and add it again to take it from elsewhere."
    : undefined;

/**
 * Switches where a definition comes from: drops the fields only the previous source had, keeping
 * them to put back if the source is switched back, and writes the new source's own field, empty,
 * so the switch shows before it is filled in.
 */
const useSourceSwitch = (form: DefinitionText) => {
  const kept = useRef(new Map<string, TakenFields>());
  return (from: string, to: string, drop: ReadonlyArray<string>, mark?: string) => {
    kept.current.set(from, takeFields(form.text, drop));
    let text = removeFields(form.text, drop);
    const back = kept.current.get(to);
    if (back !== undefined) {
      text = putFields(text, back);
    }
    form.onChange(mark === undefined ? text : ensureField(text, [mark]));
  };
};

function McpForm({ form }: { readonly form: DefinitionText }) {
  const transport = has(form, "url") ? "http" : "stdio";
  const switchSource = useSourceSwitch(form);
  return (
    <FieldGroup>
      <TextField form={form} path={["description"]} label="Description" />
      <VariantField
        label="Transport"
        value={transport}
        readOnly={form.readOnly}
        options={[
          { value: "stdio", label: "Command", description: "A local process agx starts." },
          { value: "http", label: "HTTP", description: "A server at a URL." },
        ]}
        onChange={(next) =>
          switchSource(
            transport,
            next,
            next === "http"
              ? ["command", "cwd", "environment", "transport", "share"]
              : ["url", "headers", "auth", "transport"],
            next === "http" ? "url" : undefined,
          )
        }
      />
      {transport === "stdio" ? (
        <>
          <ListField
            form={form}
            path={["command"]}
            label="Command"
            placeholder={"npx\n-y\n@modelcontextprotocol/server-filesystem"}
            description="The command and its arguments, one per line."
          />
          <TextField
            form={form}
            path={["cwd"]}
            label="Working directory"
            placeholder="the launch directory"
            action={!form.readOnly && <BrowseFolder form={form} path={["cwd"]} />}
          />
          <MapField form={form} path={["environment"]} label="Environment" />
          <ShareField form={form} />
        </>
      ) : (
        <>
          <TextField form={form} path={["url"]} label="URL" placeholder="https://example.com/mcp" />
          <MapField form={form} path={["headers"]} label="Headers" placeholder="Header-Name" />
          <AuthField form={form} />
        </>
      )}
      <FieldSeparator />
      <ChoiceField
        form={form}
        path={["load"]}
        label="Load"
        empty="native (default)"
        options={[{ value: "on_demand", label: "on demand" }]}
      />
      <BooleanField form={form} path={["override"]} label="Override" />
    </FieldGroup>
  );
}

const shares: ReadonlyArray<Option> = [
  { value: "none", label: "None (default)", description: "A process for each session." },
  { value: "directory", label: "Directory", description: "One for every launch in a directory." },
  { value: "repo", label: "Repository", description: "One for every launch in a repository." },
  { value: "global", label: "Global", description: "One for every launch, wherever it runs." },
];

// share is a type, or {type, idle} when it also sets how long the process stays.
function ShareField({ form }: { readonly form: DefinitionText }) {
  const typeId = useId();
  const idleId = useId();
  const structured = isStructured(form.text, ["share"]);
  const written = readString(form.text, structured ? ["share", "type"] : ["share"]);
  const type = written === "" ? "none" : written;
  const idle = structured ? readString(form.text, ["share", "idle"]) : "";
  const write = (nextType: string, nextIdle: string) => {
    const cleared = removeFields(form.text, ["share"]);
    if (nextIdle === "") {
      form.onChange(nextType === "none" ? cleared : writeField(cleared, ["share"], nextType));
      return;
    }
    form.onChange(
      writeField(writeField(cleared, ["share", "type"], nextType), ["share", "idle"], nextIdle),
    );
  };
  return (
    <Field>
      <HintedLabel
        htmlFor={typeId}
        hint="Shared processes stay this long after their last session; 5m when empty."
      >
        Share
      </HintedLabel>
      <div className="flex gap-2">
        <OptionSelect
          id={typeId}
          value={type}
          options={shares}
          disabled={form.readOnly}
          onChange={(next) => write(next, idle)}
        />
        {type !== "none" && (
          <Input
            id={idleId}
            aria-label="Idle"
            className="w-32 font-mono"
            placeholder="idle 5m"
            readOnly={form.readOnly}
            value={idle}
            onChange={(event) => write(type, event.target.value)}
          />
        )}
      </div>
    </Field>
  );
}

// Scopes many providers know; any other can be typed.
const oauthScopes = ["openid", "profile", "email", "offline_access", "read", "write"];

const secretKinds = [
  {
    value: "keychain",
    label: "Keychain",
    description: "Kept in the OS keychain; set on the Runtime page.",
  },
  {
    value: "oauth",
    label: "OAuth sign-in",
    description: "A sign-in, for an MCP server's auth only.",
  },
  {
    value: "environment",
    label: "Launch environment",
    description: "A variable of the shell you launch from.",
  },
  {
    value: "daemon_environment",
    label: "Daemon environment",
    description: "A variable of the shell that started the daemon.",
  },
  {
    value: "command",
    label: "Command output",
    description: "What a command, such as a password manager, prints.",
  },
  { value: "file", label: "File", description: "The contents of a file." },
  { value: "sops", label: "SOPS file", description: "A key of a SOPS-encrypted file." },
];

function SecretForm({ form }: { readonly form: DefinitionText }) {
  const kind = readString(form.text, ["kind"]);
  return (
    <FieldGroup>
      <ChoiceField form={form} path={["kind"]} label="Kind" options={secretKinds} />
      {(kind === "environment" || kind === "daemon_environment") && (
        <TextField form={form} path={["variable"]} label="Variable" placeholder="API_TOKEN" />
      )}
      {kind === "command" && (
        <ListField
          form={form}
          path={["command"]}
          label="Command"
          placeholder={"op\nread\nop://vault/item/token"}
        />
      )}
      {(kind === "file" || kind === "sops") && (
        <TextField form={form} path={["file"]} label="File" />
      )}
      {kind === "sops" && (
        <>
          <TextField form={form} path={["key"]} label="Key" placeholder="tokens.github" />
          <ValueField form={form} path={["age_key"]} label="Age key" />
        </>
      )}
      {kind === "oauth" && (
        <>
          <TextField form={form} path={["issuer"]} label="Issuer" placeholder="discovered" />
          <TextField form={form} path={["client_id"]} label="Client ID" />
          <TextField form={form} path={["resource"]} label="Resource" />
          <TagsField
            form={form}
            path={["scopes"]}
            label="Scopes"
            placeholder="read, openid…"
            suggestions={oauthScopes}
          />
        </>
      )}
      <BooleanField form={form} path={["override"]} label="Override" />
    </FieldGroup>
  );
}

const refKind = (ref: SourceRef) => (ref.default ? "default" : ref.tag ? "tag" : "branch");

/**
 * Where a Git source is pinned. Ref and revision offer the repository's branches and tags, asked
 * of it once its URL is filled in; a ref's commit is what a revision takes.
 */
function PinFields({ form }: { readonly form: DefinitionText }) {
  const git = readString(form.text, ["git"]);
  const refs = useDaemonQuery(
    queryKeys.sourceRefs(git),
    Daemon.use((daemon) => daemon.sourceRefs(git)),
    { enabled: git !== "", staleTime: 60_000, retry: false },
  );
  const listed = refs.data ?? [];
  return (
    <>
      <SuggestField
        form={form}
        path={["ref"]}
        label="Ref"
        placeholder="the default branch"
        loading={refs.isFetching}
        suggestions={listed.map((ref) => ({
          value: ref.name,
          detail: (
            <>
              <Badge variant="outline">{refKind(ref)}</Badge>
              <span className="font-mono">{ref.commit.slice(0, 7)}</span>
            </>
          ),
        }))}
      />
      <SuggestField
        form={form}
        path={["rev"]}
        label="Revision"
        loading={refs.isFetching}
        suggestions={[...new Map(listed.map((ref) => [ref.commit, ref])).values()].map((ref) => ({
          value: ref.commit,
          detail: (
            <>
              <Badge variant="outline">{refKind(ref)}</Badge>
              {ref.name}
            </>
          ),
        }))}
      />
      <HashField form={form} />
    </>
  );
}

/** How an HTTP server is authenticated: a kind of sign-in, or a secret by its ID. */
function AuthField({ form }: { readonly form: DefinitionText }) {
  const definitions = useDaemonQuery(
    queryKeys.definitions,
    Daemon.use((daemon) => daemon.definitions),
  );
  const secrets = (definitions.data ?? []).filter((d) => d.kind === "secrets");
  return (
    <SuggestField
      form={form}
      path={["auth"]}
      label="Auth"
      placeholder="OAuth when the server asks"
      description="oauth, keychain, none, or a secret ID; an inline secret is edited on the YAML tab."
      suggestions={[
        { value: "oauth", detail: "sign in when asked" },
        { value: "keychain", detail: "a token kept in the keychain" },
        { value: "none", detail: "no authentication" },
        ...secrets.map((secret) => ({ value: secret.id, detail: "secret" })),
      ]}
    />
  );
}

/** A button beside a directory field that picks one by browsing folders. */
function BrowseFolder({ form, path }: { readonly form: DefinitionText; readonly path: FieldPath }) {
  const [open, setOpen] = useState(false);
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const home = configuration.data?.home ?? "";
  const written = absoluteDirectory(readString(form.text, path), home);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <InputGroupButton size="icon-xs" aria-label="Browse folders">
            <FolderIcon />
          </InputGroupButton>
        }
      />
      <PopoverContent align="end" className="w-80 gap-0 p-0">
        <DirectoryBrowser
          key={String(open)}
          value={written.startsWith("/") ? written : ""}
          home={home}
          shortcuts={configuration.data?.scopeDirectories ?? []}
          onPick={(directory) => {
            setOpen(false);
            form.onChange(writeField(form.text, path, shortPath(directory, home)));
          }}
        />
      </PopoverContent>
    </Popover>
  );
}

/** The content hash, with a button computing it from the source at its pins. */
function HashField({ form }: { readonly form: DefinitionText }) {
  const compute = useDaemonMutation(
    (request: { git: string; ref: string; rev: string; subdir: string }) =>
      Daemon.use((daemon) => daemon.inspectSource(request)),
    [],
  );
  const read = (field: string) => readString(form.text, [field]);
  return (
    <TextField
      form={form}
      path={["hash"]}
      label="Hash"
      action={
        !form.readOnly && (
          <Tooltip>
            <TooltipTrigger
              render={
                <InputGroupButton
                  size="icon-xs"
                  aria-label="Compute the hash from the source"
                  disabled={read("git") === "" || compute.isPending}
                  onClick={() =>
                    compute.mutate(
                      {
                        git: read("git"),
                        ref: read("ref"),
                        rev: read("rev"),
                        subdir: read("subdir"),
                      },
                      {
                        onSuccess: (inspection) =>
                          form.onChange(writeField(form.text, ["hash"], inspection.hash)),
                        onError: (error) =>
                          toast.add({
                            type: "error",
                            title: "Could not compute the hash",
                            description: error.message,
                          }),
                      },
                    )
                  }
                >
                  {compute.isPending ? <Spinner /> : <HashIcon />}
                </InputGroupButton>
              }
            />
            <TooltipContent side="left">
              Compute from the source at its ref or revision
            </TooltipContent>
          </Tooltip>
        )
      }
    />
  );
}

/** A package's subdirectory, offering the source's directories once the list is opened. */
function SubdirectoryField({ form }: { readonly form: DefinitionText }) {
  const [wanted, setWanted] = useState(false);
  const git = readString(form.text, ["git"]);
  const ref = readString(form.text, ["ref"]);
  const rev = readString(form.text, ["rev"]);
  // Listing means checking the source out, so it waits until someone opens the list.
  const directories = useDaemonQuery(
    queryKeys.sourceDirectories(git, ref, rev),
    Daemon.use((daemon) =>
      daemon.inspectSource({ git, ref, rev, subdir: "" }).pipe(Effect.map((i) => i.directories)),
    ),
    { enabled: wanted && git !== "", staleTime: 300_000, retry: false },
  );
  return git === "" ? (
    <TextField form={form} path={["subdir"]} label="Subdirectory" />
  ) : (
    <SuggestField
      form={form}
      path={["subdir"]}
      label="Subdirectory"
      placeholder="the whole repository"
      loading={directories.isFetching}
      suggestions={(directories.data ?? []).map((value) => ({ value }))}
      onOpenChange={(open) => {
        if (open) {
          setWanted(true);
        }
      }}
    />
  );
}

function PackageForm({ form }: { readonly form: DefinitionText }) {
  const source = has(form, "path") ? "path" : "git";
  const switchSource = useSourceSwitch(form);
  return (
    <FieldGroup>
      <VariantField
        label="Source"
        value={source}
        readOnly={sourceForm(form).readOnly}
        locked={sourceNote(form)}
        options={[
          { value: "git", label: "Git", description: gitDescription },
          { value: "path", label: "Local directory", description: "A directory on this machine." },
        ]}
        onChange={(next) =>
          switchSource(
            source,
            next,
            next === "path" ? ["git", "ref", "rev", "hash"] : ["path"],
            next,
          )
        }
      />
      {source === "git" ? (
        <>
          <TextField
            form={sourceForm(form)}
            locked={sourceNote(form)}
            path={["git"]}
            label="Repository"
            placeholder="https://github.com/owner/repo"
          />
          <PinFields form={form} />
        </>
      ) : (
        <TextField
          form={sourceForm(form)}
          locked={sourceNote(form)}
          path={["path"]}
          label="Directory"
        />
      )}
      <SubdirectoryField form={form} />
      <BooleanField form={form} path={["override"]} label="Override" />
    </FieldGroup>
  );
}

const gitFields = ["git", "ref", "rev", "hash"];

const gitDescription = "A repository, at a branch, tag, or commit.";

/** A form's short settings side by side, above the fields that take room. */
function SettingsRow({ children }: { readonly children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-end gap-x-6 gap-y-4 *:w-auto [&>[data-orientation=horizontal]]:h-7">
      {children}
    </div>
  );
}

function SkillForm({ form }: { readonly form: DefinitionText }) {
  const source = has(form, "content") ? "content" : has(form, "git") ? "git" : "path";
  const switchSource = useSourceSwitch(form);
  return (
    <FieldGroup>
      <SettingsRow>
        <VariantField
          label="Source"
          value={source}
          readOnly={sourceForm(form).readOnly}
          locked={sourceNote(form)}
          options={[
            {
              value: "path",
              label: "Local directory",
              description: "A directory on this machine.",
            },
            { value: "git", label: "Git", description: gitDescription },
            { value: "content", label: "Inline", description: "Written in the configuration." },
          ]}
          onChange={(next) =>
            switchSource(
              source,
              next,
              next === "content"
                ? ["path", ...gitFields]
                : [
                    "content",
                    "name",
                    "description",
                    "frontmatter",
                    "files",
                    ...(next === "path" ? gitFields : []),
                  ],
              next,
            )
          }
        />
        <BooleanField form={form} path={["override"]} label="Override" />
      </SettingsRow>
      {source === "content" ? (
        <>
          <TextField form={form} path={["name"]} label="Name" placeholder="the ID's last segment" />
          <TextField form={form} path={["description"]} label="Description" />
          <MarkdownField form={form} field="content" label="SKILL.md body" />
        </>
      ) : (
        <>
          {source === "git" && (
            <>
              <TextField
                form={sourceForm(form)}
                locked={sourceNote(form)}
                path={["git"]}
                label="Repository"
                placeholder="https://github.com/owner/repo"
              />
              <PinFields form={form} />
            </>
          )}
          <TextField
            form={sourceForm(form)}
            locked={sourceNote(form)}
            path={["path"]}
            label={source === "git" ? "Path in the checkout" : "Directory"}
          />
        </>
      )}
    </FieldGroup>
  );
}

function InstructionsForm({ form }: { readonly form: DefinitionText }) {
  const source = has(form, "text") ? "text" : has(form, "git") ? "git" : "path";
  const switchSource = useSourceSwitch(form);
  return (
    <FieldGroup>
      <SettingsRow>
        <VariantField
          label="Source"
          value={source}
          readOnly={sourceForm(form).readOnly}
          locked={sourceNote(form)}
          options={[
            { value: "path", label: "Local file", description: "A file on this machine." },
            { value: "git", label: "Git", description: gitDescription },
            { value: "text", label: "Inline", description: "Written in the configuration." },
          ]}
          onChange={(next) =>
            switchSource(
              source,
              next,
              next === "text"
                ? ["path", ...gitFields]
                : ["text", ...(next === "path" ? gitFields : [])],
              next,
            )
          }
        />
        <NumberField form={form} path={["order"]} label="Order" placeholder="0" />
        <BooleanField form={form} path={["override"]} label="Override" />
      </SettingsRow>
      {source === "text" ? (
        <MarkdownField form={form} field="text" label="Text" />
      ) : (
        <>
          {source === "git" && (
            <>
              <TextField
                form={sourceForm(form)}
                locked={sourceNote(form)}
                path={["git"]}
                label="Repository"
                placeholder="https://github.com/owner/repo"
              />
              <PinFields form={form} />
            </>
          )}
          <TextField
            form={sourceForm(form)}
            locked={sourceNote(form)}
            path={["path"]}
            label={source === "git" ? "Path in the checkout" : "File"}
          />
        </>
      )}
    </FieldGroup>
  );
}

function MarkdownField({
  form,
  field,
  label,
}: {
  readonly form: DefinitionText;
  readonly field: string;
  readonly label: string;
}) {
  return (
    <div className="flex flex-col gap-2">
      <span className="font-medium text-xs">{label}</span>
      <CodeEditor
        language="markdown"
        className="min-h-40"
        readOnly={form.readOnly}
        value={readString(form.text, [field])}
        onChange={(value) => form.onChange(writeField(form.text, [field], value))}
      />
    </div>
  );
}
