import { cn } from "cn";
import {
  createContext,
  Fragment,
  type ReactElement,
  type ReactNode,
  use,
  useMemo,
  useState,
} from "react";
import { createPortal } from "react-dom";

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "~/components/ui/breadcrumb.tsx";
import { SidebarTrigger } from "~/components/ui/sidebar.tsx";
import { NewItemMenu } from "~/features/config/NewItemMenu.tsx";
import { StagedButton } from "~/features/config/ReviewChanges.tsx";
import { ScopeControl } from "~/features/scope/ScopeControl.tsx";
import { pageTitle } from "~/lib/title.ts";

interface Slots {
  /** Beside the breadcrumbs, for switching between views of what the page shows. */
  readonly views: HTMLElement | null;
  /** At the header's end, for actions. */
  readonly actions: HTMLElement | null;
}

/** Where parts of a page, such as an editor, put their views and actions: the page header. */
const PageSlots = createContext<Slots>({ views: null, actions: null });

export interface Crumb {
  readonly label: ReactNode;
  /** A router link to what the crumb names; the last crumb is the page itself and has none. */
  readonly link?: ReactElement;
  /** Names an ID or path, set in the code font. */
  readonly mono?: boolean;
}

interface PageProps {
  /** The window title's parts, most specific first, as pageTitle takes them. */
  readonly title: ReadonlyArray<string | undefined>;
  /** The page's place: its section first, then what it shows. */
  readonly crumbs: ReadonlyArray<Crumb>;
  readonly actions?: ReactNode;
  /** The content takes the window's remaining height, for an editor that scrolls on its own. */
  readonly fill?: boolean;
  readonly className?: string;
  readonly children: ReactNode;
}

/**
 * A page: a header with where it is and what can be done, and the content centered below. Parts
 * of the page add actions to the header with PageActions, so every action is at the top.
 */
export function Page({ title, crumbs, actions, fill = false, className, children }: PageProps) {
  const [views, setViews] = useState<HTMLElement | null>(null);
  const [actionsSlot, setActionsSlot] = useState<HTMLElement | null>(null);
  const slots = useMemo(() => ({ views, actions: actionsSlot }), [views, actionsSlot]);
  const width = cn("mx-auto w-full max-w-5xl px-4", className);
  return (
    <>
      <title>{pageTitle(...title)}</title>
      <header className="sticky top-0 z-10 flex items-center border-b bg-background">
        {/* At the sidebar's edge, beside what it hides. The header spans the window; only the
            content below is centered. */}
        <div className="flex w-10 shrink-0 justify-center self-start pt-2.5">
          <SidebarTrigger />
        </div>
        <div className="flex min-h-11 min-w-0 flex-1 flex-wrap items-center gap-3 py-1.5 pr-3">
          <ScopeControl />
          <Breadcrumb className="min-w-0">
            <BreadcrumbList className="flex-nowrap">
              {crumbs.map((crumb, index) => (
                // Crumbs are a path; a position names one.
                // oxlint-disable-next-line react/no-array-index-key
                <Fragment key={index}>
                  {index > 0 && <BreadcrumbSeparator />}
                  <BreadcrumbItem className={cn("min-w-0", crumb.mono === true && "font-mono")}>
                    {index === crumbs.length - 1 ? (
                      <BreadcrumbPage className="truncate">
                        <h1
                          className={cn(
                            "truncate font-semibold",
                            crumb.mono !== true && "font-heading",
                          )}
                        >
                          {crumb.label}
                        </h1>
                      </BreadcrumbPage>
                    ) : crumb.link === undefined ? (
                      <span className="truncate">{crumb.label}</span>
                    ) : (
                      <BreadcrumbLink render={crumb.link} className="truncate">
                        {crumb.label}
                      </BreadcrumbLink>
                    )}
                  </BreadcrumbItem>
                </Fragment>
              ))}
            </BreadcrumbList>
          </Breadcrumb>
          <div ref={setViews} className="flex shrink-0 items-center gap-2 empty:hidden" />
          <div ref={setActionsSlot} className="ml-auto flex shrink-0 items-center gap-2">
            {actions}
            <NewItemMenu />
            <StagedButton />
          </div>
        </div>
      </header>
      <PageSlots value={slots}>
        <div
          data-slot="page"
          className={cn(width, "flex flex-col gap-2 py-3", fill && "min-h-0 flex-1")}
        >
          {children}
        </div>
      </PageSlots>
    </>
  );
}

/** Actions of a part of the page, shown in the page header after the page's own. */
export function PageActions({ children }: { readonly children: ReactNode }) {
  const slot = use(PageSlots).actions;
  return slot === null ? null : createPortal(children, slot);
}

/** Views of what the page shows, such as tabs, shown in the page header after the breadcrumbs. */
export function PageViews({ children }: { readonly children: ReactNode }) {
  const slot = use(PageSlots).views;
  return slot === null ? null : createPortal(children, slot);
}
