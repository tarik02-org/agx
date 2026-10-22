import type { ComponentProps } from "react";

import { cn } from "~/lib/className.ts";

export function Table({ className, ...props }: ComponentProps<"table">) {
  return <table className={cn("w-full border-collapse text-left", className)} {...props} />;
}

export function TableHead({ className, ...props }: ComponentProps<"th">) {
  return (
    <th
      className={cn(
        "px-3 py-1.5 font-medium text-[11px] text-muted-foreground uppercase tracking-wide",
        className,
      )}
      {...props}
    />
  );
}

export function TableRow({ className, ...props }: ComponentProps<"tr">) {
  return (
    <tr className={cn("border-t first:border-t-0 hover:bg-accent/40", className)} {...props} />
  );
}

export function TableCell({ className, ...props }: ComponentProps<"td">) {
  return <td className={cn("px-3 py-1.5 align-middle", className)} {...props} />;
}
