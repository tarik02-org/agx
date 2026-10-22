import { PlusIcon, XIcon } from "lucide-react";
import { type ReactNode, useId, useState } from "react";

import { Button } from "~/components/ui/button.tsx";
import { Field, FieldDescription, FieldLabel } from "~/components/ui/field.tsx";
import { Input } from "~/components/ui/input.tsx";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select.tsx";
import { Switch } from "~/components/ui/switch.tsx";
import { Textarea } from "~/components/ui/textarea.tsx";
import { ToggleGroup, ToggleGroupItem } from "~/components/ui/toggle-group.tsx";
import {
  type FieldPath,
  isStructured,
  type MapEntry,
  readBoolean,
  readList,
  readMap,
  readString,
  readValue,
  type TaggedValue,
  type ValueTag,
  writeField,
  writeMap,
  writeValue,
} from "~/features/config/yamlDocument.ts";

/** A definition's YAML as a form edits it, with what the schema says about each field. */
export interface DefinitionText {
  readonly text: string;
  readonly onChange: (text: string) => void;
  readonly describe: (field: string) => string | undefined;
  readonly readOnly: boolean;
}

interface FieldProps {
  readonly form: DefinitionText;
  readonly path: FieldPath;
  readonly label: string;
  readonly placeholder?: string;
  /** Overrides the schema's description. */
  readonly description?: ReactNode;
}

function Described({ form, path, description }: Pick<FieldProps, "form" | "path" | "description">) {
  const text = description ?? form.describe(path[0] ?? "");
  return text === undefined ? null : <FieldDescription>{text}</FieldDescription>;
}

// A field written as a map or list where a plain value is expected stays for the YAML tab.
function StructuredNote({ label }: { readonly label: string }) {
  return (
    <Field>
      <FieldLabel>{label}</FieldLabel>
      <FieldDescription>Written as structured YAML; edit it on the YAML tab.</FieldDescription>
    </Field>
  );
}

export function TextField({ form, path, label, placeholder, description }: FieldProps) {
  const id = useId();
  if (isStructured(form.text, path)) {
    return <StructuredNote label={label} />;
  }
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        className="font-mono"
        spellCheck={false}
        readOnly={form.readOnly}
        placeholder={placeholder}
        value={readString(form.text, path)}
        onChange={(event) => form.onChange(writeField(form.text, path, event.target.value))}
      />
      <Described form={form} path={path} description={description} />
    </Field>
  );
}

export function NumberField({ form, path, label, placeholder, description }: FieldProps) {
  const id = useId();
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        type="number"
        className="w-32 font-mono"
        readOnly={form.readOnly}
        placeholder={placeholder}
        value={readString(form.text, path)}
        onChange={(event) => {
          const number = event.target.valueAsNumber;
          form.onChange(writeField(form.text, path, Number.isNaN(number) ? "" : number));
        }}
      />
      <Described form={form} path={path} description={description} />
    </Field>
  );
}

export function BooleanField({ form, path, label, description }: FieldProps) {
  const id = useId();
  return (
    <Field orientation="horizontal">
      <Switch
        id={id}
        disabled={form.readOnly}
        checked={readBoolean(form.text, path)}
        onCheckedChange={(checked) => form.onChange(writeField(form.text, path, checked))}
      />
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Described form={form} path={path} description={description} />
    </Field>
  );
}

/** A list of strings, one per line; written when the field loses focus. */
export function ListField({ form, path, label, placeholder, description }: FieldProps) {
  const id = useId();
  const items = readList(form.text, path);
  if (items.length === 0 && isStructured(form.text, path)) {
    return <StructuredNote label={label} />;
  }
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Textarea
        id={id}
        // Remounts with what the YAML now holds once an edit is written.
        key={items.join("\n")}
        className="min-h-16 font-mono"
        spellCheck={false}
        readOnly={form.readOnly}
        placeholder={placeholder ?? "one per line"}
        defaultValue={items.join("\n")}
        onBlur={(event) =>
          form.onChange(
            writeField(
              form.text,
              path,
              event.target.value.split("\n").filter((line) => line.trim() !== ""),
            ),
          )
        }
      />
      <Described form={form} path={path} description={description} />
    </Field>
  );
}

interface Option {
  readonly value: string;
  readonly label: string;
}

/** A choice among the schema's values; the empty choice removes the field. */
export function ChoiceField({
  form,
  path,
  label,
  description,
  options,
  empty,
}: FieldProps & { readonly options: ReadonlyArray<Option>; readonly empty?: string }) {
  const id = useId();
  const items = [...(empty === undefined ? [] : [{ value: "", label: empty }]), ...options];
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Select
        items={items}
        value={readString(form.text, path)}
        disabled={form.readOnly}
        onValueChange={(next) => form.onChange(writeField(form.text, path, next ?? ""))}
      >
        <SelectTrigger id={id} className="w-64">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {items.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <Described form={form} path={path} description={description} />
    </Field>
  );
}

