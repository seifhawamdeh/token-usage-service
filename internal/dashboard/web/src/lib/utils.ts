import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export const fmt = {
  int(n: number | null) {
    if (n == null) return "—";
    return new Intl.NumberFormat("en-US").format(Math.round(n));
  },
  usd(n: number | null) {
    if (n == null) return "—";
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
      maximumFractionDigits: 2,
    }).format(n);
  },
  compact(n: number | null) {
    if (n == null) return "—";
    return new Intl.NumberFormat("en-US", {
      notation: "compact",
      maximumFractionDigits: 1,
    }).format(n);
  },
  day(s: string | null) {
    if (!s) return "—";
    return s.slice(5); // MM-DD
  },
  when(s: string | null) {
    if (!s) return "—";
    const d = new Date(s);
    if (isNaN(d.getTime())) return s;
    // Add 3 hours for GMT+3
    d.setTime(d.getTime() + (3 * 60 * 60 * 1000));
    return d.toISOString().replace("T", " ").replace(/\.\d+Z$/, "");
  },
};
