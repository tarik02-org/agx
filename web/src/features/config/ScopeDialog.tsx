import { useState } from "react";
import type { ReactElement } from "react";

import { Button } from "~/components/ui/button.tsx";
import { Checkbox } from "~/components/ui/checkbox.tsx";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "~/components/ui/dialog.tsx";
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "~/components/ui/field.tsx";
import { ToggleGroup, ToggleGroupItem } from "~/components/ui/toggle-group.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { type Scope, scopeLabel, select, useStaging } from "~/features/config/staging.ts";
import { DirectoryPicker } from "~/features/explain/DirectoryPicker.tsx";

interface ScopePickerProps {
  readonly scope: Scope;
  readonly onChange: (scope: Scope) => void;
}

/** Where a selection applies: everywhere or under a directory, for every harness or some. */
export function ScopePicker({ scope, onChange }: ScopePickerProps) {
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const everywhere = scope.directory === "";
  return (
    <FieldGroup>
      <Field>
        <FieldLabel>Where</FieldLabel>
        <ToggleGroup
          variant="outline"
          size="sm"
          value={[everywhere ? "everywhere" : "directory"]}
          onValueChange={(next) =>
            onChange({
              ...scope,
              directory: next.includes("directory") ? (configuration.data?.home ?? "~") : "",
            })
          }
        >
          <ToggleGroupItem value="everywhere">Everywhere</ToggleGroupItem>
          <ToggleGroupItem value="directory">Under a directory</ToggleGroupItem>
        </ToggleGroup>
        {!everywhere && (
          <DirectoryPicker
            value={scope.directory}
            suggestions={configuration.data?.scopeDirectories ?? []}
            onChange={(directory) => onChange({ ...scope, directory })}
          />
        )}
      </Field>
      <FieldSet>
        <FieldLegend variant="label">Harnesses</FieldLegend>
        <FieldDescription>None checked applies to every harness.</FieldDescription>
        <div className="flex flex-wrap gap-3">
          {(configuration.data?.harnesses ?? []).map((harness) => (
            <Field key={harness.name} orientation="horizontal" className="w-auto">
              <Checkbox
                id={`harness-${harness.name}`}
                checked={scope.harnesses.includes(harness.name)}
                onCheckedChange={(checked) =>
                  onChange({
                    ...scope,
                    harnesses: checked
                      ? [...scope.harnesses, harness.name].toSorted()
                      : scope.harnesses.filter((name) => name !== harness.name),
                  })
                }
              />
              <FieldLabel htmlFor={`harness-${harness.name}`} className="font-mono">
                {harness.name}
              </FieldLabel>
            </Field>
          ))}
        </div>
      </FieldSet>
    </FieldGroup>
  );
}

interface ScopeDialogProps {
  readonly kind: string;
  readonly id: string;
  readonly trigger: ReactElement;
}

/** Stages enabling or disabling a definition in a scope. */
export function ScopeDialog({ kind, id, trigger }: ScopeDialogProps) {
  const stage = useStaging((state) => state.stage);
  const [action, setAction] = useState<"enable" | "disable">("enable");
  const [scope, setScope] = useState<Scope>({ directory: "", harnesses: [] });
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={trigger} />
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            Select <span className="font-mono">{id}</span>
          </DialogTitle>
          <DialogDescription>
            Adds to the scope rule for this place, or writes a new one; nothing changes until you
            apply.
          </DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel>Action</FieldLabel>
            <ToggleGroup
              variant="outline"
              size="sm"
              value={[action]}
              onValueChange={(next) => setAction(next.includes("disable") ? "disable" : "enable")}
            >
              <ToggleGroupItem value="enable">Enable</ToggleGroupItem>
              <ToggleGroupItem value="disable">Disable</ToggleGroupItem>
            </ToggleGroup>
          </Field>
          <ScopePicker scope={scope} onChange={setScope} />
        </FieldGroup>
        <DialogFooter>
          <DialogClose render={<Button variant="outline">Cancel</Button>} />
          <Button
            onClick={() => {
              stage(select(action, kind, id, scope));
              toast.add({ type: "success", title: `Staged: ${action} ${id} ${scopeLabel(scope)}` });
              setOpen(false);
            }}
          >
            Stage
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
