import * as Context from "effect/Context";

export class Configuration extends Context.Service<
  Configuration,
  {
    readonly executable: string;
    readonly config: string | null;
    readonly discovery: string | null;
    readonly harness: string;
    readonly scopeDirectory: string;
    readonly mcpDirectory: string;
    readonly codemode: boolean | null;
  }
>()("agx-opencode/Configuration") {}
