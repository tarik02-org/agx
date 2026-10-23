import { type Document, isMap, isScalar, isSeq, parseDocument, Scalar } from "yaml";

/** Where a field is in a definition's YAML; forms edit the text through a parsed document. */
export type FieldPath = ReadonlyArray<string>;

/** agx's value tags: a literal, or a variable, file, or secret resolved at launch. */
export type ValueTag = "" | "!env" | "!file" | "!secret";

export interface TaggedValue {
  readonly tag: ValueTag;
  readonly value: string;
}

export interface MapEntry {
  readonly key: string;
  readonly value: TaggedValue;
}

const toTag = (tag: string | undefined): ValueTag => {
  switch (tag) {
    case "!env":
    case "!file":
    case "!secret":
      return tag;
    default:
      return "";
  }
};

const toTagged = (node: unknown): TaggedValue =>
  isScalar(node)
    ? { tag: toTag(node.tag), value: node.value === null ? "" : String(node.value) }
    : { tag: "", value: "" };

const taggedNode = ({ tag, value }: TaggedValue): Scalar => {
  const node = new Scalar(value);
  if (tag !== "") {
    node.tag = tag;
  }
  return node;
};

/** The text's parse errors; a form edits only YAML without any. */
export const yamlErrors = (text: string): ReadonlyArray<string> =>
  parseDocument(text).errors.map((error) => error.message);

export const readString = (text: string, path: FieldPath): string => {
  const node = parseDocument(text).getIn(path, true);
  return isScalar(node) && node.value !== null ? String(node.value) : "";
};

export const readBoolean = (text: string, path: FieldPath): boolean =>
  parseDocument(text).getIn(path) === true;

export const readList = (text: string, path: FieldPath): ReadonlyArray<string> => {
  const node = parseDocument(text).getIn(path, true);
  return isSeq(node) ? node.items.map((item) => (isScalar(item) ? String(item.value) : "")) : [];
};

/** Whether a field is written, even empty. */
export const hasField = (text: string, path: FieldPath): boolean => parseDocument(text).hasIn(path);

/** Top-level fields as written, comments and tags included, for putFields to put back. */
export type TakenFields = ReadonlyArray<readonly [string, unknown]>;

export const takeFields = (text: string, fields: ReadonlyArray<string>): TakenFields => {
  const document = parseDocument(text);
  return fields.flatMap((field) =>
    document.has(field) ? [[field, document.get(field, true)] as const] : [],
  );
};

export const putFields = (text: string, fields: TakenFields): string => {
  const document = parseDocument(text);
  for (const [field, node] of fields) {
    document.set(field, node);
  }
  return document.toString();
};

/** Writes a field as an empty string unless it is written already. */
export const ensureField = (text: string, path: FieldPath): string => {
  const document = parseDocument(text);
  if (document.hasIn(path)) {
    return text;
  }
  document.setIn(path, "");
  return document.toString();
};

/** Whether a field holds a map or list where the form expects a plain value. */
export const isStructured = (text: string, path: FieldPath): boolean => {
  const node = parseDocument(text).getIn(path, true);
  return node !== undefined && node !== null && !isScalar(node);
};

export const readValue = (text: string, path: FieldPath): TaggedValue =>
  toTagged(parseDocument(text).getIn(path, true));

export const readMap = (text: string, path: FieldPath): ReadonlyArray<MapEntry> => {
  const node = parseDocument(text).getIn(path, true);
  if (!isMap(node)) {
    return [];
  }
  return node.items.map((pair) => ({
    key: isScalar(pair.key) ? String(pair.key.value) : "",
    value: toTagged(pair.value),
  }));
};

const edit = (text: string, change: (document: Document) => void): string => {
  const document = parseDocument(text);
  change(document);
  // A definition with every field removed stays a mapping.
  return document.contents === null ? "{}\n" : document.toString();
};

/** Sets a field; an empty string or list, or false, removes it. */
export const writeField = (
  text: string,
  path: FieldPath,
  value: string | number | boolean | ReadonlyArray<string>,
): string =>
  edit(text, (document) => {
    if (value === "" || value === false || (typeof value === "object" && value.length === 0)) {
      document.deleteIn(path);
    } else {
      document.setIn(path, document.createNode(value));
    }
  });

/** Sets a value field with its tag; an empty literal removes it. */
export const writeValue = (text: string, path: FieldPath, value: TaggedValue): string =>
  edit(text, (document) => {
    if (value.value === "" && value.tag === "") {
      document.deleteIn(path);
    } else {
      document.setIn(path, taggedNode(value));
    }
  });

/** Replaces a map field's entries, in order; no entries removes the field. */
export const writeMap = (text: string, path: FieldPath, entries: ReadonlyArray<MapEntry>): string =>
  edit(text, (document) => {
    document.deleteIn(path);
    for (const entry of entries) {
      document.setIn([...path, entry.key], taggedNode(entry.value));
    }
  });

/** Removes fields, such as those of a source the form switched away from. */
export const removeFields = (text: string, fields: ReadonlyArray<string>): string =>
  edit(text, (document) => {
    for (const field of fields) {
      document.deleteIn([field]);
    }
  });
