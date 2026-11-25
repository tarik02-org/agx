import { createLink } from "@tanstack/react-router";
import type { VariantProps } from "class-variance-authority";
import { cn } from "cn";
import type { AnchorHTMLAttributes, Ref } from "react";

import { buttonVariants } from "~/components/ui/button.tsx";

interface ButtonAnchorProps
  extends AnchorHTMLAttributes<HTMLAnchorElement>, VariantProps<typeof buttonVariants> {
  readonly ref?: Ref<HTMLAnchorElement>;
}

function ButtonAnchor({ variant, size, className, ...props }: ButtonAnchorProps) {
  return <a {...props} className={cn(buttonVariants({ variant, size }), className)} />;
}

/**
 * Navigation that looks like a button: a real link, so it opens in a new tab with ⌘/Ctrl-click
 * and screen readers announce it as one.
 */
export const ButtonLink = createLink(ButtonAnchor);
