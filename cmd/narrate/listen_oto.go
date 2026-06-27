// In-process oto v3 player — the real listen engine (issue #101, productionized
// from the #100 spike).
//
// This is the ONLY file in cmd/narrate that imports github.com/ebitengine/oto/v3.
// It supplies the device-bound seams the build-tag-free transport (listen.go)
// drives through interfaces: openOtoContext constructs the one process-wide
// oto.Context and returns a listenPlayer factory; otoPlayer adapts *oto.Player
// to the listenPlayer seam. driveListenWith (listen.go) checks the construction
// error and the bounded ready-wait; runListen never imports oto, so the whole
// transport loop is unit-testable with a fake player and no audio device.
//
// macOS phase one — purego/CoreAudio path, no CGo (CGO_ENABLED=0 builds clean).
package main

import (
	"bytes"

	"github.com/ebitengine/oto/v3"
)

// oto context audio format — Kokoro native: 24 kHz mono int16, NO resampling at
// our layer (CLAUDE.md). oto handles device resampling internally; the by-ear
// /verify gate confirms that path sounds acceptable (acceptance criterion 3).
const (
	otoSampleRate   = 24000
	otoChannelCount = 1
)

// openOtoContext constructs the single process-wide oto.Context (oto permits
// exactly one) and returns a listenPlayer factory bound to it, the context's
// ready channel, and any construction error. It is the otoContextOpener the
// production driveListen passes to driveListenWith — the sole oto entry point.
//
// On a construction error the factory and ready channel are nil; driveListenWith
// routes that through listenCleanup and surfaces an honest, actionable error.
func openOtoContext() (newPlayer func(pcm []byte) listenPlayer, ready <-chan struct{}, err error) {
	otoCtx, readyCh, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   otoSampleRate,
		ChannelCount: otoChannelCount,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		return nil, nil, err
	}
	factory := func(pcm []byte) listenPlayer {
		// bytes.NewReader gives an in-memory io.ReadSeeker with no fd, so oto's
		// finalizer-driven teardown can never read a closed descriptor. The full
		// PCM buffer is already in hand (loadBlockPCM read it before this call).
		return &otoPlayer{p: otoCtx.NewPlayer(bytes.NewReader(pcm))}
	}
	return factory, readyCh, nil
}

// otoPlayer adapts *oto.Player to the listenPlayer seam. It exposes exactly
// Play/Pause/IsPlaying/Err — and deliberately NOT Close(): in oto v3.4
// Player.Close() is a documented no-op (teardown is finalizer/GC-driven) that
// trips SA1019, so teardown is Pause()+drop-the-reference in the loop, not an
// explicit release here.
type otoPlayer struct {
	p *oto.Player
}

func (o *otoPlayer) Play()           { o.p.Play() }
func (o *otoPlayer) Pause()          { o.p.Pause() }
func (o *otoPlayer) IsPlaying() bool { return o.p.IsPlaying() }
func (o *otoPlayer) Err() error      { return o.p.Err() }
