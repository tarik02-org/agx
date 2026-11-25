import { type ReactElement, type ReactNode, useState } from "react";

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

interface ConfirmActionProps {
  /** The button that opens the confirmation. */
  readonly trigger: ReactElement;
  readonly title: string;
  readonly description: ReactNode;
  readonly action: string;
  readonly onConfirm: () => void;
}

/**
 * Asks before an action that ends something running. Clicking outside or pressing Escape
 * cancels, as Cancel does.
 */
export function ConfirmAction({
  trigger,
  title,
  description,
  action,
  onConfirm,
}: ConfirmActionProps) {
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={trigger} />
      <DialogContent className="sm:max-w-sm" showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <DialogClose render={<Button variant="outline">Cancel</Button>} />
          <Button
            variant="destructive"
            onClick={() => {
              setOpen(false);
              onConfirm();
            }}
          >
            {action}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
