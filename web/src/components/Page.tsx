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
  /** The toolbar's start, for what the page shows is: where it is defined, how it is viewed. */
  readonly details: HTMLElement | null;
  /** The toolbar's end, for actions. */
  readonly actions: HTMLElement | null;
}

/** Where parts of a page, such as an editor, put their views and actions: the page header. */
const PageSlots = createContext<Slots>({ views: null, details: null, actions: null });

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
  /** What the page shows is, at the toolbar's start. */
  readonly details?: ReactNode;
  readonly actions?: ReactNode;
  /** The content takes the window's remaining height, for an editor that scrolls on its own. */
  readonly fill?: boolean;
  readonly className?: string;
  readonly children: ReactNode;
}

/**
 * A page: a header with the scope and where the page is, a toolbar with what can be done, and the
 * content centered below. Parts of the page add to the toolbar with PageDetails and PageActions,
 * so every action is at the top; without any, there is no toolbar.
 */
export function Page({
  title,
  crumbs,
  details: pageDetails,
  actions,
  fill = false,
  className,
  children,
}: PageProps) {
  const [views, setViews] = useState<HTMLElement | null>(null);
  const [details, setDetails] = useState<HTMLElement | null>(null);
  const [actionsSlot, setActionsSlot] = useState<HTMLElement | null>(null);
  const slots = useMemo(
    () => ({ views, details, actions: actionsSlot }),
    [views, details, actionsSlot],
  );
  const width = cn("mx-auto w-full max-w-5xl px-4", className);
  return (
    <>
      <title>{pageTitle(...title)}</title>
      {/* The header spans the window; only the content below is centered. Its first row is as
          tall as the sidebar's, so their borders meet. */}
      <header className="sticky top-0 z-10 flex flex-col bg-background">
        <div className="flex h-12 min-w-0 shrink-0 items-center gap-3 border-b px-3">
          <SidebarTrigger />
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
          <div className="ml-auto flex shrink-0 items-center gap-2">
            <NewItemMenu />
            <StagedButton />
          </div>
        </div>
        <div className="hidden h-10 min-w-0 shrink-0 items-center gap-3 border-b px-3 has-[>div>*]:flex">
          <div ref={setDetails} className="flex min-w-0 items-center gap-2">
            {pageDetails}
          </div>
          <div ref={setActionsSlot} className="ml-auto flex shrink-0 items-center gap-2">
            {actions}
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

/** What a part of the page shows is, such as where it is defined, at the toolbar's start. */
export function PageDetails({ children }: { readonly children: ReactNode }) {
  const slot = use(PageSlots).details;
  return slot === null ? null : createPortal(children, slot);
}

/** Actions of a part of the page, at the toolbar's end after the page's own. */
export function PageActions({ children }: { readonly children: ReactNode }) {
  const slot = use(PageSlots).actions;
  return slot === null ? null : createPortal(children, slot);
}

/** Views of what the page shows, such as tabs, shown in the page header after the breadcrumbs. */
export function PageViews({ children }: { readonly children: ReactNode }) {
  const slot = use(PageSlots).views;
  return slot === null ? null : createPortal(children, slot);
}
