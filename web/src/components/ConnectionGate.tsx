import { useQueryClient } from "@tanstack/react-query";
import * as Effect from "effect/Effect";
import { cn } from "cn";
import { CheckIcon, CopyIcon, RotateCwIcon } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import { Button } from "~/components/ui/button.tsx";
import { Spinner } from "~/components/ui/spinner.tsx";
import { Tooltip, TooltipContent, TooltipTrigger } from "~/components/ui/tooltip.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { type ConnectionLoss, useConnection } from "~/daemon/connection.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { useEffectCallback, useFork } from "~/effect/react.tsx";

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

/** The whole window, on a grid that fades out around one panel. */
function FullScreen({ children }: { readonly children: ReactNode }) {
  return (
    <main className="fixed inset-0 z-50 flex items-center justify-center bg-background p-6">
      <div
        aria-hidden
        className="absolute inset-0 bg-[linear-gradient(to_right,var(--color-border)_1px,transparent_1px),linear-gradient(to_bottom,var(--color-border)_1px,transparent_1px)] bg-size-[2rem_2rem] mask-[radial-gradient(ellipse_at_center,black_10%,transparent_65%)] opacity-60"
      />
      {/* A panel of its own; Card is a page section in this UI's density. */}
      <section className="relative w-full max-w-md overflow-hidden rounded-lg border bg-card shadow-2xl">
        {children}
      </section>
    </main>
  );
}

// A state's colour is its word's.
const tones = {
  primary: "text-primary",
  warning: "text-warning",
  destructive: "text-destructive",
} as const;

/** The panel's top: the state as one word, in its colour. */
function StateBanner({ word, tone }: { readonly word: string; readonly tone: keyof typeof tones }) {
  return (
    <header className="flex justify-center border-b px-6 py-7">
      <span
        className={cn("font-mono font-semibold text-3xl uppercase tracking-tight", tones[tone])}
      >
        {word}
      </span>
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
      <StateBanner word="agx" tone="primary" />
      <p className="flex items-center justify-center gap-2 p-6 text-muted-foreground text-xs">
        <Spinner className="size-3" />
        Connecting to the daemon…
      </p>
    </FullScreen>
  );
}

const screens: Record<
  ConnectionLoss,
  {
    readonly word: string;
    readonly tone: keyof typeof tones;
    readonly title: string;
    readonly command: string;
    readonly waiting: string;
  }
> = {
  "signed-out": {
    word: "Signed out",
    tone: "warning",
    title: "This page's session ended",
    command: "agx ui",
    waiting: "Waiting for sign-in…",
  },
  unreachable: {
    word: "Offline",
    tone: "destructive",
    title: "The daemon does not answer",
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
  return (
    <FullScreen>
      <StateBanner word={screen.word} tone={screen.tone} />
      <div className="flex flex-col items-center gap-4 px-6 pt-6 pb-5 text-center">
        <h1 className="font-heading font-semibold text-base tracking-tight">{screen.title}</h1>
        <div className="flex items-center gap-1 rounded-md border bg-muted/50 py-0.5 pr-0.5 pl-3">
          <code className="font-mono text-sm">{screen.command}</code>
          <CopyButton text={screen.command} />
        </div>
        <div className="flex gap-2">
          <Button size="sm" variant="outline" onClick={onRetry}>
            <RotateCwIcon data-icon="inline-start" />
            Try now
          </Button>
        </div>
        <span className="flex items-center gap-2 text-muted-foreground text-xs" aria-live="polite">
          <Spinner className="size-3" />
          {screen.waiting}
        </span>
      </div>
    </FullScreen>
  );
}

// How long the button says it copied before offering to copy again.
const copiedFor = "1500 millis";

type CopyState = "idle" | "copied" | "failed";

const copyLabels: Record<CopyState, string> = {
  idle: "Copy",
  copied: "Copied",
  failed: "Could not copy",
};

/** Copies text to the clipboard, saying so for a moment. */
function CopyButton({ text }: { readonly text: string }) {
  const [state, setState] = useState<CopyState>("idle");
  const copy = useEffectCallback(
    () =>
      Effect.tryPromise(() => navigator.clipboard.writeText(text)).pipe(
        // The browser refuses without permission or focus; the button says so instead.
        Effect.match({ onFailure: () => setState("failed"), onSuccess: () => setState("copied") }),
      ),
    [text],
  );
  useFork(
    () =>
      state === "idle"
        ? undefined
        : Effect.sleep(copiedFor).pipe(Effect.andThen(Effect.sync(() => setState("idle")))),
    [state],
  );
  return (
    <Tooltip>
      {/* Outside the app's tooltip provider, so the delay is set here: none, as for every copy button. */}
      <TooltipTrigger
        delay={0}
        closeOnClick={false}
        render={
          <Button size="icon-sm" variant="ghost" aria-label={copyLabels[state]} onClick={copy}>
            {state === "copied" ? <CheckIcon /> : <CopyIcon />}
          </Button>
        }
      />
      <TooltipContent side="right">{copyLabels[state]}</TooltipContent>
    </Tooltip>
  );
}
