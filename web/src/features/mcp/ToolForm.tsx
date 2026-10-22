import { PlayIcon } from "lucide-react";
import { useState } from "react";

import { Button } from "~/components/ui/button.tsx";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "~/components/ui/field.tsx";
import { Input } from "~/components/ui/input.tsx";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select.tsx";
import { Spinner } from "~/components/ui/spinner.tsx";
import { Switch } from "~/components/ui/switch.tsx";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "~/components/ui/tabs.tsx";
import { Textarea } from "~/components/ui/textarea.tsx";
import { type JsonSchemaNode, schemaType } from "~/lib/jsonSchema.ts";

/** A field's input as typed; converted to the argument's JSON type when the form submits. */
type FieldValue =
  | { readonly _tag: "Text"; readonly text: string }
  | { readonly _tag: "Flag"; readonly on: boolean };

interface ToolFormProps {
  readonly schema: JsonSchemaNode;
  readonly pending: boolean;
  /** The arguments as a JSON object text. */
  readonly onCall: (argumentsJson: string) => void;
}

const initialValue = (schema: JsonSchemaNode): FieldValue => {
  if (schemaType(schema) === "boolean") {
    return { _tag: "Flag", on: schema.default === true };
  }
  if (schema.default === undefined) {
    return { _tag: "Text", text: "" };
  }
  return {
    _tag: "Text",
    text: typeof schema.default === "string" ? schema.default : JSON.stringify(schema.default),
  };
};

// structured fields take JSON; others take the text a person would type.
const isStructured = (schema: JsonSchemaNode): boolean => {
  const type = schemaType(schema);
  return (
    type === "object" ||
    (type === "array" && schemaType(schema.items ?? {}) !== "string") ||
    type === undefined
  );
};

/** Builds the arguments object, or says which field does not parse. */
const toArguments = (
  properties: ReadonlyArray<readonly [string, JsonSchemaNode]>,
  values: Record<string, FieldValue>,
):
  | { readonly _tag: "Ok"; readonly json: string }
  | { readonly _tag: "Invalid"; readonly field: string } => {
  const out: Record<string, unknown> = {};
  for (const [name, schema] of properties) {
    const value = values[name] ?? initialValue(schema);
    if (value._tag === "Flag") {
      out[name] = value.on;
      continue;
    }
    if (value.text === "") {
      continue;
    }
    const type = schemaType(schema);
    if (schema.enum !== undefined) {
      out[name] = schema.enum.find((option) => String(option) === value.text) ?? value.text;
    } else if (type === "number" || type === "integer") {
      const number = Number(value.text);
      if (Number.isNaN(number)) {
        return { _tag: "Invalid", field: name };
      }
      out[name] = number;
    } else if (type === "array" && !isStructured(schema)) {
      out[name] = value.text.split("\n").filter((line) => line !== "");
    } else if (isStructured(schema)) {
      try {
        out[name] = JSON.parse(value.text);
      } catch {
        return { _tag: "Invalid", field: name };
      }
    } else {
      out[name] = value.text;
    }
  }
  return { _tag: "Ok", json: JSON.stringify(out) };
};

