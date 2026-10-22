import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

/** Joins class names, letting later Tailwind utilities override earlier ones. */
export const cn = (...inputs: ClassValue[]): string => twMerge(clsx(inputs));
