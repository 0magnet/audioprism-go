#!/bin/bash
# X-Y scope optimized audio - different content per channel for interesting patterns
# Outputs to default + any USB sound cards found

# Detect USB sound cards (look for "USB Audio" or any card that's not card 0)
USB_CARDS=$(aplay -l 2>/dev/null | grep -i "usb audio\|AB13X" | head -1 | sed -n 's/card \([0-9]\+\):.*/hw:\1,0/p')

# If no USB Audio found, try to find any non-PCH audio card (card 1+)
if [ -z "$USB_CARDS" ]; then
    USB_CARDS=$(aplay -l 2>/dev/null | grep "^card [1-9]" | head -1 | sed -n 's/card \([0-9]\+\):.*/hw:\1,0/p')
fi

# Build output arguments
if [ -n "$USB_CARDS" ]; then
    OUTPUT_ARGS="-f alsa default -f alsa $USB_CARDS"
    echo "Outputting to: default + $USB_CARDS"
else
    OUTPUT_ARGS="-f alsa default"
    echo "Outputting to: default only"
fi

# Circle (90° phase shift - same frequency, phase offset)
ffmpeg -f lavfi -i "aevalsrc=sin(2*PI*440*t)|cos(2*PI*440*t):d=5:s=48000:c=stereo" $OUTPUT_ARGS
# Lissajous 3:2 (440Hz left, 660Hz right)
ffmpeg -f lavfi -i "aevalsrc=sin(2*PI*440*t)|sin(2*PI*660*t):d=5:s=48000:c=stereo" $OUTPUT_ARGS
# Lissajous 2:1 (440Hz left, 880Hz right - figure-8 pattern)
ffmpeg -f lavfi -i "aevalsrc=sin(2*PI*440*t)|sin(2*PI*880*t):d=5:s=48000:c=stereo" $OUTPUT_ARGS
# Lissajous 5:4 (400Hz left, 500Hz right)
ffmpeg -f lavfi -i "aevalsrc=sin(2*PI*400*t)|sin(2*PI*500*t):d=5:s=48000:c=stereo" $OUTPUT_ARGS
# Pink noise X, White noise Y
if [ -n "$USB_CARDS" ]; then
    ffmpeg -f lavfi -i "anoisesrc=d=5:c=pink:r=48000" -f lavfi -i "anoisesrc=d=5:c=white:r=48000" -filter_complex "[0:a][1:a]amerge=inputs=2,pan=stereo|c0=c0|c1=c1[a];[a]asplit=2[out1][out2]" -map "[out1]" -f alsa default -map "[out2]" -f alsa $USB_CARDS
else
    ffmpeg -f lavfi -i "anoisesrc=d=5:c=pink:r=48000" -f lavfi -i "anoisesrc=d=5:c=white:r=48000" -filter_complex "[0:a][1:a]amerge=inputs=2,pan=stereo|c0=c0|c1=c1[out]" -map "[out]" -f alsa default
fi
# Sweep on X (100-5kHz), constant tone on Y (440Hz)
ffmpeg -f lavfi -i "aevalsrc=sin(2*PI*(100+490*t/10)*t)|sin(2*PI*440*t):d=10:s=48000:c=stereo" $OUTPUT_ARGS
# Chord X (multi-tone), single tone Y
ffmpeg -f lavfi -i "aevalsrc=(sin(2*PI*440*t)+sin(2*PI*660*t)+sin(2*PI*523.25*t))/3|sin(2*PI*440*t):d=5:s=48000:c=stereo" $OUTPUT_ARGS
# Rotating phase sweep (creates spirals)
ffmpeg -f lavfi -i "aevalsrc=sin(2*PI*440*t+2*PI*t/5)|sin(2*PI*440*t):d=10:s=48000:c=stereo" $OUTPUT_ARGS
