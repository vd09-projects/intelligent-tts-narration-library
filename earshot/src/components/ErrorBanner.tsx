// ErrorBanner.tsx — the visible, assertive error node (plan Focus Management +
// Live Region). role="alert" announces assertively; it is focusable (tabIndex
// -1) and receives focus when an error appears so a keyboard user is jumped to
// the actionable info. Fetch errors and audio-load failures carry DISTINCT text.

import { useEffect, useRef } from "react";

export function ErrorBanner({ message }: { message: string | null }) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (message) {
      ref.current?.focus();
    }
  }, [message]);

  if (!message) {
    return null;
  }

  return (
    <div
      ref={ref}
      role="alert"
      tabIndex={-1}
      className="error-banner"
      data-testid="error-banner"
    >
      {message}
    </div>
  );
}
