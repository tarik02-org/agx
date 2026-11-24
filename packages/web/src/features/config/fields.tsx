import { InfoIcon, LockIcon, XIcon } from "lucide-react";
import { type ReactNode, useId, useState } from "react";

import { Button } from "~/components/ui/button.tsx";
import {
  Combobox,
  ComboboxChip,
  ComboboxChips,
  ComboboxChipsInput,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxValue,
  useComboboxAnchor,
} from "~/components/ui/combobox.tsx";
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
import { InputGroup, InputGroupAddon, InputGroupInput } from "~/components/ui/input-group.tsx";
import { Spinner } from "~/components/ui/spinner.tsx";
import { shortPath } from "~/lib/path.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { Daemon } from "~/daemon/Daemon.ts";
import { Tooltip, TooltipContent, TooltipTrigger } from "~/components/ui/tooltip.tsx";
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
  /** The definition is already configured, so where it comes from is fixed. */
  readonly existing: boolean;
}

interface FieldProps {
  readonly form: DefinitionText;
  readonly path: FieldPath;
  readonly label: string;
  readonly placeholder?: string;
  /** Overrides the schema's description. */
  readonly description?: ReactNode;
  /** Why the field cannot change, shown behind a lock beside the label. */
  readonly locked?: string | undefined;
}

// Words that say nothing about a field by themselves.
const fillers = new Set(["the", "and", "for", "with", "its", "this", "that", "from", "are", "not"]);

/**
 * What a field means, when that says more than its label: the description, unless it mostly
 * repeats the label, as "Git repository URL." does for Repository.
 */
const meaning = (
  form: DefinitionText,
  path: FieldPath,
  label: string,
  description: ReactNode,
): ReactNode => {
  if (description !== undefined) {
    return description;
  }
  const text = form.describe(path[0] ?? "");
  if (text === undefined) {
    return undefined;
  }
  const said = new Set(label.toLowerCase().split(/[^a-z0-9]+/));
  const news = text
    .toLowerCase()
    .split(/[^a-z0-9]+/)
    .filter((word) => word.length > 2 && !fillers.has(word) && !said.has(word));
  return news.length >= 3 ? text : undefined;
};

const infoIcon = <InfoIcon />;

/** A small icon whose tooltip tells more, beside a field's label. */
export function LabelHint({
  icon = infoIcon,
  label,
  children,
}: {
  readonly icon?: ReactNode;
  readonly label: string;
  readonly children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            role="img"
            tabIndex={0}
            aria-label={label}
            className="inline-flex text-muted-foreground outline-none hover:text-foreground focus-visible:text-foreground [&_svg]:size-3"
          >
            {icon}
          </span>
        }
      />
      <TooltipContent className="max-w-xs">{children}</TooltipContent>
    </Tooltip>
  );
}

