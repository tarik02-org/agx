import { Launch_State, McpRuntime_State, SecretStatus_State } from "~/gen/agx/v1/agx_pb.ts";

export type StateVariant = "secondary" | "success" | "warning" | "destructive";

interface StateLabel {
  readonly label: string;
  readonly variant: StateVariant;
}

export const launchStates: Record<Launch_State, StateLabel> = {
  [Launch_State.UNSPECIFIED]: { label: "unknown", variant: "secondary" },
  [Launch_State.PREPARED]: { label: "prepared", variant: "secondary" },
  [Launch_State.RUNNING]: { label: "running", variant: "success" },
  [Launch_State.FINISHED]: { label: "finished", variant: "secondary" },
  [Launch_State.REVOKED]: { label: "revoked", variant: "destructive" },
  [Launch_State.EXPIRED]: { label: "expired", variant: "warning" },
};

export const mcpStates: Record<McpRuntime_State, StateLabel> = {
  [McpRuntime_State.UNSPECIFIED]: { label: "unknown", variant: "secondary" },
  [McpRuntime_State.CONNECTING]: { label: "connecting", variant: "secondary" },
  [McpRuntime_State.READY]: { label: "ready", variant: "success" },
  [McpRuntime_State.NEEDS_AUTH]: { label: "needs login", variant: "warning" },
  [McpRuntime_State.FAILED]: { label: "failed", variant: "destructive" },
};

export const secretStates: Record<SecretStatus_State, StateLabel> = {
  [SecretStatus_State.UNSPECIFIED]: { label: "unknown", variant: "secondary" },
  [SecretStatus_State.UNCONFIGURED]: { label: "not set", variant: "warning" },
  [SecretStatus_State.PENDING]: { label: "login pending", variant: "secondary" },
  [SecretStatus_State.READY]: { label: "ready", variant: "success" },
  [SecretStatus_State.NEEDS_LOGIN]: { label: "needs login", variant: "warning" },
  [SecretStatus_State.UNAVAILABLE]: { label: "unavailable", variant: "destructive" },
};

export const isActiveLaunch = (state: Launch_State): boolean =>
  state === Launch_State.PREPARED || state === Launch_State.RUNNING;
