#!/usr/bin/env python3
"""Placeholder sound clips and music loops, synthesised rather than recorded.

python3 tools/sounds.py      (needs numpy, lame and espeak-ng)

A recording at art/sounds/<name>.mp3 is copied over its synthesised clip.
"""

import pathlib
import shutil
import subprocess
import sys
import tempfile
import wave

import numpy as np

ROOT = pathlib.Path(__file__).resolve().parent.parent
OUT = ROOT / "client" / "public" / "sounds"
ART = ROOT / "art" / "sounds"
SR = 44100
rng = np.random.default_rng(1)


def t(dur):
    return np.arange(int(dur * SR)) / SR


def env(dur, attack=0.005, decay=None):
    """Linear attack, then exponential fall to silence at dur."""
    x = t(dur)
    a = np.minimum(x / attack, 1)
    return a * np.exp(-x / (decay or dur / 5))


def osc(f0, f1, dur, wave_="sine"):
    """A tone sweeping exponentially from f0 to f1 Hz."""
    f = f0 * (f1 / f0) ** (t(dur) / dur)
    ph = np.cumsum(f) / SR % 1
    return {
        "sine": np.sin(2 * np.pi * ph),
        "square": np.sign(np.sin(2 * np.pi * ph)) * 0.6,
        "saw": 2 * ph - 1,
        "tri": 2 * np.abs(2 * ph - 1) - 1,
    }[wave_]


def noise(dur, smooth=1):
    """White noise, low-passed by a box filter smooth samples wide."""
    n = rng.uniform(-1, 1, int(dur * SR))
    return np.convolve(n, np.ones(smooth) / smooth, "same") * np.sqrt(smooth) if smooth > 1 else n


def mix(*parts):
    out = np.zeros(max(len(p) for p in parts))
    for p in parts:
        out[: len(p)] += p
    return out


def seq(*parts):
    return np.concatenate(parts)


def voice(text, pitch=30, slow=1.15):
    """espeak-ng, slowed and lowered by slow, with an arena echo."""
    with tempfile.NamedTemporaryFile(suffix=".wav") as f:
        subprocess.run(["espeak-ng", "-s", "130", "-p", str(pitch), "-w", f.name, text], check=True)
        with wave.open(f.name) as w:
            rate, raw = w.getframerate(), w.readframes(w.getnframes())
    x = np.frombuffer(raw, np.int16) / 32768
    x = np.interp(np.arange(0, len(x), rate / SR / slow), np.arange(len(x)), x)
    d = int(0.11 * SR)
    out = np.zeros(len(x) + 4 * d)
    for k in range(4):
        out[k * d : k * d + len(x)] += x * 0.45**k
    return out


def note(m):
    return 440 * 2 ** ((m - 69) / 12)


def clips():
    hit = mix(osc(160, 45, 0.14) * env(0.14), noise(0.08, 6) * env(0.08) * 0.8)
    return {
        # The shell.
        "select": osc(900, 1200, 0.04, "tri") * env(0.04) * 0.5,
        "confirm": seq(osc(660, 660, 0.06, "tri") * env(0.06), osc(990, 990, 0.16, "tri") * env(0.16)) * 0.6,
        "back": osc(520, 300, 0.1, "tri") * env(0.1) * 0.5,
        "found": mix(
            seq(*[osc(note(m), note(m), 0.07, "square") * env(0.07) for m in (57, 60, 64, 69, 72, 76)]) * 0.4,
            np.concatenate([np.zeros(int(0.35 * SR)), voice("You found a new warrior!", 25)]),
        ),
        "win": seq(*[osc(note(m), note(m), d, "square") * env(d, decay=d) for m, d in
                     ((60, 0.12), (64, 0.12), (67, 0.12), (72, 0.5))]) * 0.5,
        "lose": seq(*[osc(note(m), note(m) * 0.97, d, "tri") * env(d, decay=d) for m, d in
                      ((64, 0.25), (63, 0.25), (62, 0.25), (59, 0.8))]) * 0.6,
        # The match.
        "hit": hit,
        "block": mix(noise(0.05, 2) * env(0.05) * 0.5, osc(1200, 800, 0.06, "tri") * env(0.06) * 0.5),
        "throw": seq(noise(0.18, 12) * np.hanning(int(0.18 * SR)) * 0.6, hit),
        "knockdown": mix(osc(110, 30, 0.4) * env(0.4, decay=0.12), noise(0.25, 30) * env(0.25) * 0.7),
        "super": mix(osc(150, 1800, 0.7, "saw") * np.hanning(int(0.7 * SR)) * 0.4,
                     osc(300, 3600, 0.7, "square") * np.hanning(int(0.7 * SR)) * 0.15),
        "swing": noise(0.13, 18) * np.hanning(int(0.13 * SR)) * 0.35,
        "fireball": mix(osc(220, 660, 0.35, "saw") * env(0.35, 0.03) * 0.3,
                        noise(0.35, 10) * env(0.35, 0.05) * 0.4),
        "jump": osc(280, 560, 0.08) * env(0.08) * 0.3,
        "land": mix(osc(120, 60, 0.08) * env(0.08) * 0.5, noise(0.05, 20) * env(0.05) * 0.3),
        "ko": mix(osc(80, 30, 1.0) * env(1.0, decay=0.3) * 0.6, voice("K. O.", 15, 1.3)),
        "fight": mix(osc(note(45), note(45), 0.6, "saw") * env(0.6) * 0.3, voice("Fight!", 20, 1.2)),
    }


