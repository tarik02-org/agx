import { KeyRoundIcon } from "lucide-react";
import { useState } from "react";

import { Button } from "~/components/ui/button.tsx";
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
import { Field, FieldDescription, FieldGroup, FieldLabel } from "~/components/ui/field.tsx";
import { Textarea } from "~/components/ui/textarea.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonMutation } from "~/daemon/query.ts";
import { submitOnModEnter } from "~/lib/keyboard.ts";

/** Stores a keychain secret's value; the page never reads it back. */
export function SetSecretDialog({ secretId }: { readonly secretId: string }) {
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const store = useDaemonMutation(
    (next: string) => Daemon.use((daemon) => daemon.setSecret(secretId, next)),
    [queryKeys.secrets],
  );
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        setValue("");
      }}
    >
      <DialogTrigger
        render={
          <Button variant="ghost" size="sm">
            <KeyRoundIcon data-icon="inline-start" />
            Set value
          </Button>
        }
      />
      <DialogContent>
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            store.mutate(value, {
              onSuccess: () => {
                toast.add({ type: "success", title: `Stored ${secretId}` });
                setOpen(false);
              },
              onError: (error) =>
                toast.add({ type: "error", title: "Storing failed", description: error.message }),
            });
          }}
        >
          <DialogHeader>
            <DialogTitle>Set {secretId}</DialogTitle>
            <DialogDescription>
              Stored in the OS keychain; MCP servers using it reconnect with the new value.
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="secret-value">Value</FieldLabel>
              <Textarea
                id="secret-value"
                className="font-mono"
                autoComplete="off"
                spellCheck={false}
                onKeyDown={submitOnModEnter}
                value={value}
                onChange={(event) => setValue(event.target.value)}
              />
              <FieldDescription>Stored exactly as typed. ⌘/Ctrl+Enter stores it.</FieldDescription>
            </Field>
          </FieldGroup>
          <DialogFooter>
            <DialogClose render={<Button variant="outline">Cancel</Button>} />
            <Button type="submit" disabled={value === "" || store.isPending}>
              Store
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
