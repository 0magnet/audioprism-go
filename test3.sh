#!/bin/bash
# Test script with IN-PHASE signals (should create diagonal line on X-Y scope)
# If sin/cos currently shows diagonal, and sin/sin also shows diagonal, 
# then the issue is in display/capture, not in audio mixing

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

echo "=== IN-PHASE TEST PATTERNS ==="
echo "These should produce DIAGONAL LINES on X-Y scope"
echo ""

# Diagonal line - both channels same (sin/sin)
echo "1. Diagonal (sin/sin - both channels identical)"
ffmpeg -f lavfi -i "aevalsrc=sin(2*PI*440*t)|sin(2*PI*440*t):d=5:s=48000:c=stereo" $OUTPUT_ARGS

# Another diagonal - both cos
echo "2. Diagonal (cos/cos - both channels identical)"
ffmpeg -f lavfi -i "aevalsrc=cos(2*PI*440*t)|cos(2*PI*440*t):d=5:s=48000:c=stereo" $OUTPUT_ARGS

# Diagonal with different amplitudes (should be tilted diagonal)
echo "3. Tilted diagonal (sin * 1.0 | sin * 0.5)"
ffmpeg -f lavfi -i "aevalsrc=sin(2*PI*440*t)|sin(2*PI*440*t)*0.5:d=5:s=48000:c=stereo" $OUTPUT_ARGS

# Now test what SHOULD be a circle (sin/cos - 90° phase)
echo "4. Circle (sin/cos - should be CIRCULAR if working correctly)"
ffmpeg -f lavfi -i "aevalsrc=sin(2*PI*440*t)|cos(2*PI*440*t):d=5:s=48000:c=stereo" $OUTPUT_ARGS

echo ""
echo "=== TEST COMPLETE ==="
echo "If patterns 1-3 look the same as pattern 4, there's a display/capture issue"
echo "If pattern 4 looks different (circular), then it's working correctly"
