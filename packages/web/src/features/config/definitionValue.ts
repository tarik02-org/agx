import { fromJson, type JsonObject, type JsonValue, toJson } from "@bufbuild/protobuf";
import { type Value, ValueSchema } from "@bufbuild/protobuf/wkt";
import type { Definition } from "agx-client/protocol";
import { stringify } from "yaml";

/** Where a field is in a definition's value; forms edit the value through these. */
export type FieldPath = ReadonlyArray<string>;

/**
 * agx's value tags: a literal, or a variable, file, or secret resolved at launch. A value writes
 * one as an object with the tag's $-key, such as `{"$env": "NAME"}`.
 */
export type ValueTag = "" | "!env" | "!file" | "!secret";

export interface TaggedValue {
  readonly tag: ValueTag;
  readonly value: string;
}

export interface MapEntry {
  readonly key: string;
  readonly value: TaggedValue;
}

export const isJsonObject = (value: unknown): value is JsonObject =>
  typeof value === "object" && value !== null && !Array.isArray(value);

/** A local definition's editable value; empty for one without, such as a new definition. */
export const definitionValue = (definition: Definition | undefined): JsonObject => {
  const value = definition?.value === undefined ? undefined : toJson(ValueSchema, definition.value);
  return isJsonObject(value) ? value : {};
};

export const toValue = (value: JsonObject): Value => fromJson(ValueSchema, value);

/** A value as YAML text, for showing and comparing it. */
export const valueText = (value: JsonObject | undefined): string =>
  value === undefined ? "" : stringify(value);

export const sameValue = (a: JsonValue, b: JsonValue): boolean =>
  JSON.stringify(a) === JSON.stringify(b);

const getIn = (value: JsonValue, path: FieldPath): JsonValue | undefined => {
  let node: JsonValue | undefined = value;
  for (const key of path) {
    if (isJsonObject(node)) {
      node = node[key];
    } else if (Array.isArray(node)) {
      node = node[Number(key)];
    } else {
      return undefined;
    }
  }
  return node;
};

// Changes return a new value; a copy keeps the one a React state holds untouched.
const edit = (value: JsonObject, change: (copy: JsonObject) => void): JsonObject => {
  const copy = structuredClone(value);
  change(copy);
  return copy;
};

const setIn = (value: JsonObject, path: FieldPath, next: JsonValue) => {
  let node = value;
  for (const key of path.slice(0, -1)) {
    const child = node[key];
    const parent: JsonObject = isJsonObject(child) ? child : {};
    node[key] = parent;
    node = parent;
  }
  const last = path.at(-1);
  if (last !== undefined) {
    node[last] = next;
  }
};

const deleteIn = (value: JsonObject, path: FieldPath) => {
  const parent = getIn(value, path.slice(0, -1));
  const last = path.at(-1);
  if (isJsonObject(parent) && last !== undefined) {
    delete parent[last];
  }
};

const scalarText = (node: JsonValue | undefined): string | undefined =>
  typeof node === "string" || typeof node === "number" || typeof node === "boolean"
    ? String(node)
    : undefined;

const tags: ReadonlyArray<Exclude<ValueTag, "">> = ["!env", "!file", "!secret"];
const tagKey = (tag: Exclude<ValueTag, "">) => `$${tag.slice(1)}`;

const toTagged = (node: JsonValue | undefined): TaggedValue | undefined => {
  if (isJsonObject(node)) {
    const keys = Object.keys(node);
    const tag = tags.find((candidate) => keys.length === 1 && keys[0] === tagKey(candidate));
    const name = tag === undefined ? undefined : node[tagKey(tag)];
    return tag !== undefined && typeof name === "string" ? { tag, value: name } : undefined;
  }
  const text = scalarText(node);
  return text === undefined ? undefined : { tag: "", value: text };
};

const taggedNode = ({ tag, value }: TaggedValue): JsonValue =>
  tag === "" ? value : { [tagKey(tag)]: value };

export const readString = (value: JsonObject, path: FieldPath): string =>
  scalarText(getIn(value, path)) ?? "";

export const readBoolean = (value: JsonObject, path: FieldPath): boolean =>
  getIn(value, path) === true;

export const readList = (value: JsonObject, path: FieldPath): ReadonlyArray<string> => {
  const node = getIn(value, path);
  return Array.isArray(node) ? node.map((item) => scalarText(item) ?? "") : [];
};

/** Whether a field is written, even empty. */
export const hasField = (value: JsonObject, path: FieldPath): boolean =>
  getIn(value, path) !== undefined;

/** Top-level fields as written, for putFields to put back. */
export type TakenFields = ReadonlyArray<readonly [string, JsonValue]>;

export const takeFields = (value: JsonObject, fields: ReadonlyArray<string>): TakenFields =>
  fields.flatMap((field) => {
    const node = value[field];
    return node === undefined ? [] : [[field, node] as const];
  });

export const putFields = (value: JsonObject, fields: TakenFields): JsonObject =>
  edit(value, (copy) => {
    for (const [field, node] of fields) {
      copy[field] = node;
    }
  });

/** Writes a field as an empty string unless it is written already. */
export const ensureField = (value: JsonObject, path: FieldPath): JsonObject =>
  hasField(value, path) ? value : edit(value, (copy) => setIn(copy, path, ""));

/** Whether a field holds a map or list where the form expects a plain value. */
export const isStructured = (value: JsonObject, path: FieldPath): boolean => {
  const node = getIn(value, path);
  return typeof node === "object" && node !== null && toTagged(node) === undefined;
};

export const readValue = (value: JsonObject, path: FieldPath): TaggedValue =>
  toTagged(getIn(value, path)) ?? { tag: "", value: "" };

export const readMap = (value: JsonObject, path: FieldPath): ReadonlyArray<MapEntry> => {
  const node = getIn(value, path);
  return isJsonObject(node)
    ? Object.entries(node).map(([key, entry]) => ({
        key,
        value: toTagged(entry) ?? { tag: "", value: "" },
      }))
    : [];
};

/** Sets a field; an empty string or list, or false, removes it. */
export const writeField = (
  value: JsonObject,
  path: FieldPath,
  next: string | number | boolean | ReadonlyArray<string>,
): JsonObject =>
  edit(value, (copy) => {
    if (next === "" || next === false || (typeof next === "object" && next.length === 0)) {
      deleteIn(copy, path);
    } else {
      setIn(copy, path, typeof next === "object" ? [...next] : next);
    }
  });

/** Sets a value field with its tag; an empty literal removes it. */
export const writeValue = (value: JsonObject, path: FieldPath, next: TaggedValue): JsonObject =>
  edit(value, (copy) => {
    if (next.value === "" && next.tag === "") {
      deleteIn(copy, path);
    } else {
      setIn(copy, path, taggedNode(next));
    }
  });

/** Replaces a map field's entries, in order; no entries removes the field. */
export const writeMap = (
  value: JsonObject,
  path: FieldPath,
  entries: ReadonlyArray<MapEntry>,
): JsonObject =>
  edit(value, (copy) => {
    if (entries.length === 0) {
      deleteIn(copy, path);
      return;
    }
    setIn(
      copy,
      path,
      Object.fromEntries(entries.map((entry) => [entry.key, taggedNode(entry.value)])),
    );
  });

/** Removes fields, such as those of a source the form switched away from. */
export const removeFields = (value: JsonObject, fields: ReadonlyArray<string>): JsonObject =>
  edit(value, (copy) => {
    for (const field of fields) {
      delete copy[field];
    }
  });
