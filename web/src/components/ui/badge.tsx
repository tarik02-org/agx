import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";

import { cn } from "~/lib/className.ts";

const badgeVariants = cva(
  "inline-flex items-center gap-1 rounded-sm px-1.5 py-px font-medium text-[11px] leading-4 ring-1 ring-inset",
  {
    variants: {
      tone: {
        neutral: "bg-muted text-muted-foreground ring-border",
        success: "bg-success/10 text-success ring-success/25",
        warning: "bg-warning/10 text-warning ring-warning/25",
        destructive: "bg-destructive/10 text-destructive ring-destructive/25",
      },
    },
    defaultVariants: { tone: "neutral" },
  },
);

export interface BadgeProps extends ComponentProps<"span">, VariantProps<typeof badgeVariants> {}

export function Badge({ className, tone, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ tone }), className)} {...props} />;
}
