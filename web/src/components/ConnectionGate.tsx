import { useQueryClient } from "@tanstack/react-query";
import { KeyRoundIcon, UnplugIcon } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "~/components/ui/empty.tsx";
import { Spinner } from "~/components/ui/spinner.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { type ConnectionLoss, useConnection } from "~/daemon/connection.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";

// While the connection is lost, the configuration is read again this often to notice it is back:
// the daemon restarting, or `agx ui` signing the browser in again from another tab.
const probeMs = 2000;
// The daemon usually answers within a frame; a loading screen shown at once would only flicker.
const showLoadingAfterMs = 200;

/**
 * Shows the pages only while the daemon answers this page. Until it first does, and whenever the
 * session ends or the daemon goes away, the whole window says so; pages stay mounted underneath
 * once loaded, so unsaved edits survive the daemon restarting.
 */
export function ConnectionGate({ children }: { readonly children: ReactNode }) {
  const lost = useConnection((state) => state.lost);
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
    { refetchInterval: lost === undefined ? false : probeMs },
  );

  // Reads that failed while the connection was lost are read again once it is back.
  const client = useQueryClient();
  const wasLost = useRef(lost);
  useEffect(() => {
    if (wasLost.current !== undefined && lost === undefined) {
      void client.invalidateQueries();
    }
    wasLost.current = lost;
  }, [client, lost]);

  if (lost !== undefined) {
    return (
      <>
        {configuration.data !== undefined && <div inert>{children}</div>}
        <ConnectionScreen loss={lost} />
      </>
    );
  }
  if (configuration.isPending) {
    return <LoadingScreen />;
  }
  // Any other failure is the page's to show, such as an invalid configuration to fix.
  return children;
}

function FullScreen({ children }: { readonly children: ReactNode }) {
  return (
    <main className="fixed inset-0 z-50 flex flex-col bg-background">
      <span className="px-4 py-3 font-mono font-semibold">agx</span>
      {children}
    </main>
  );
}

function LoadingScreen() {
  const [shown, setShown] = useState(false);
  useEffect(() => {
    const timer = setTimeout(() => setShown(true), showLoadingAfterMs);
    return () => clearTimeout(timer);
  }, []);
  return (
    <FullScreen>
      {shown && (
        <Empty>
          <EmptyHeader>
            <EmptyMedia>
              <Spinner />
            </EmptyMedia>
            <EmptyDescription>Connecting to the daemon…</EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
    </FullScreen>
  );
}

const screens: Record<
  ConnectionLoss,
  {
    readonly icon: ReactNode;
    readonly title: string;
    readonly description: string;
    readonly command: string;
    readonly waiting: string;
  }
> = {
  "signed-out": {
    icon: <KeyRoundIcon />,
    title: "Signed out",
    description:
      "This page's session with the daemon ended: the daemon restarted, or the page sat idle for 12 hours. Sign in again from a terminal:",
    command: "agx ui",
    waiting: "This page continues once you have signed in.",
  },
  unreachable: {
    icon: <UnplugIcon />,
    title: "Daemon unreachable",
    description: "The daemon does not answer. If it is not running, start it:",
    command: "agx daemon start",
    waiting: "Retrying…",
  },
};

function ConnectionScreen({ loss }: { readonly loss: ConnectionLoss }) {
  const screen = screens[loss];
  return (
    <FullScreen>
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant="icon">{screen.icon}</EmptyMedia>
          <EmptyTitle>{screen.title}</EmptyTitle>
          <EmptyDescription>{screen.description}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <code className="rounded-sm border bg-muted px-2 py-1 font-mono">{screen.command}</code>
          <span className="flex items-center gap-2 text-muted-foreground" aria-live="polite">
            <Spinner className="size-3" />
            {screen.waiting}
          </span>
        </EmptyContent>
      </Empty>
    </FullScreen>
  );
}
