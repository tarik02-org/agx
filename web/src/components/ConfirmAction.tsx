import type { ReactElement, ReactNode } from "react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "~/components/ui/alert-dialog.tsx";

interface ConfirmActionProps {
  /** The button that opens the confirmation. */
  readonly trigger: ReactElement;
  readonly title: string;
  readonly description: ReactNode;
  readonly action: string;
  readonly onConfirm: () => void;
}

/** Asks before an action that ends something running. */
export function ConfirmAction({
  trigger,
  title,
  description,
  action,
  onConfirm,
}: ConfirmActionProps) {
  return (
    <AlertDialog>
      <AlertDialogTrigger render={trigger} />
      <AlertDialogContent size="sm">
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onConfirm}>
            {action}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