/** A field's label, with what the field means behind an info icon when that is not obvious. */
export function HintedLabel({
  htmlFor,
  hint,
  locked,
  children,
}: {
  readonly htmlFor?: string;
  readonly hint?: ReactNode;
  readonly locked?: ReactNode;
  readonly children: ReactNode;
}) {
  return (
    <FieldLabel htmlFor={htmlFor} className="gap-1.5">
      {children}
      {hint !== undefined && hint !== null && (
        <LabelHint label="About this field">{hint}</LabelHint>
      )}
      {locked !== undefined && locked !== null && (
        <LabelHint icon={<LockIcon />} label="Fixed">
          {locked}
        </LabelHint>
      )}
    </FieldLabel>
  );
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

export function TextField({
  form,
  path,
  label,
  placeholder,
  description,
  locked,
  action,
}: FieldProps & {
  /** A button inside the field, such as one filling it in. */
  readonly action?: ReactNode;
}) {
  const id = useId();
  if (isStructured(form.text, path)) {
    return <StructuredNote label={label} />;
  }
  return (
    <Field>
      <HintedLabel htmlFor={id} hint={meaning(form, path, label, description)} locked={locked}>
        {label}
      </HintedLabel>
      {action === undefined ? (
        <Input
          id={id}
          className="font-mono"
          spellCheck={false}
          readOnly={form.readOnly}
          placeholder={placeholder}
          value={readString(form.text, path)}
          onChange={(event) => form.onChange(writeField(form.text, path, event.target.value))}
        />
      ) : (
        <InputGroup>
          <InputGroupInput
            id={id}
            className="font-mono"
            spellCheck={false}
            readOnly={form.readOnly}
            placeholder={placeholder}
            value={readString(form.text, path)}
            onChange={(event) => form.onChange(writeField(form.text, path, event.target.value))}
          />
          <InputGroupAddon align="inline-end">{action}</InputGroupAddon>
        </InputGroup>
      )}
    </Field>
  );
}

export function NumberField({ form, path, label, placeholder, description }: FieldProps) {
  const id = useId();
  return (
    <Field>
      <HintedLabel htmlFor={id} hint={meaning(form, path, label, description)}>
        {label}
      </HintedLabel>
      <Input
        id={id}
        type="number"
        className="w-32! font-mono"
        readOnly={form.readOnly}
        placeholder={placeholder}
        value={readString(form.text, path)}
        onChange={(event) => {
          const number = event.target.valueAsNumber;
          form.onChange(writeField(form.text, path, Number.isNaN(number) ? "" : number));
        }}
      />
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
      <HintedLabel htmlFor={id} hint={meaning(form, path, label, description)}>
        {label}
      </HintedLabel>
    </Field>
  );
}

/** A row of a list or map editor, with an identity of its own while it is edited. */
const newRowId = (): string => crypto.randomUUID();

interface ListRow {
  readonly row: string;
  readonly value: string;
}

// A list's rows, with the empty one for adding at the end.
const listRows = (values: ReadonlyArray<string>): ReadonlyArray<ListRow> => [
  ...values.map((value) => ({ row: newRowId(), value })),
  { row: newRowId(), value: "" },
];

// What a list's rows write: those not empty.
const listValues = (rows: ReadonlyArray<ListRow>) =>
  rows.map((row) => row.value).filter((value) => value !== "");

/** Whether focus left a row for somewhere outside it, not for another of its inputs. */
const leftRow = (event: React.FocusEvent<HTMLElement>) =>
  !event.currentTarget.contains(event.relatedTarget);

/**
 * A list of strings, such as a command and its arguments, one input each. The last row is always
 * empty, for adding one; a row emptied is dropped once focus leaves it.
 */
export function ListField({ form, path, label, placeholder, description }: FieldProps) {
  const id = useId();
  const items = readList(form.text, path);
  const [rows, setRows] = useState(() => listRows(items));
  // The YAML changed elsewhere, such as by Revert or on the YAML tab: show what it holds.
  if (JSON.stringify(listValues(rows)) !== JSON.stringify(items)) {
    setRows(listRows(items));
  }
  if (items.length === 0 && isStructured(form.text, path)) {
    return <StructuredNote label={label} />;
  }
  const examples = (placeholder ?? "").split("\n");
  const write = (next: ReadonlyArray<{ readonly row: string; readonly value: string }>) => {
    const last = next.at(-1);
    setRows(
      last === undefined || last.value !== ""
        ? [...next, { row: newRowId(), value: "" }]
        : [...next],
    );
    form.onChange(writeField(form.text, path, listValues(next)));
  };
  return (
    <Field>
      <HintedLabel htmlFor={id} hint={meaning(form, path, label, description)}>
        {label}
      </HintedLabel>
      <div className="flex flex-col gap-1">
        {rows.map((row, index) => (
          <div
            key={row.row}
            className="flex items-center gap-1"
            onBlur={(event) => {
              if (leftRow(event) && row.value === "" && index < rows.length - 1) {
                setRows(rows.filter((r) => r.row !== row.row));
              }
            }}
          >
            <Input
              id={index === 0 ? id : undefined}
              aria-label={index === 0 ? undefined : `${label} ${index + 1}`}
              className="font-mono"
              spellCheck={false}
              readOnly={form.readOnly}
              placeholder={items.length === 0 ? (examples[index] ?? "") : ""}
              value={row.value}
              onChange={(event) =>
                write(
                  rows.map((r) => (r.row === row.row ? { ...r, value: event.target.value } : r)),
                )
              }
            />
            {index < rows.length - 1 && (
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={`Remove ${row.value}`}
                disabled={form.readOnly}
                onClick={() => write(rows.filter((r) => r.row !== row.row))}
              >
                <XIcon />
              </Button>
            )}
          </div>
        ))}
      </div>
    </Field>
  );
}

export interface Option<V extends string = string> {
  readonly value: V;
  readonly label: string;
  /** A line under the label in the list, saying what the choice means. */
  readonly description?: string;
}

/** A value offered for a SuggestField: what it writes, and what the list shows beside it. */
export interface Suggestion {
  readonly value: string;
  readonly detail?: ReactNode;
}

/**
 * A text field that offers values, such as a repository's branches, while still taking any
 * typed one.
 */
export function SuggestField({
  form,
  path,
  label,
  placeholder,
  description,
  suggestions,
  loading = false,
  onOpenChange,
}: FieldProps & {
  readonly suggestions: ReadonlyArray<Suggestion>;
  readonly loading?: boolean;
  /** Told when the list opens, for loading suggestions only once they are wanted. */
  readonly onOpenChange?: (open: boolean) => void;
}) {
  const id = useId();
  const value = readString(form.text, path);
  const details = new Map(suggestions.map((item) => [item.value, item.detail]));
  if (isStructured(form.text, path)) {
    return <StructuredNote label={label} />;
  }
  return (
    <Field>
      <HintedLabel htmlFor={id} hint={meaning(form, path, label, description)}>
        {label}
      </HintedLabel>
      <Combobox
        items={suggestions.map((item) => item.value)}
        inputValue={value}
        readOnly={form.readOnly}
        onInputValueChange={(next) => form.onChange(writeField(form.text, path, next))}
        {...(onOpenChange === undefined ? {} : { onOpenChange })}
      >
        <ComboboxInput
          id={id}
          className="font-mono"
          spellCheck={false}
          placeholder={placeholder}
          showTrigger={suggestions.length > 0 || onOpenChange !== undefined}
        >
          {loading && (
            <InputGroupAddon align="inline-end">
              <Spinner className="size-3" />
            </InputGroupAddon>
          )}
        </ComboboxInput>
        <ComboboxContent>
          <ComboboxEmpty>
            {loading ? "Loading…" : "Nothing matches; the typed value is kept."}
          </ComboboxEmpty>
          <ComboboxList>
            {(item: string) => (
              <ComboboxItem key={item} value={item}>
                <span className="font-mono">{item}</span>
                {details.get(item) !== undefined && (
                  <span className="ml-auto flex items-center gap-1.5 text-muted-foreground text-xs">
                    {details.get(item)}
                  </span>
                )}
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
    </Field>
  );
}

/**
 * A set of words, such as OAuth scopes, as chips: pick from suggestions or type one and press
 * Enter to add it.
 */
export function TagsField({
  form,
  path,
  label,
  placeholder,
  description,
  suggestions = noSuggestions,
}: FieldProps & { readonly suggestions?: ReadonlyArray<string> }) {
  const id = useId();
  const anchor = useComboboxAnchor();
  const values = readList(form.text, path);
  const [query, setQuery] = useState("");
  const typed = query.trim();
  const items = [...new Set([...(typed === "" ? [] : [typed]), ...values, ...suggestions])];
  if (values.length === 0 && isStructured(form.text, path)) {
    return <StructuredNote label={label} />;
  }
  return (
    <Field>
      <HintedLabel htmlFor={id} hint={meaning(form, path, label, description)}>
        {label}
      </HintedLabel>
      <Combobox
        multiple
        autoHighlight
        items={items}
        value={values}
        inputValue={query}
        disabled={form.readOnly}
        onInputValueChange={setQuery}
        onValueChange={(next: ReadonlyArray<string>) => {
          setQuery("");
          form.onChange(writeField(form.text, path, [...next]));
        }}
      >
        <ComboboxChips ref={anchor}>
          <ComboboxValue>
            {(picked: ReadonlyArray<string>) =>
              picked.map((value) => (
                <ComboboxChip key={value} className="font-mono">
                  {value}
                </ComboboxChip>
              ))
            }
          </ComboboxValue>
          <ComboboxChipsInput
            id={id}
            className="font-mono"
            placeholder={values.length === 0 ? (placeholder ?? "Type to add…") : undefined}
          />
        </ComboboxChips>
        <ComboboxContent anchor={anchor}>
          <ComboboxEmpty>Type to add one.</ComboboxEmpty>
          <ComboboxList>
            {(item: string) => (
              <ComboboxItem key={item} value={item} className="font-mono">
                {item === typed && !values.includes(item) && !suggestions.includes(item)
                  ? `Add “${item}”`
                  : item}
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
    </Field>
  );
}

const noSuggestions: ReadonlyArray<string> = [];

/**
 * A select whose options may carry a line saying what they mean; the list may be wider than the
 * trigger, so those lines wrap instead of running out of it.
 */
export function OptionSelect<V extends string>({
  id,
  value,
  options,
  disabled,
  onChange,
}: {
  readonly id?: string;
  readonly value: V;
  readonly options: ReadonlyArray<Option<V>>;
  readonly disabled: boolean;
  readonly onChange: (value: V) => void;
}) {
  return (
    <Select
      items={options.map(({ value: item, label }) => ({ value: item, label }))}
      value={value}
      disabled={disabled}
      onValueChange={(next) => {
        const picked = options.find((option) => option.value === next);
        if (picked !== undefined) {
          onChange(picked.value);
        }
      }}
    >
      <SelectTrigger id={id} className="w-64!">
        <SelectValue />
      </SelectTrigger>
      <SelectContent
        alignItemWithTrigger={false}
        align="start"
        className="w-auto min-w-(--anchor-width) max-w-sm"
      >
        <SelectGroup>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value} className="whitespace-normal">
              {option.description === undefined ? (
                option.label
              ) : (
                <span className="flex flex-col gap-0.5 py-0.5">
                  <span>{option.label}</span>
                  <span className="text-muted-foreground text-xs">{option.description}</span>
                </span>
              )}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
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
  return (
    <Field>
      <HintedLabel htmlFor={id} hint={meaning(form, path, label, description)}>
        {label}
      </HintedLabel>
      <OptionSelect
        id={id}
        value={readString(form.text, path)}
        options={[...(empty === undefined ? [] : [{ value: "", label: empty }]), ...options]}
        disabled={form.readOnly}
        onChange={(next) => form.onChange(writeField(form.text, path, next))}
      />
    </Field>
  );
}

/** Picks one of a few mutually exclusive shapes, such as where a skill comes from. */
export function VariantField<V extends string>({
  label,
  value,
  options,
  readOnly,
  locked,
  onChange,
}: {
  readonly label: string;
  /** Why the choice cannot change, shown behind a lock beside the label. */
  readonly locked?: string | undefined;
  readonly value: V;
  readonly options: ReadonlyArray<Option<V>>;
  readonly readOnly: boolean;
  readonly onChange: (value: V) => void;
}) {
  const id = useId();
  return (
    <Field className="w-auto">
      <HintedLabel htmlFor={id} locked={locked}>
        {label}
      </HintedLabel>
      <OptionSelect
        id={id}
        value={value}
        options={options}
        disabled={readOnly}
        onChange={onChange}
      />
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

/** A text input offering values as it is typed, while taking any typed one. */
function SuggestInput({
  id,
  value,
  placeholder,
  suggestions,
  readOnly,
  onChange,
}: {
  readonly id?: string | undefined;
  readonly value: string;
  readonly placeholder: string;
  readonly suggestions: ReadonlyArray<string>;
  readonly readOnly: boolean;
  readonly onChange: (value: string) => void;
}) {
  return (
    <Combobox
      items={suggestions}
      inputValue={value}
      readOnly={readOnly}
      onInputValueChange={onChange}
    >
      <ComboboxInput
        id={id}
        className="min-w-0 flex-1 font-mono"
        spellCheck={false}
        placeholder={placeholder}
        showTrigger={suggestions.length > 0}
      />
      <ComboboxContent>
        <ComboboxList>
          {(item: string) => (
            <ComboboxItem key={item} value={item} className="font-mono">
              {item}
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  );
}

/** The secrets configured, for a !secret value to name. */
function useSecretIds(enabled: boolean): ReadonlyArray<string> {
  const definitions = useDaemonQuery(
    queryKeys.definitions,
    Daemon.use((daemon) => daemon.definitions),
    { enabled },
  );
  return (definitions.data ?? []).filter((d) => d.kind === "secrets").map((d) => d.id);
}

/**
 * Paths completing what is typed, for a !file value: folders with a slash, to go on into, and
 * files; under home as ~/….
 */
function usePathCompletions(value: string, enabled: boolean): ReadonlyArray<string> {
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const home = configuration.data?.home ?? "";
  const prefix = value === "" ? "~/" : value;
  const completions = useDaemonQuery(
    queryKeys.completePath(prefix),
    Daemon.use((daemon) => daemon.completePath(prefix)),
    { enabled: enabled && (prefix.startsWith("/") || prefix.startsWith("~/")) },
  );
  return [
    ...(completions.data?.directories ?? []).map((path) => `${shortPath(path, home)}/`),
    ...(completions.data?.files ?? []).map((path) => shortPath(path, home)),
  ];
}

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
  const secrets = useSecretIds(value.tag === "!secret");
  const paths = usePathCompletions(value.value, value.tag === "!file");
  const suggestions = value.tag === "!secret" ? secrets : value.tag === "!file" ? paths : [];
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
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {tags.map((tag) => (
              <SelectItem key={tag.value} value={tag.value} className="font-mono">
                {tag.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      {value.tag === "!secret" || value.tag === "!file" ? (
        <SuggestInput
          id={id}
          value={value.value}
          placeholder={tagPlaceholder[value.tag]}
          suggestions={suggestions}
          readOnly={readOnly}
          onChange={(next) => onChange({ tag: value.tag, value: next })}
        />
      ) : (
        <Input
          id={id}
          className="min-w-0 flex-1 font-mono"
          spellCheck={false}
          readOnly={readOnly}
          placeholder={tagPlaceholder[value.tag]}
          value={value.value}
          onChange={(event) => onChange({ tag: value.tag, value: event.target.value })}
        />
      )}
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
      <HintedLabel htmlFor={id} hint={meaning(form, path, label, description)}>
        {label}
      </HintedLabel>
      <ValueInput
        id={id}
        value={readValue(form.text, path)}
        readOnly={form.readOnly}
        onChange={(value) => form.onChange(writeValue(form.text, path, value))}
      />
    </Field>
  );
}

interface Row extends MapEntry {
  readonly row: string;
}

const emptyRow = (): Row => ({ row: newRowId(), key: "", value: { tag: "", value: "" } });

const isEmptyRow = (row: Row) => row.key === "" && row.value.value === "";

// A map's rows, with the empty one for adding at the end.
const mapRows = (values: ReadonlyArray<MapEntry>): ReadonlyArray<Row> => [
  ...values.map(({ key, value }) => ({ key, value, row: newRowId() })),
  emptyRow(),
];

// A map's entries as plain tuples, to compare what rows hold with what the YAML holds.
const entryKey = (entries: ReadonlyArray<MapEntry>) =>
  JSON.stringify(entries.map(({ key, value }) => [key, value.tag, value.value]));

// What a map's rows write: those with a name.
const mapEntries = (rows: ReadonlyArray<Row>): ReadonlyArray<MapEntry> =>
  rows.filter((row) => row.key !== "").map(({ key, value }) => ({ key, value }));

/**
 * Names mapped to values, such as environment variables or request headers. The last row is
 * always empty, for adding one; a row emptied is dropped once focus leaves it.
 */
export function MapField({ form, path, label, placeholder, description }: FieldProps) {
  const entries = readMap(form.text, path);
  const [rows, setRows] = useState(() => mapRows(entries));
  // The YAML changed elsewhere, such as by Revert or on the YAML tab: show what it holds.
  if (entryKey(mapEntries(rows)) !== entryKey(entries)) {
    setRows(mapRows(entries));
  }
  const write = (next: ReadonlyArray<Row>) => {
    const last = next.at(-1);
    setRows(last === undefined || !isEmptyRow(last) ? [...next, emptyRow()] : next);
    form.onChange(writeMap(form.text, path, mapEntries(next)));
  };
  return (
    <Field>
      <HintedLabel hint={meaning(form, path, label, description)}>{label}</HintedLabel>
      <div className="flex flex-col gap-1">
        {rows.map((row, index) => (
          <div
            key={row.row}
            className="flex items-center gap-1"
            onBlur={(event) => {
              if (leftRow(event) && isEmptyRow(row) && index < rows.length - 1) {
                setRows(rows.filter((r) => r.row !== row.row));
              }
            }}
          >
            <Input
              aria-label={`${label} name`}
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
            {index < rows.length - 1 ? (
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={`Remove ${row.key}`}
                disabled={form.readOnly}
                onClick={() => write(rows.filter((r) => r.row !== row.row))}
              >
                <XIcon />
              </Button>
            ) : (
              <span className="size-6 shrink-0" />
            )}
          </div>
        ))}
      </div>
    </Field>
  );
}
