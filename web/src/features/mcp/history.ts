import * as Option from "effect/Option";
import * as Schema from "effect/Schema";
import { useState } from "react";

// Calls made from this browser, newest first, kept in local storage across reloads.
const HistoryEntry = Schema.Struct({
  server: Schema.String,
  tool: Schema.String,
  argumentsJson: Schema.String,
  isError: Schema.Boolean,
  at: Schema.Number,
});
export type HistoryEntry = typeof HistoryEntry.Type;

const storageKey = "agx.mcp.history";
const limit = 50;
const decodeHistory = Schema.decodeUnknownOption(Schema.fromJsonString(Schema.Array(HistoryEntry)));
const encodeHistory = Schema.encodeSync(Schema.fromJsonString(Schema.Array(HistoryEntry)));

const read = (): ReadonlyArray<HistoryEntry> =>
  Option.getOrElse(decodeHistory(globalThis.localStorage.getItem(storageKey) ?? "[]"), () => []);

export function useCallHistory() {
  const [entries, setEntries] = useState(read);
  const record = (entry: HistoryEntry) => {
    const next = [entry, ...entries].slice(0, limit);
    globalThis.localStorage.setItem(storageKey, encodeHistory(next));
    setEntries(next);
  };
  return { entries, record };
}
