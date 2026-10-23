import { useId } from "react";

import { Field, FieldGroup, FieldSeparator } from "~/components/ui/field.tsx";
import { Input } from "~/components/ui/input.tsx";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select.tsx";
import { CodeEditor } from "~/features/config/CodeEditor.tsx";
import {
  BooleanField,
  ChoiceField,
  type DefinitionText,
  HintedLabel,
  ListField,
  TagsField,
  MapField,
  NumberField,
  TextField,
  ValueField,
  VariantField,
} from "~/features/config/fields.tsx";
import {
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

const has = (form: DefinitionText, field: string) =>
  readString(form.text, [field]) !== "" || isStructured(form.text, [field]);

// Where a configured definition comes from is fixed: a new source makes a different thing, so
// it is removed and added again rather than edited. Its pins stay editable.
const sourceForm = (form: DefinitionText): DefinitionText =>
  form.existing ? { ...form, readOnly: true } : form;

const sourceNote = (form: DefinitionText) =>
  form.existing
    ? "Fixed once configured: remove it and add it again to take it from elsewhere."
    : undefined;

// Switching where a definition comes from drops the fields only the previous source had.
const switchSource = (form: DefinitionText, drop: ReadonlyArray<string>) =>
  form.onChange(removeFields(form.text, drop));

function McpForm({ form }: { readonly form: DefinitionText }) {
  const transport = has(form, "url") ? "http" : "stdio";
  return (
    <FieldGroup>
      <TextField form={form} path={["description"]} label="Description" />
      <VariantField
        label="Transport"
        value={transport}
        readOnly={form.readOnly}
        options={[
          { value: "stdio", label: "stdio command" },
          { value: "http", label: "HTTP" },
        ]}
        onChange={(next) =>
          switchSource(
            form,
            next === "http"
              ? ["command", "cwd", "environment", "transport", "share"]
              : ["url", "headers", "auth", "transport"],
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
          />
          <MapField form={form} path={["environment"]} label="Environment" />
          <ShareField form={form} />
        </>
      ) : (
        <>
          <TextField form={form} path={["url"]} label="URL" placeholder="https://example.com/mcp" />
          <MapField form={form} path={["headers"]} label="Headers" placeholder="Header-Name" />
          <TextField
            form={form}
            path={["auth"]}
            label="Auth"
            placeholder="OAuth when the server asks"
            description="oauth, keychain, none, or a secret ID; an inline secret is edited on the YAML tab."
          />
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

const shares = [
  { value: "none", label: "none: a process per session" },
  { value: "directory", label: "directory: one per launch directory" },
  { value: "repo", label: "repo: one per repository" },
  { value: "global", label: "global: one everywhere" },
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
        <Select
          items={shares}
          value={type}
          disabled={form.readOnly}
          onValueChange={(next) => write(next ?? "none", idle)}
        >
          <SelectTrigger id={typeId} className="w-72">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {shares.map((share) => (
                <SelectItem key={share.value} value={share.value}>
                  {share.label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
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
      {kind === "keychain" && (
        <p className="text-muted-foreground text-xs">
          The value lives in the OS keychain; set it from the Runtime page once this is applied.
        </p>
      )}
      <BooleanField form={form} path={["override"]} label="Override" />
    </FieldGroup>
  );
}

function PinFields({ form }: { readonly form: DefinitionText }) {
  return (
    <>
      <TextField form={form} path={["ref"]} label="Ref" placeholder="the default branch" />
      <TextField form={form} path={["rev"]} label="Revision" />
      <TextField form={form} path={["hash"]} label="Hash" />
    </>
  );
}

function PackageForm({ form }: { readonly form: DefinitionText }) {
  const source = has(form, "path") ? "path" : "git";
  return (
    <FieldGroup>
      <VariantField
        label="Source"
        value={source}
        readOnly={sourceForm(form).readOnly}
        locked={sourceNote(form)}
        options={[
          { value: "git", label: "Git" },
          { value: "path", label: "local directory" },
        ]}
        onChange={(next) =>
          switchSource(form, next === "path" ? ["git", "ref", "rev", "hash"] : ["path"])
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
      <TextField form={form} path={["subdir"]} label="Subdirectory" />
      <BooleanField form={form} path={["override"]} label="Override" />
    </FieldGroup>
  );
}

const gitFields = ["git", "ref", "rev", "hash"];

function SkillForm({ form }: { readonly form: DefinitionText }) {
  const source = has(form, "content") ? "content" : has(form, "git") ? "git" : "path";
  return (
    <FieldGroup>
      <VariantField
        label="Source"
        value={source}
        readOnly={sourceForm(form).readOnly}
        locked={sourceNote(form)}
        options={[
          { value: "path", label: "local directory" },
          { value: "git", label: "Git" },
          { value: "content", label: "written here" },
        ]}
        onChange={(next) =>
          switchSource(
            form,
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
          )
        }
      />
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
      <BooleanField form={form} path={["override"]} label="Override" />
    </FieldGroup>
  );
}

function InstructionsForm({ form }: { readonly form: DefinitionText }) {
  const source = has(form, "text") ? "text" : has(form, "git") ? "git" : "path";
  return (
    <FieldGroup>
      <VariantField
        label="Source"
        value={source}
        readOnly={sourceForm(form).readOnly}
        locked={sourceNote(form)}
        options={[
          { value: "path", label: "local file" },
          { value: "git", label: "Git" },
          { value: "text", label: "written here" },
        ]}
        onChange={(next) =>
          switchSource(
            form,
            next === "text"
              ? ["path", ...gitFields]
              : ["text", ...(next === "path" ? gitFields : [])],
          )
        }
      />
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
      <NumberField form={form} path={["order"]} label="Order" placeholder="0" />
      <BooleanField form={form} path={["override"]} label="Override" />
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
