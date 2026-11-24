export interface TextContent {
  type: "text";
  text: string;
}
export interface MediaContent {
  type: "image" | "audio";
  /** Base64-encoded data. */
  data: string;
  mimeType: string;
}
export interface ResourceLinkContent {
  type: "resource_link";
  uri: string;
  name: string;
  title?: string;
  description?: string;
  mimeType?: string;
  size?: number;
}
export interface EmbeddedResourceContent {
  type: "resource";
  resource: { uri: string; mimeType?: string; text?: string; blob?: string };
}
export type Content = TextContent | MediaContent | ResourceLinkContent | EmbeddedResourceContent;

export interface CallResult<S = unknown> {
  /** The tool reported an error; content describes it. */
  isError: boolean;
  content: Content[];
  /** The structured result, when the tool returned one. */
  structuredContent: S | undefined;
  /** The text content, joined by newlines. */
  text: string;
}

export interface CallOptions {
  /** How long the daemon waits for the result; it waits up to ten minutes by default. */
  timeoutMs?: number;
  signal?: AbortSignal;
}

/** A failed call with a diagnostic message. */
export declare class AgxError extends Error {}

/** Calls a tool of an MCP server by name. */
export declare function call(
  server: string,
  tool: string,
  args?: Record<string, unknown>,
  options?: CallOptions,
): Promise<CallResult>;
