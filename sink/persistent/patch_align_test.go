package persistent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// TestFrameAlignPCM covers the pad / trim / no-op branches of the helper that
// keeps a patched block's byte length equal to its manifest ms-derived length.
func TestFrameAlignPCM(t *testing.T) {
	format := render.DefaultFormat() // 48 bytes/ms.

	// Already whole-ms (200 ms) → returned unchanged.
	in := framePCM(200, 0x10)
	if got := frameAlignPCM(in, 200, format); len(got) != len(in) {
		t.Errorf("whole-ms: len %d, want %d", len(got), len(in))
	}

	// 16 bytes past 200 ms, durMs rounds to 200 → trim back to 9600.
	long := append(framePCM(200, 0x20), make([]byte, 16)...)
	if got := frameAlignPCM(long, 200, format); len(got) != 200*bytesPerMs {
		t.Errorf("trim: len %d, want %d", len(got), 200*bytesPerMs)
	}

	// 16 bytes short of 200 ms, durMs 200 → pad up to 9600, tail zero.
	short := framePCM(200, 0x30)[:200*bytesPerMs-16]
	got := frameAlignPCM(short, 200, format)
	if len(got) != 200*bytesPerMs {
		t.Fatalf("pad: len %d, want %d", len(got), 200*bytesPerMs)
	}
	for i := len(short); i < len(got); i++ {
		if got[i] != 0 {
			t.Errorf("pad byte %d non-zero (%d)", i, got[i])
		}
	}
}

// TestPatchBlock_NonWholeMsTarget_SecondPatchSucceeds is the regression for the
// reported "Couldn't re-narrate" bug. A re-render whose PCM is not a whole-ms
// multiple (exactly what raw Kokoro emits) used to leave the container's
// on-disk length disagreeing with its manifest-derived length, so the NEXT
// patch's F1 container-vs-manifest check refused the uncorrupted file with
// ErrContainerMismatch. With frame alignment the container stays exact and the
// second patch succeeds.
func TestPatchBlock_NonWholeMsTarget_SecondPatchSucceeds(t *testing.T) {
	outDir, p, _ := seededOutDir(t)
	format := render.DefaultFormat()

	// First patch: target b1 with 16 bytes past 200 ms of PCM.
	fresh := append(framePCM(200, 0xC0), make([]byte, 16)...)
	if _, err := PatchBlock(context.Background(), outDir, p, rerender(t, "b1", fresh), "b1"); err != nil {
		t.Fatalf("first patch: %v", err)
	}

	// Container must be self-consistent now: derived total == on-disk length.
	m, err := readManifest(filepath.Join(outDir, manifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	_, total := deriveBlockLayout(m.Blocks, format)
	if got := len(containerPCMOf(t, outDir)); got != total {
		t.Fatalf("container/manifest drift after first patch: on-disk %d derived %d", got, total)
	}

	// Second patch on a different block must NOT refuse with ErrContainerMismatch.
	fresh2 := append(framePCM(100, 0xD0), make([]byte, 32)...)
	if _, err := PatchBlock(context.Background(), outDir, p, rerender(t, "b0", fresh2), "b0"); err != nil {
		t.Fatalf("second patch refused (the reported bug): %v", err)
	}
}
