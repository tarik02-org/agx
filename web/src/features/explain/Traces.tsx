import { ArrowRightIcon, CircleCheckIcon, CircleXIcon } from "lucide-react";

import { Badge } from "~/components/ui/badge.tsx";
import type { ResourceRef, ResourceTrace, ResolutionStep } from "~/gen/agx/v1/agx_pb.ts";
import { ResolutionStep_Change } from "~/gen/agx/v1/agx_pb.ts";
import { shortPath } from "~/lib/path.ts";

/** A resource's kind and ID, from the reference's set field. */
export function resourceKey(ref: ResourceRef | undefined): {
  readonly kind: string;
  readonly id: string;
} {
  switch (ref?.resource.case) {
    case "skillId":
      return { kind: "skill", id: ref.resource.value };
    case "instructionsId":
      return { kind: "instructions", id: ref.resource.value };
    case "mcpId":
      return { kind: "mcp", id: ref.resource.value };
    case undefined:
      return { kind: "unknown", id: "" };
  }
}

interface TracesProps {
  readonly traces: ReadonlyArray<ResourceTrace>;
  readonly selected: ReadonlySet<string>;
  readonly home: string;
}

/** Every resource a rule touched for the directory, and the rules that decided it. */
export function Traces({ traces, selected, home }: TracesProps) {
  return (
    <ul className="flex flex-col divide-y">
      {traces.map((trace) => {
        const { kind, id } = resourceKey(trace.resource);
        const isSelected = selected.has(`${kind}/${id}`);
        return (
          <li key={`${kind}/${id}`} className="flex flex-col gap-1 py-2">
            <div className="flex items-center gap-2">
              {isSelected ? (
                <CircleCheckIcon className="text-success" aria-label="selected" />
              ) : (
                <CircleXIcon className="text-destructive" aria-label="removed" />
              )}
              <Badge variant="secondary">{kind}</Badge>
              <span className="font-medium font-mono">{id}</span>
            </div>
            <ul className="flex flex-col gap-0.5 pl-6">
              {trace.steps.map((step) => (
                <Step
                  key={`${step.change}:${step.source?.path}:${step.source?.line}:${step.bundleChain.join(">")}`}
                  step={step}
                  home={home}
                />
              ))}
            </ul>
          </li>
        );
      })}
    </ul>
  );
}

function Step({ step, home }: { readonly step: ResolutionStep; readonly home: string }) {
  const removed = step.change === ResolutionStep_Change.REMOVE;
  return (
    <li className="flex flex-wrap items-center gap-1.5 text-muted-foreground text-xs">
      <Badge variant={removed ? "destructive" : "success"}>{removed ? "remove" : "add"}</Badge>
      {step.harness !== "" ? (
        <span>
          harness <span className="font-mono text-foreground">{step.harness}</span>
        </span>
      ) : (
        <span>
          scope{" "}
          <span className="font-mono text-foreground">
            {step.scopeDirectory === "" ? "everywhere" : shortPath(step.scopeDirectory, home)}
          </span>
        </span>
      )}
      {step.bundleChain.length > 0 && (
        <span className="flex items-center gap-1">
          via
          {step.bundleChain.map((bundle, index) => (
            <span key={bundle} className="flex items-center gap-1 font-mono text-foreground">
              {index > 0 && <ArrowRightIcon />}
              {bundle}
            </span>
          ))}
        </span>
      )}
      {step.source !== undefined && (
        <span className="font-mono">
          {shortPath(step.source.path, home)}:{step.source.line}
        </span>
      )}
    </li>
  );
}
