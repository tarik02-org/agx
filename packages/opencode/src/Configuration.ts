import type { ConnectionOptions } from "agx-client/schema";
import * as Context from "effect/Context";

export class Configuration extends Context.Service<
  Configuration,
  {
    readonly connection: ConnectionOptions;
    readonly harness: string;
    readonly scopeDirectory: string;
    readonly mcpDirectory: string;
    readonly codemode: boolean | null;
  }
>()("agx-opencode/Configuration") {}
