import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { useScopeDirectory } from "~/features/scope/scope.ts";

/** Where stdio servers without their own cwd start: the scope directory, or home when unscoped. */
export const useInspectorDirectory = () => {
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const scope = useScopeDirectory();
  return scope === "" ? configuration.data?.home : scope;
};

/** A configured MCP server with its tools, started when first asked for. */
export const useServerTools = (mcpId: string, enabled = true) => {
  const directory = useInspectorDirectory() ?? "";
  const tools = useDaemonQuery(
    queryKeys.tools(directory, mcpId),
    Daemon.use((daemon) => daemon.tools(directory, mcpId)),
    { enabled: enabled && directory !== "", retry: false },
  );
  return { tools, directory };
};
