# X-Y Oscilloscope (Ebiten)

X-Y scope visualization for stereo audio patterns using Ebiten game engine.

## Usage

```bash
# Build standalone binary
cd xy
go build .
./xy

# With logging
./xy --log audio_capture.log
```

## Why Standalone?

Ebiten uses GLFW, which conflicts with other GUI frameworks (Fyne, Cogentcore) in the main audioprism-go binary. This standalone version provides the best visual quality without compilation conflicts.

## Features

- **Real-time stereo visualization**: Left channel = X axis, Right channel = Y axis
- **Classic oscilloscope aesthetic**: Green fade trails on black background
- **Interactive zoom**: +/- keys to adjust scale
- **Stereo audio capture**: Correctly captures L/R channels via PulseAudio/PipeWire
- **800x800 window**: Optimized for clear pattern visibility
- **3000 point trail**: Smooth visualization of audio patterns
- **60 FPS rendering**: Smooth updates via Ebiten game loop

## Controls

- **+/=** : Increase zoom (scale up)
- **-** : Decrease zoom (scale down)
- **ESC** : Exit

## Test Patterns

Use the test scripts to generate various patterns:

```bash
# From repo root
./test1.sh  # Lissajous patterns (circles, figure-8s, noise, sweeps)
./test2.sh  # Lorenz attractor (chaotic patterns)
./test3.sh  # In-phase test patterns (diagonal lines)
```

## Implementation Details

- **GUI Framework**: Ebiten v2 (game engine, excellent performance)
- **Audio Capture**: PulseAudio/PipeWire via `github.com/jfreymuth/pulse`
- **Stereo Configuration**: Explicit `RecordChannels` with L/R channel map
- **Sample Rate**: 48kHz
- **Trail Length**: 3000 samples (~62ms at 48kHz)
- **Scale**: Default 300.0, adjustable with +/- keys

## Technical Notes

### Stereo Channel Fix

Critical for proper stereo capture:

```go
pulse.RecordChannels(proto.ChannelMap{proto.ChannelLeft, proto.ChannelRight})
```

Without this, the pulse library defaults to mono recording, causing all patterns to appear as diagonal lines.

### Performance

Ebiten provides superior rendering performance compared to Fyne's raster approach, resulting in:
- Smoother trails
- More responsive zoom
- Better fade effects
- Lower CPU usage

### Build

```bash
go build .
```

No special build tags needed - Ebiten works standalone.
