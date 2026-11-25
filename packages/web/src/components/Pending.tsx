import { useEffect, useState } from "react";

import { Skeleton } from "~/components/ui/skeleton.tsx";

// The daemon is local and usually answers within a frame; a placeholder shown at once would only
// flicker, so it appears once a read takes noticeably long.
const showAfterMs = 200;

/** A loading placeholder that stays hidden for fast reads. */
export function Pending({ className }: { readonly className: string }) {
  const [shown, setShown] = useState(false);
  useEffect(() => {
    const timer = setTimeout(() => setShown(true), showAfterMs);
    return () => clearTimeout(timer);
  }, []);
  return shown ? <Skeleton className={className} /> : <div className={className} aria-busy />;
}
