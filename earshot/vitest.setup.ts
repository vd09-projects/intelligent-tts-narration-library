import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

// jsdom does not implement HTMLMediaElement playback. Stub the methods the audio
// hook calls so component tests can drive play/pause without a real audio device.
// The <audio> error path is exercised by dispatching the native "error" event in
// tests, which jsdom supports.
if (typeof window !== "undefined" && window.HTMLMediaElement) {
  window.HTMLMediaElement.prototype.play = function play() {
    return Promise.resolve();
  };
  window.HTMLMediaElement.prototype.pause = function pause() {
    /* no-op in jsdom */
  };
  window.HTMLMediaElement.prototype.load = function load() {
    /* no-op in jsdom */
  };
}

afterEach(() => {
  cleanup();
});
