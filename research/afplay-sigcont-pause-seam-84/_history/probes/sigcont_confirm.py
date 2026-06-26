#!/usr/bin/env python3
"""Decisive test: does SIGSTOP on afplay actually pause AUDIBLE playback,
or does coreaudiod keep draining the already-enqueued buffer while the
afplay process is frozen?

Logic: if audio truly pauses for the freeze window H, total wall time
~= baseline + H. If audio keeps playing (buffer drain), total ~= baseline.
"""
import os
import signal
import subprocess
import time

CLIP = "/tmp/sigcont_probe_clip.wav"


def baseline():
    t0 = time.monotonic()
    subprocess.run(["afplay", CLIP])
    return time.monotonic() - t0


def freeze_run(stop_after, hold):
    t0 = time.monotonic()
    p = subprocess.Popen(["afplay", CLIP])
    time.sleep(stop_after)
    os.kill(p.pid, signal.SIGSTOP)
    # During this window: if audio is silent -> true pause.
    # If audio keeps playing -> buffer drain (no real pause).
    time.sleep(hold)
    os.kill(p.pid, signal.SIGCONT)
    p.wait()
    return time.monotonic() - t0


def main():
    print("Measuring baseline (no signals, full clip)...")
    b = baseline()
    print(f"  baseline total = {b:.2f}s")

    for stop_after, hold in [(2.0, 6.0), (1.0, 8.0)]:
        print(f"\nFreeze run: stop_after={stop_after}s hold={hold}s")
        t = freeze_run(stop_after, hold)
        added = t - b
        print(f"  total = {t:.2f}s   (freeze window held = {hold:.1f}s)")
        print(f"  added over baseline = {added:.2f}s")
        if added >= hold * 0.7:
            print(f"  => AUDIO PAUSED: freeze added ~the full hold -> "
                  f"true pause behavior")
        elif added <= hold * 0.3:
            print(f"  => AUDIO DID NOT PAUSE: freeze added almost nothing -> "
                  f"coreaudiod kept draining buffer; SIGSTOP did not silence "
                  f"audible playback")
        else:
            print(f"  => PARTIAL: freeze added {added:.2f}s of {hold:.1f}s -> "
                  f"only the un-enqueued tail paused; buffered audio kept "
                  f"playing")


if __name__ == "__main__":
    main()
