import { useBlocker } from "@tanstack/react-router";
import { useState } from "react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "~/components/ui/alert-dialog.tsx";

// Where a location points, apart from search parameters that only change how a page shows it.
const target = (
  location: { readonly pathname: string; readonly search: object },
  ignore: ReadonlyArray<string>,
): string =>
  JSON.stringify([
    location.pathname,
    Object.entries(location.search)
      .filter(([key]) => !ignore.includes(key))
      .toSorted(([a], [b]) => a.localeCompare(b)),
  ]);

interface UnsavedChangesProps {
  readonly dirty: boolean;
  /** Search parameters whose change keeps the editor, such as its tab. */
  readonly ignore?: ReadonlyArray<string>;
}

/**
 * Asks before leaving an editor whose edits are not staged yet, whether by following a link or
 * by closing or reloading the tab.
 */
const nothingIgnored: ReadonlyArray<string> = [];

export function UnsavedChanges({ dirty, ignore = nothingIgnored }: UnsavedChangesProps) {
  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) => dirty && target(current, ignore) !== target(next, ignore),
    enableBeforeUnload: dirty,
    withResolver: true,
  });
  // Leaving unmounts the editor and this dialog with it, so the dialog closes first and the
  // navigation proceeds once its closing transition has finished.
  const [discarding, setDiscarding] = useState(false);
  return (
    <AlertDialog
      open={blocker.status === "blocked" && !discarding}
      onOpenChange={(open) => {
        if (!open) {
          blocker.reset?.();
        }
      }}
      onOpenChangeComplete={(open) => {
        if (!open && discarding) {
          setDiscarding(false);
          blocker.proceed?.();
        }
      }}
    >
      <AlertDialogContent size="sm">
        <AlertDialogHeader>
          <AlertDialogTitle>Discard your edits?</AlertDialogTitle>
          <AlertDialogDescription>
            They are not staged. Stage them to keep them until you apply.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Keep editing</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={() => setDiscarding(true)}>
            Discard
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
