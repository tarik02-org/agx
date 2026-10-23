import { cn } from "cn";
import { createContext, Fragment, type ReactElement, type ReactNode, use, useState } from "react";
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
import { pageTitle } from "~/lib/title.ts";

/** Where parts of a page, such as an editor, put their actions: the page header. */
const PageActionsSlot = createContext<HTMLElement | null>(null);

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
  const [slot, setSlot] = useState<HTMLElement | null>(null);
  const width = cn("mx-auto w-full max-w-5xl px-4", className);
  return (
    <>
      <title>{pageTitle(...title)}</title>
      <header className="sticky top-0 z-10 flex items-center border-b bg-background">
        {/* At the sidebar's edge, beside what it hides; an equal gap on the right keeps the
            header's content centered over the page's. */}
        <div className="flex w-10 shrink-0 justify-center">
          <SidebarTrigger />
        </div>
        <div className={cn(width, "flex h-11 min-w-0 items-center gap-2")}>
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
          <div ref={setSlot} className="ml-auto flex shrink-0 items-center gap-2">
            {actions}
          </div>
        </div>
        <div className="w-10 shrink-0" />
      </header>
      <PageActionsSlot value={slot}>
        <div
          data-slot="page"
          className={cn(width, "flex flex-col gap-2 py-3", fill && "min-h-0 flex-1")}
        >
          {children}
        </div>
      </PageActionsSlot>
    </>
  );
}

/** Actions of a part of the page, shown in the page header after the page's own. */
export function PageActions({ children }: { readonly children: ReactNode }) {
  const slot = use(PageActionsSlot);
  return slot === null ? null : createPortal(children, slot);
}
