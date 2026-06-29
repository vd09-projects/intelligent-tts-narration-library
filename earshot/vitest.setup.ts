import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

// jsdom under this config exposes `localStorage` as a bare object with no Storage
// methods (opaque origin). resumeStore (#112) degrades gracefully to a no-op when
// methods are missing, but tests that exercise real persistence need a working
// Storage. Install a minimal in-memory implementation, cleared between tests.
class MemoryStorage implements Storage {
  private map = new Map<string, string>();
  get length() {
    return this.map.size;
  }
  clear() {
    this.map.clear();
  }
  getItem(key: string) {
    return this.map.has(key) ? (this.map.get(key) as string) : null;
  }
  key(index: number) {
    return Array.from(this.map.keys())[index] ?? null;
  }
  removeItem(key: string) {
    this.map.delete(key);
  }
  setItem(key: string, value: string) {
    this.map.set(key, String(value));
  }
}

if (typeof window !== "undefined" && typeof window.localStorage?.setItem !== "function") {
  Object.defineProperty(window, "localStorage", {
    value: new MemoryStorage(),
    configurable: true,
    writable: true,
  });
}

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
