#!/usr/bin/env python3
"""By-ear seam probe for ticket #84.

Exercises the exact afplay seam the listen-path controller uses
(`afplay <wav>`), sends SIGSTOP then SIGCONT to the live PID mid-playback,
and measures whether playback truly resumes from the pause point or
restarts. The audible "is the resume clean" judgment is left to a human
ear — this only gathers the measurable evidence.
"""
import os
import signal
import subprocess
import sys
import time

CLIP = "/tmp/sigcont_probe_clip.wav"
CLIP_DUR = 10.069  # afinfo estimated duration, seconds
STOP_AFTER = 3.0   # play this long before SIGSTOP
HOLD = 4.0         # freeze duration between SIGSTOP and SIGCONT


def ps_state(pid):
    try:
        out = subprocess.check_output(
            ["ps", "-o", "state=", "-p", str(pid)], text=True
        ).strip()
        return out or "(gone)"
    except subprocess.CalledProcessError:
        return "(gone)"


def run(run_no):
    print(f"\n=== RUN {run_no} ===")
    t0 = time.monotonic()
    proc = subprocess.Popen(["afplay", CLIP])
    pid = proc.pid
    print(f"  afplay PID={pid}  state={ps_state(pid)}  (audio playing)")

    time.sleep(STOP_AFTER)
    t_stop = time.monotonic()
    os.kill(pid, signal.SIGSTOP)
    time.sleep(0.15)  # let the kernel settle the state
    st = ps_state(pid)
    played_before = t_stop - t0
    print(f"  +{played_before:5.2f}s  SIGSTOP sent  -> state={st}  "
          f"(expect 'T' = stopped; audio should be SILENT now)")
    stopped_ok = st.startswith("T")

    time.sleep(HOLD)
    held_state = ps_state(pid)
    print(f"  ...held {HOLD:.1f}s frozen, state still={held_state}")

    t_cont = time.monotonic()
    os.kill(pid, signal.SIGCONT)
    time.sleep(0.15)
    st2 = ps_state(pid)
    print(f"  +{t_cont - t0:5.2f}s  SIGCONT sent  -> state={st2}  "
          f"(expect 'S'/'R' = running; audio should RESUME now)")
    resumed_ok = not st2.startswith("T") and st2 != "(gone)"

    proc.wait()
    t_exit = time.monotonic()
    after_cont = t_exit - t_cont
    total = t_exit - t0

    remaining_if_resume = CLIP_DUR - played_before
    remaining_if_restart = CLIP_DUR

    print(f"  +{total:5.2f}s  afplay exited (rc={proc.returncode})")
    print(f"  --- timing analysis ---")
    print(f"  played before stop          : {played_before:5.2f}s")
    print(f"  frozen (stop->cont)         : {t_cont - t_stop:5.2f}s")
    print(f"  playback AFTER cont->exit   : {after_cont:5.2f}s")
    print(f"  expected remaining if TRUE RESUME : {remaining_if_resume:5.2f}s")
    print(f"  expected remaining if RESTART@0   : {remaining_if_restart:5.2f}s")
    d_resume = abs(after_cont - remaining_if_resume)
    d_restart = abs(after_cont - remaining_if_restart)
    verdict = "TRUE-RESUME" if d_resume < d_restart else "RESTART-FROM-ZERO"
    print(f"  |after_cont - resume|={d_resume:.2f}  "
          f"|after_cont - restart|={d_restart:.2f}  => {verdict}")
    return {
        "stopped_ok": stopped_ok,
        "resumed_ok": resumed_ok,
        "played_before": played_before,
        "after_cont": after_cont,
        "remaining_if_resume": remaining_if_resume,
        "verdict": verdict,
        "rc": proc.returncode,
    }


def main():
    if not os.path.exists(CLIP):
        print(f"clip missing: {CLIP}", file=sys.stderr)
        sys.exit(1)
    print(f"clip={CLIP} dur={CLIP_DUR}s  stop_after={STOP_AFTER}s hold={HOLD}s")
    results = [run(1), run(2)]
    print("\n=== SUMMARY ===")
    for i, r in enumerate(results, 1):
        print(f"run{i}: SIGSTOP_freeze={'OK' if r['stopped_ok'] else 'FAIL'}  "
              f"SIGCONT_resume={'OK' if r['resumed_ok'] else 'FAIL'}  "
              f"timing_verdict={r['verdict']}  "
              f"(after_cont={r['after_cont']:.2f}s vs "
              f"resume-expected={r['remaining_if_resume']:.2f}s)  rc={r['rc']}")


if __name__ == "__main__":
    main()