/** A form generated from a tool's input schema, with raw JSON for what it cannot express. */
export function ToolForm({ schema, pending, onCall }: ToolFormProps) {
  const properties = Object.entries(schema.properties ?? {});
  const required = new Set(schema.required ?? []);
  const [values, setValues] = useState<Record<string, FieldValue>>({});
  const [raw, setRaw] = useState("{}");
  const [mode, setMode] = useState<"form" | "json">(properties.length === 0 ? "json" : "form");
  const [invalid, setInvalid] = useState<string | null>(null);
  const set = (name: string, value: FieldValue) =>
    setValues((current) => ({ ...current, [name]: value }));

  const submit = () => {
    if (mode === "json") {
      try {
        JSON.parse(raw);
      } catch {
        setInvalid("arguments");
        return;
      }
      setInvalid(null);
      onCall(raw);
      return;
    }
    const result = toArguments(properties, values);
    if (result._tag === "Invalid") {
      setInvalid(result.field);
      return;
    }
    setInvalid(null);
    onCall(result.json);
  };

  return (
    <form
      className="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <Tabs
        value={mode}
        onValueChange={(next) => {
          if (next === "json" && mode === "form") {
            const result = toArguments(properties, values);
            if (result._tag === "Ok") {
              setRaw(JSON.stringify(JSON.parse(result.json), null, 2));
            }
          }
          setMode(next === "json" ? "json" : "form");
        }}
      >
        <TabsList>
          <TabsTrigger value="form" disabled={properties.length === 0}>
            Form
          </TabsTrigger>
          <TabsTrigger value="json">JSON</TabsTrigger>
        </TabsList>
        <TabsContent value="form">
          <FieldGroup>
            {properties.map(([name, property]) => {
              const value = values[name] ?? initialValue(property);
              return (
                <Field key={name} data-invalid={invalid === name}>
                  <FieldLabel htmlFor={`argument-${name}`}>
                    <span className="font-mono">{name}</span>
                    {required.has(name) && <span className="text-destructive">*</span>}
                    <span className="font-normal text-muted-foreground">
                      {schemaType(property) ?? "json"}
                    </span>
                  </FieldLabel>
                  <ArgumentInput
                    id={`argument-${name}`}
                    schema={property}
                    value={value}
                    invalid={invalid === name}
                    onChange={(next) => set(name, next)}
                  />
                  {property.description !== undefined && (
                    <FieldDescription>{property.description}</FieldDescription>
                  )}
                </Field>
              );
            })}
          </FieldGroup>
        </TabsContent>
        <TabsContent value="json">
          <Field data-invalid={invalid === "arguments"}>
            <Textarea
              className="min-h-40 font-mono"
              spellCheck={false}
              aria-invalid={invalid === "arguments"}
              value={raw}
              onChange={(event) => setRaw(event.target.value)}
            />
          </Field>
        </TabsContent>
      </Tabs>
      <div className="flex items-center gap-2">
        <Button type="submit" disabled={pending}>
          {pending ? <Spinner data-icon="inline-start" /> : <PlayIcon data-icon="inline-start" />}
          Call
        </Button>
        {invalid !== null && (
          <span className="text-destructive text-xs">{invalid} is not valid</span>
        )}
      </div>
    </form>
  );
}

interface ArgumentInputProps {
  readonly id: string;
  readonly schema: JsonSchemaNode;
  readonly value: FieldValue;
  readonly invalid: boolean;
  readonly onChange: (value: FieldValue) => void;
}

function ArgumentInput({ id, schema, value, invalid, onChange }: ArgumentInputProps) {
  if (value._tag === "Flag") {
    return (
      <Switch id={id} checked={value.on} onCheckedChange={(on) => onChange({ _tag: "Flag", on })} />
    );
  }
  if (schema.enum !== undefined) {
    const items = [
      { label: "—", value: "" },
      ...schema.enum.map((option) => ({ label: String(option), value: String(option) })),
    ];
    return (
      <Select
        items={items}
        value={value.text}
        onValueChange={(next) => onChange({ _tag: "Text", text: next ?? "" })}
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
    );
  }
  const type = schemaType(schema);
  if (isStructured(schema) || (type === "array" && !isStructured(schema))) {
    return (
      <Textarea
        id={id}
        className="font-mono"
        spellCheck={false}
        aria-invalid={invalid}
        placeholder={type === "array" ? "one item per line" : "JSON"}
        value={value.text}
        onChange={(event) => onChange({ _tag: "Text", text: event.target.value })}
      />
    );
  }
  return (
    <Input
      id={id}
      inputMode={type === "number" || type === "integer" ? "decimal" : undefined}
      aria-invalid={invalid}
      value={value.text}
      onChange={(event) => onChange({ _tag: "Text", text: event.target.value })}
    />
  );
}
