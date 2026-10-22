import { FileIcon, LinkIcon } from "lucide-react";

import { Alert, AlertTitle } from "~/components/ui/alert.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { type DecodedContent, decodeContent } from "~/features/mcp/content.ts";
import type { CallToolResponse } from "~/gen/agx/v1/agx_pb.ts";
import { Code, Markdown } from "~/lib/markdown.tsx";

const prettyJson = (json: string): string => {
  try {
    return JSON.stringify(JSON.parse(json), null, 2);
  } catch {
    return json;
  }
};

/** A tool's result: its content items, then its structured content. */
export function ToolResult({ response }: { readonly response: CallToolResponse }) {
  const content = decodeContent(response.contentJson);
  return (
    <div className="flex flex-col gap-3">
      {response.isError && (
        <Alert variant="destructive">
          <AlertTitle>The tool reported an error</AlertTitle>
        </Alert>
      )}
      {content.map((item, index) => (
        // Content items have no identity of their own; their order is the result's.
        // oxlint-disable-next-line react/no-array-index-key
        <ContentView key={index} content={item} />
      ))}
      {response.structuredContentJson !== "" && (
        <div className="flex flex-col gap-1">
          <Badge variant="secondary">structured content</Badge>
          <Code code={prettyJson(response.structuredContentJson)} language="json" />
        </div>
      )}
    </div>
  );
}

function ContentView({ content }: { readonly content: DecodedContent }) {
  if (content._tag === "Unknown") {
    return <Code code={JSON.stringify(content.value, null, 2)} language="json" />;
  }
  const { item } = content;
  switch (item.type) {
    case "text":
      return <Markdown>{item.text}</Markdown>;
    case "image":
      return (
        <img
          alt="Tool result"
          className="max-h-96 self-start rounded-md border"
          src={`data:${item.mimeType};base64,${item.data}`}
        />
      );
    case "audio":
      return (
        // The tool's audio has no captions to offer.
        // oxlint-disable-next-line jsx-a11y/media-has-caption
        <audio controls src={`data:${item.mimeType};base64,${item.data}`} />
      );
    case "resource_link":
      return (
        <div className="flex items-center gap-2">
          <LinkIcon className="text-muted-foreground" />
          <span className="font-mono">{item.name ?? item.uri}</span>
          {item.description !== undefined && (
            <span className="text-muted-foreground">{item.description}</span>
          )}
        </div>
      );
    case "resource":
      return (
        <div className="flex flex-col gap-1">
          <div className="flex items-center gap-2 text-muted-foreground">
            <FileIcon />
            <span className="font-mono">{item.resource.uri}</span>
          </div>
          {item.resource.text !== undefined && (
            <Code
              code={item.resource.text}
              language={item.resource.mimeType === "application/json" ? "json" : "text"}
            />
          )}
        </div>
      );
  }
}
