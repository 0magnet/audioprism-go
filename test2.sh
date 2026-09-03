#!/bin/bash
# Lorenz attractor for X-Y oscilloscope visualization
# Generates chaotic attractor pattern using the classic Lorenz equations

# Detect USB sound cards
USB_CARDS=$(aplay -l 2>/dev/null | grep -i "usb audio\|AB13X" | head -1 | sed -n 's/card \([0-9]\+\):.*/hw:\1,0/p')
if [ -z "$USB_CARDS" ]; then
    USB_CARDS=$(aplay -l 2>/dev/null | grep "^card [1-9]" | head -1 | sed -n 's/card \([0-9]\+\):.*/hw:\1,0/p')
fi

if [ -n "$USB_CARDS" ]; then
    OUTPUT_ARGS="-f alsa default -f alsa $USB_CARDS"
    echo "Outputting to: default + $USB_CARDS"
else
    OUTPUT_ARGS="-f alsa default"
    echo "Outputting to: default only"
fi

# Generate Lorenz attractor data and play it
# Parameters: sigma=10, rho=28, beta=8/3
# Duration: 30 seconds at 48000 Hz sample rate
echo "Generating Lorenz attractor..."

python3 << 'PYTHON_SCRIPT' | ffmpeg -f f32le -ar 48000 -ac 2 -i - $OUTPUT_ARGS
import struct
import sys

# Lorenz attractor parameters
sigma = 10.0
rho = 28.0
beta = 8.0 / 3.0
dt = 0.0001  # Time step for integration

# Initial conditions
x, y, z = 1.0, 1.0, 1.0

# Audio parameters
sample_rate = 48000
duration = 30  # seconds
samples = sample_rate * duration

# Scale factors to normalize output to roughly -1 to 1 range
x_scale = 0.04
y_scale = 0.03

sample_count = 0
skip = 10  # Only output every Nth point to slow down visualization

for i in range(samples * skip):
    # Runge-Kutta 4th order integration
    dx = sigma * (y - x)
    dy = x * (rho - z) - y
    dz = x * y - beta * z
    
    x += dx * dt
    y += dy * dt
    z += dz * dt
    
    # Output audio sample every 'skip' iterations
    if i % skip == 0:
        # Scale to audio range (-1.0 to 1.0)
        left = x * x_scale
        right = y * y_scale
        
        # Clamp values
        left = max(-1.0, min(1.0, left))
        right = max(-1.0, min(1.0, right))
        
        # Write as 32-bit float stereo
        sys.stdout.buffer.write(struct.pack('ff', left, right))
        
        sample_count += 1
        if sample_count >= samples:
            break

PYTHON_SCRIPT

echo "Done!"