/** Picks one of a few mutually exclusive shapes, such as where a skill comes from. */
export function VariantField<V extends string>({
  label,
  value,
  options,
  readOnly,
  onChange,
}: {
  readonly label: string;
  readonly value: V;
  readonly options: ReadonlyArray<{ readonly value: V; readonly label: string }>;
  readonly readOnly: boolean;
  readonly onChange: (value: V) => void;
}) {
  return (
    <Field>
      <FieldLabel>{label}</FieldLabel>
      <ToggleGroup
        variant="outline"
        size="sm"
        value={[value]}
        disabled={readOnly}
        onValueChange={(next) => {
          const picked = options.find((option) => next.includes(option.value));
          if (picked !== undefined) {
            onChange(picked.value);
          }
        }}
      >
        {options.map((option) => (
          <ToggleGroupItem key={option.value} value={option.value}>
            {option.label}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </Field>
  );
}

const tags: ReadonlyArray<{ readonly value: ValueTag; readonly label: string }> = [
  { value: "", label: "text" },
  { value: "!env", label: "!env" },
  { value: "!file", label: "!file" },
  { value: "!secret", label: "!secret" },
];

const tagPlaceholder: Record<ValueTag, string> = {
  "": "value",
  "!env": "VARIABLE",
  "!file": "~/path/to/file",
  "!secret": "secret-id",
};

function ValueInput({
  value,
  readOnly,
  onChange,
  id,
}: {
  readonly value: TaggedValue;
  readonly readOnly: boolean;
  readonly onChange: (value: TaggedValue) => void;
  readonly id?: string;
}) {
  return (
    <div className="flex min-w-0 flex-1 gap-1">
      <Select
        items={tags}
        value={value.tag}
        disabled={readOnly}
        onValueChange={(tag) => onChange({ tag: tag ?? "", value: value.value })}
      >
        <SelectTrigger className="w-24 font-mono">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {tags.map((tag) => (
              <SelectItem key={tag.value} value={tag.value} className="font-mono">
                {tag.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <Input
        id={id}
        className="min-w-0 flex-1 font-mono"
        spellCheck={false}
        readOnly={readOnly}
        placeholder={tagPlaceholder[value.tag]}
        value={value.value}
        onChange={(event) => onChange({ tag: value.tag, value: event.target.value })}
      />
    </div>
  );
}

/** A literal, or a variable, file, or secret resolved at launch. */
export function ValueField({ form, path, label, description }: FieldProps) {
  const id = useId();
  if (isStructured(form.text, path)) {
    return <StructuredNote label={label} />;
  }
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <ValueInput
        id={id}
        value={readValue(form.text, path)}
        readOnly={form.readOnly}
        onChange={(value) => form.onChange(writeValue(form.text, path, value))}
      />
      <Described form={form} path={path} description={description} />
    </Field>
  );
}

interface Row extends MapEntry {
  readonly row: string;
}

/** Names mapped to values, such as environment variables or request headers. */
export function MapField({ form, path, label, placeholder, description }: FieldProps) {
  // Rows keep an identity of their own while their names are edited.
  const [rows, setRows] = useState<ReadonlyArray<Row>>(() =>
    readMap(form.text, path).map(({ key, value }) => ({ key, value, row: crypto.randomUUID() })),
  );
  const write = (next: ReadonlyArray<Row>) => {
    setRows(next);
    form.onChange(
      writeMap(
        form.text,
        path,
        next.filter((row) => row.key !== ""),
      ),
    );
  };
  return (
    <Field>
      <FieldLabel>{label}</FieldLabel>
      <div className="flex flex-col gap-1">
        {rows.map((row) => (
          <div key={row.row} className="flex items-center gap-1">
            <Input
              className="w-48 font-mono"
              spellCheck={false}
              readOnly={form.readOnly}
              placeholder={placeholder ?? "NAME"}
              value={row.key}
              onChange={(event) =>
                write(rows.map((r) => (r.row === row.row ? { ...r, key: event.target.value } : r)))
              }
            />
            <ValueInput
              value={row.value}
              readOnly={form.readOnly}
              onChange={(value) =>
                write(rows.map((r) => (r.row === row.row ? { ...r, value } : r)))
              }
            />
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`Remove ${row.key}`}
              disabled={form.readOnly}
              onClick={() => write(rows.filter((r) => r.row !== row.row))}
            >
              <XIcon />
            </Button>
          </div>
        ))}
        <div>
          <Button
            variant="outline"
            size="sm"
            disabled={form.readOnly}
            onClick={() =>
              setRows([
                ...rows,
                { row: crypto.randomUUID(), key: "", value: { tag: "", value: "" } },
              ])
            }
          >
            <PlusIcon data-icon="inline-start" />
            Add
          </Button>
        </div>
      </div>
      <Described form={form} path={path} description={description} />
    </Field>
  );
}
