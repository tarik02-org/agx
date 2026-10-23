import { useQueryClient } from "@tanstack/react-query";
import {
  CheckIcon,
  CopyIcon,
  KeyRoundIcon,
  RotateCwIcon,
  TerminalIcon,
  UnplugIcon,
} from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import { Badge } from "~/components/ui/badge.tsx";
import { Button } from "~/components/ui/button.tsx";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "~/components/ui/input-group.tsx";
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
        <ConnectionScreen loss={lost} onRetry={() => void configuration.refetch()} />
      </>
    );
  }
  if (configuration.isPending) {
    return <LoadingScreen />;
  }
  // Any other failure is the page's to show, such as an invalid configuration to fix.
  return children;
}

/** The whole window, with one card in the middle: what happened above the rule, what to do below. */
function FullScreen({ children }: { readonly children: ReactNode }) {
  return (
    <main className="fixed inset-0 z-50 flex items-center justify-center bg-background p-6">
      {/* A panel of its own; Card is a page section in this UI's density. */}
      <section className="w-full max-w-md rounded-lg border bg-card text-xs shadow-lg">
        {children}
      </section>
    </main>
  );
}

/** The card's top: the state at a glance. */
function StateHeader({ icon, badge }: { readonly icon: ReactNode; readonly badge: ReactNode }) {
  return (
    <header className="flex flex-col items-center gap-3 border-b p-6">
      <div className="flex size-10 items-center justify-center rounded-md bg-muted text-muted-foreground [&_svg]:size-5">
        {icon}
      </div>
      {badge}
    </header>
  );
}

function LoadingScreen() {
  const [shown, setShown] = useState(false);
  useEffect(() => {
    const timer = setTimeout(() => setShown(true), showLoadingAfterMs);
    return () => clearTimeout(timer);
  }, []);
  if (!shown) {
    return <main className="fixed inset-0 bg-background" aria-busy />;
  }
  return (
    <FullScreen>
      <StateHeader icon={<Spinner />} badge={<Badge variant="secondary">connecting</Badge>} />
      <p className="p-6 text-center text-muted-foreground">Connecting to the agx daemon…</p>
    </FullScreen>
  );
}

const screens: Record<
  ConnectionLoss,
  {
    readonly icon: ReactNode;
    readonly badge: ReactNode;
    readonly title: string;
    readonly description: string;
    readonly command: string;
    readonly waiting: string;
  }
> = {
  "signed-out": {
    icon: <KeyRoundIcon />,
    badge: <Badge variant="warning">signed out</Badge>,
    title: "This page's session ended",
    description:
      "The daemon restarted, or the page sat idle for 12 hours. Sign in again from a terminal; this page carries on by itself.",
    command: "agx ui",
    waiting: "Waiting for sign-in…",
  },
  unreachable: {
    icon: <UnplugIcon />,
    badge: <Badge variant="destructive">offline</Badge>,
    title: "The daemon does not answer",
    description:
      "It stopped or is restarting. If it is not running, start it; this page carries on by itself.",
    command: "agx daemon start",
    waiting: "Retrying…",
  },
};

function ConnectionScreen({
  loss,
  onRetry,
}: {
  readonly loss: ConnectionLoss;
  readonly onRetry: () => void;
}) {
  const screen = screens[loss];
  const [copied, setCopied] = useState(false);
  return (
    <FullScreen>
      <StateHeader icon={screen.icon} badge={screen.badge} />
      <div className="flex flex-col items-center gap-3 p-6 text-center">
        <h1 className="font-heading font-semibold text-sm">{screen.title}</h1>
        <p className="text-balance text-muted-foreground text-xs/relaxed">{screen.description}</p>
        <InputGroup className="max-w-xs">
          <InputGroupAddon>
            <TerminalIcon />
          </InputGroupAddon>
          <InputGroupInput
            readOnly
            aria-label="Command"
            value={screen.command}
            className="font-mono"
            onFocus={(event) => event.currentTarget.select()}
          />
          <InputGroupAddon align="inline-end">
            <InputGroupButton
              size="icon-xs"
              aria-label={copied ? "Copied" : "Copy command"}
              onClick={() =>
                void navigator.clipboard.writeText(screen.command).then(() => setCopied(true))
              }
            >
              {copied ? <CheckIcon /> : <CopyIcon />}
            </InputGroupButton>
          </InputGroupAddon>
        </InputGroup>
      </div>
      <footer className="flex items-center justify-between gap-2 border-t bg-muted/30 px-4 py-2.5">
        <span className="flex items-center gap-2 text-muted-foreground text-xs" aria-live="polite">
          <Spinner className="size-3" />
          {screen.waiting}
        </span>
        <Button size="sm" variant="outline" onClick={onRetry}>
          <RotateCwIcon data-icon="inline-start" />
          Try now
        </Button>
      </footer>
    </FullScreen>
  );
}
