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

(cd "$(dirname "$0")" && go run ./cmd/lorenz) | ffmpeg -f f32le -ar 48000 -ac 2 -i - $OUTPUT_ARGS

echo "Done!"
