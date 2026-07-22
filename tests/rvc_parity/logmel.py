"""Shared log-mel extraction for the RVC parity gate (issue #144).

Both sides of the full-pipeline gate — target generation (gen_targets.py, from the
torch-reference WAV) AND comparison (parity_test.py, on the worker's ONNX output) —
import THIS module so the correlation is apples-to-apples (review round-2 #3).

The params below are PINNED. Do not change one side without regenerating the
committed <slug>_logmel_target.npy from the torch reference with the same params,
or the gate compares two different transforms and the corr is meaningless.
"""

from __future__ import annotations

import numpy as np
import librosa

# ── PINNED log-mel params (change only in lockstep with target regeneration) ──
LOGMEL_SR = 40000       # worker output rate (§4: net_g emits 40 kHz)
LOGMEL_N_FFT = 2048
LOGMEL_HOP = 512
LOGMEL_N_MELS = 128
LOGMEL_FMIN = 0.0
LOGMEL_FMAX = 20000.0   # Nyquist for 40 kHz
LOGMEL_POWER = 2.0
LOGMEL_TOP_DB = 80.0


def log_mel(audio: np.ndarray, sr: int = LOGMEL_SR) -> np.ndarray:
    """Return a log-mel spectrogram (dB) with the pinned params.

    audio: float32 mono waveform at `sr` (must equal LOGMEL_SR — no resampling,
    the worker output is already 40 kHz).
    """
    if sr != LOGMEL_SR:
        raise ValueError(f"log_mel expects {LOGMEL_SR} Hz, got {sr}")
    audio = np.ascontiguousarray(audio, dtype=np.float32)
    mel = librosa.feature.melspectrogram(
        y=audio,
        sr=LOGMEL_SR,
        n_fft=LOGMEL_N_FFT,
        hop_length=LOGMEL_HOP,
        n_mels=LOGMEL_N_MELS,
        fmin=LOGMEL_FMIN,
        fmax=LOGMEL_FMAX,
        power=LOGMEL_POWER,
    )
    return librosa.power_to_db(mel, ref=np.max, top_db=LOGMEL_TOP_DB).astype(np.float32)


def logmel_corr(a: np.ndarray, b: np.ndarray) -> float:
    """Pearson correlation of two log-mel spectrograms over the common frame span.

    Trims to the shorter frame count so a ±1-frame length difference (from the
    two independent RNG draws in the RVC path) does not fail the compare.
    """
    n = min(a.shape[1], b.shape[1])
    fa = a[:, :n].reshape(-1).astype(np.float64)
    fb = b[:, :n].reshape(-1).astype(np.float64)
    if fa.size == 0 or fb.size == 0:
        return float("nan")
    return float(np.corrcoef(fa, fb)[0, 1])
