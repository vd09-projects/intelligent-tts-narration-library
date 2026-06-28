// LiveRegion.tsx — the single polite aria-live status node. Mounted once in the
// shell at load so assistive tech is already observing it before any message.

import { useAnnouncer } from "../state/Announcer";

export function LiveRegion() {
  const { politeMessage } = useAnnouncer();
  return (
    <div aria-live="polite" aria-atomic="true" className="visually-hidden" data-testid="live-region">
      {politeMessage}
    </div>
  );
}
