#!/bin/bash
# Simple tones
#speaker-test -t sine -f 440 -l 1        # 440Hz tone
#speaker-test -t sine -f 1000 -l 1       # 1kHz tone
#speaker-test -t pink -l 1               # Pink noise
set -x
# Force stereo and 48kHz for compatibility with both devices
# Simple tones (both default + USB)
ffmpeg -f lavfi -i "sine=f=440:d=5:r=48000" -filter_complex "[0:a]asplit=2[a1][a2];[a1]pan=stereo|c0=c0|c1=c0[out1];
[a2]pan=stereo|c0=c0|c1=c0[out2]" -map "[out1]" -f alsa default -map "[out2]" -f alsa hw:1,0
ffmpeg -f lavfi -i "sine=f=1000:d=5:r=48000" -filter_complex "[0:a]asplit=2[a1][a2];[a1]pan=stereo|c0=c0|
c1=c0[out1];[a2]pan=stereo|c0=c0|c1=c0[out2]" -map "[out1]" -f alsa default -map "[out2]" -f alsa hw:1,0

# Noise
ffmpeg -f lavfi -i "anoisesrc=d=5:c=pink:r=48000" -filter_complex "[0:a]asplit=2[a1][a2];[a1]pan=stereo|c0=c0|
c1=c0[out1];[a2]pan=stereo|c0=c0|c1=c0[out2]" -map "[out1]" -f alsa default -map "[out2]" -f alsa hw:1,0
ffmpeg -f lavfi -i "anoisesrc=d=5:c=white:r=48000" -filter_complex "[0:a]asplit=2[a1][a2];[a1]pan=stereo|c0=c0|
c1=c0[out1];[a2]pan=stereo|c0=c0|c1=c0[out2]" -map "[out1]" -f alsa default -map "[out2]" -f alsa hw:1,0

# Frequency sweep (100Hz to 5kHz over 10 seconds)
ffmpeg -f lavfi -i "aevalsrc=sin(2*PI*(100+490*t/10)*t):d=10:s=48000:c=stereo" -f alsa default -f alsa hw:1,0

# Multi-frequency chord (A440 + E660 + C523)
ffmpeg -f lavfi -i "aevalsrc=sin(2*PI*440*t)+sin(2*PI*660*t)+sin(2*PI*523.25*t):d=5:s=48000:c=stereo" -f alsa
default -f alsa hw:1,0

# Square wave (rich harmonics)
ffmpeg -f lavfi -i "aevalsrc=sin(2*PI*440*t)+0.3*sin(6*PI*440*t)+0.2*sin(10*PI*440*t):d=5:s=48000:c=stereo" -f alsa
default -f alsa hw:1,0

# Exponential sweep
ffmpeg -f lavfi -i "aevalsrc=sin(2*PI*100*exp(t/2)*t):d=10:s=48000:c=stereo" -f alsa default -f alsa hw:1,0
set +x