def loop(bpm, bars, chords, layers):
    """A seamless loop: whatever rings past the end wraps round to the start."""
    beat = 60 / bpm
    buf = np.zeros(int(bars * 4 * beat * SR))

    def add(at, sig):
        i = (int(at * SR) + np.arange(len(sig))) % len(buf)
        np.add.at(buf, i, sig)

    for bar in range(bars):
        layers(add, bar * 4 * beat, beat, chords[bar % len(chords)])
    return buf


def menu():
    def layers(add, at, beat, chord):
        root = chord[0]
        for m in chord:  # pad: two detuned saws, slow in and out
            pad = (osc(note(m), note(m), 4 * beat, "saw") + osc(note(m) * 1.004, note(m) * 1.004, 4 * beat, "saw"))
            add(at, np.convolve(pad, np.ones(30) / 30, "same") * np.hanning(len(pad)) * 0.06)
        for k in range(8):  # arpeggio in eighths
            m = (chord + [chord[1] + 12])[[0, 1, 2, 3, 2, 1, 2, 3][k]] + 12
            add(at + k * beat / 2, osc(note(m), note(m), beat / 2, "tri") * env(beat / 2) * 0.12)
        for k in (0, 2):
            add(at + k * beat, osc(note(root - 12), note(root - 12), beat * 1.5) * env(beat * 1.5) * 0.25)
        add(at, osc(90, 40, 0.2) * env(0.2) * 0.3)

    return loop(84, 8, [[57, 60, 64], [53, 57, 60], [48, 52, 55], [55, 59, 62]], layers)


def match():
    def layers(add, at, beat, chord):
        root = chord[0]
        for k in range(8):  # bass in eighths, root and octave
            m = root - 12 + (12 if k % 2 else 0)
            add(at + k * beat / 2, osc(note(m), note(m), beat / 2, "square") * env(beat / 2, decay=0.08) * 0.18)
        for k in range(16):  # lead arpeggio in sixteenths
            m = chord[[0, 1, 2, 1][k % 4]] + 12 + (12 if k >= 8 else 0)
            add(at + k * beat / 4, osc(note(m), note(m), beat / 4, "square") * env(beat / 4, decay=0.04) * 0.07)
        for k in range(4):  # drums
            if k % 2 == 0:
                add(at + k * beat, osc(150, 40, 0.15) * env(0.15) * 0.6)
            else:
                add(at + k * beat, noise(0.12, 3) * env(0.12, decay=0.04) * 0.35)
        for k in range(8):
            add(at + k * beat / 2, noise(0.03) * env(0.03, decay=0.008) * 0.12)

    am, f, g, e = [57, 60, 64], [53, 57, 60], [55, 59, 62], [52, 56, 59]
    return loop(150, 16, [am, am, f, g, am, am, f, e], layers)


def write(name, x, peak):
    x = x / np.max(np.abs(x)) * peak
    with tempfile.NamedTemporaryFile(suffix=".wav") as f:
        with wave.open(f.name, "wb") as w:
            w.setnchannels(1), w.setsampwidth(2), w.setframerate(SR)
            w.writeframes((x * 32767).astype(np.int16).tobytes())
        subprocess.run(["lame", "--quiet", "-V4", f.name, str(OUT / f"{name}.mp3")], check=True)


def main():
    for tool in ("lame", "espeak-ng"):
        if not shutil.which(tool):
            sys.exit(f"sounds.py needs {tool}")
    # Loudness is set inside each clip; the peak only stops clipping.
    for name, x in clips().items():
        write(name, x, 0.9 * min(1, np.max(np.abs(x)) / 0.6))
    write("menu", menu(), 0.7)
    write("match", match(), 0.7)
    for rec in ART.glob("*.mp3"):
        shutil.copy(rec, OUT / rec.name)
    print(f"wrote {len(list(OUT.glob('*.mp3')))} files to {OUT.relative_to(ROOT)}")


if __name__ == "__main__":
    main()
