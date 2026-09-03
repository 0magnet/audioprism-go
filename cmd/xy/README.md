# X-Y Oscilloscope

X-Y scope visualization for stereo audio patterns using Fyne GUI.

## Usage

```bash
# From main binary
go run . xy

# Or build and run
go build .
./audioprism-go xy
```

## Features

- **Real-time stereo visualization**: Left channel = X axis, Right channel = Y axis
- **Classic oscilloscope aesthetic**: Green fade trails on black background
- **Stereo audio capture**: Correctly captures L/R channels via PulseAudio/PipeWire
- **800x800 window**: Optimized for clear pattern visibility
- **3000 point trail**: Smooth visualization of audio patterns

## Patterns

Use the test scripts to generate various patterns:

```bash
# Lissajous patterns and test signals
./test1.sh  # Circles, figure-8s, noise, sweeps

# Lorenz attractor (chaotic patterns)
./test2.sh  

# In-phase test patterns (diagonal lines)
./test3.sh
```

## Implementation

- **GUI Framework**: Fyne (no GLFW conflicts with other audioprism UIs)
- **Audio Capture**: PulseAudio/PipeWire via `github.com/jfreymuth/pulse`
- **Stereo Configuration**: Explicit `RecordChannels` with L/R channel map
- **Sample Rate**: 48kHz
- **Trail Length**: 3000 samples (~62ms at 48kHz)

## Integration

The XY scope is fully integrated as a subcommand alongside the other visualization modes:

- `c` - C.O.R.E. desktop GUI
- `d` - C.O.R.E. web UI  
- `f` - Fyne GUI
- `m` - Gomobile GUI
- `t` - Tcell TUI
- `w` - WASM
- `xy` - X-Y Oscilloscope (this)

## Technical Notes

### Stereo Channel Fix

The critical fix for proper stereo capture was adding explicit channel configuration:

```go
pulse.RecordChannels(proto.ChannelMap{proto.ChannelLeft, proto.ChannelRight})
```

Without this, the pulse library defaults to mono recording, causing all patterns to appear as diagonal lines.

### Why Fyne?

Initially implemented with Ebiten, but Ebiten uses GLFW which conflicts with other GUI frameworks (Fyne, Cogentcore) already in use by audioprism-go. Fyne implementation allows all subcommands to coexist in a single binary.
