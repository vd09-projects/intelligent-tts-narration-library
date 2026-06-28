// TransportBar.tsx — bottom-fixed transport (plan Step 2). This SHELL ships a
// placeholder transport region (role="toolbar") that hosts the bottom audio
// CONTROL: a native <audio controls> bound to the shared owner's audioRef +
// opaque activeAudioUrl (D4). The native control gives play/pause/seek for free.
//
// The full transport DECK (prev/play-pause/next wiring, block-quantized scrubber,
// return-to-playing, roving tabindex traversal) is OUT OF SCOPE — a follow-on
// transport-interactions ticket. This is intentionally just the audio element.
//
// TODO: replace native <audio controls> with the block-quantized transport deck
// (prev/play-pause/next + scrubber) in the transport-interactions follow-on,
// because the shell only needs whole-clip playback and the deck is out of scope.

import { useNarration } from "../state/NarrationContext";

export function TransportBar() {
  const { audioRef, activeAudioUrl } = useNarration();
  return (
    <div
      role="toolbar"
      aria-label="Playback transport"
      className="transport-bar"
      data-testid="transport-bar"
    >
      {/* The opaque audio_url (D4) is set imperatively on the element by
          useAudio.setSource — the hook is the single source of truth, so no
          declarative <source> child here. No captions: audio output, not video
          (plan Media Accessibility). */}
      <audio
        ref={audioRef}
        className="transport-bar__audio"
        controls
        preload="none"
        data-testid="audio-element"
        aria-label="Narration audio"
      />
      {!activeAudioUrl ? (
        <span className="transport-bar__hint">No audio loaded</span>
      ) : null}
    </div>
  );
}
