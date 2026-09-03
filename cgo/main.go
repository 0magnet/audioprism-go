package main

/*
#cgo pkg-config: sdl2 fftw3f libpulse-simple
#cgo LDFLAGS: -lm -lpthread

#include <SDL2/SDL.h>
#include <fftw3.h>
#include <pulse/simple.h>
#include <pulse/error.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <pthread.h>
#include <time.h>
#include <stdio.h>

// ---- Configuration ----
typedef struct {
    int width, height;
    int sample_rate;
    int dft_size;
    float overlap;
    int color_scheme; // 0=heat, 1=blue, 2=grayscale
    int win_type;     // 0=hann, 1=hamming, 2=bartlett, 3=rectangular
    int scale;        // 0=log, 1=linear
    float mag_min, mag_max;
    int fullscreen;
} config_t;

// ---- FFTW3 ----
typedef struct {
    fftwf_plan plan;
    float *windowed;
    fftwf_complex *out;
    int size;
} fft_ctx;

static fft_ctx* fft_create(int size) {
    fft_ctx *ctx = (fft_ctx*)malloc(sizeof(fft_ctx));
    ctx->size = size;
    ctx->windowed = fftwf_alloc_real(size);
    ctx->out = fftwf_alloc_complex(size/2 + 1);
    ctx->plan = fftwf_plan_dft_r2c_1d(size, ctx->windowed, ctx->out, FFTW_MEASURE);
    return ctx;
}

static void fft_destroy(fft_ctx *ctx) {
    fftwf_destroy_plan(ctx->plan);
    fftwf_free(ctx->out);
    fftwf_free(ctx->windowed);
    free(ctx);
}

static void fft_execute(fft_ctx *ctx, float *input, float *magnitudes, int win_type) {
    int N = ctx->size;
    for (int i = 0; i < N; i++) {
        double w;
        switch (win_type) {
        case 1: w = 0.54 - 0.46 * cos(2.0 * M_PI * i / (N - 1)); break;
        case 2: w = 1.0 - fabs((double)i - (double)(N-1)/2.0) / ((double)(N-1)/2.0); break;
        case 3: w = 1.0; break;
        default: w = 0.5 * (1.0 - cos(2.0 * M_PI * i / (N - 1))); break;
        }
        ctx->windowed[i] = input[i] * (float)w;
    }
    fftwf_execute(ctx->plan);
    int nbins = N/2 + 1;
    for (int i = 0; i < nbins; i++) {
        float re = ctx->out[i][0];
        float im = ctx->out[i][1];
        magnitudes[i] = sqrtf(re*re + im*im);
    }
}

// ---- Color mapping ----
static inline float clampf(float v, float lo, float hi) {
    return v < lo ? lo : (v > hi ? hi : v);
}
static inline float normf(float v, float lo, float hi) {
    return (clampf(v, lo, hi) - lo) / (hi - lo);
}

static uint32_t color_heat(float v) {
    uint8_t r=0,g=0,b=0;
    if (v < 0.2f) { b = (uint8_t)(255.0f * normf(v, 0.0f, 0.2f)); }
    else if (v < 0.4f) { float n=normf(v,0.2f,0.4f); g=(uint8_t)(255.0f*n); b=(uint8_t)(255.0f*(1.0f-n)); }
    else if (v < 0.6f) { r=(uint8_t)(255.0f*normf(v,0.4f,0.6f)); g=255; }
    else if (v < 0.8f) { r=255; g=(uint8_t)(255.0f*(1.0f-normf(v,0.6f,0.8f))); }
    else { float n=normf(v,0.8f,1.0f); r=255; g=(uint8_t)(255.0f*n); b=(uint8_t)(255.0f*n); }
    return ((uint32_t)r<<16)|((uint32_t)g<<8)|(uint32_t)b;
}

static uint32_t color_blue(float v) {
    uint8_t r=0,g=0,b=0;
    if (v < 0.5f) { b=(uint8_t)(255.0f*normf(v,0.0f,0.5f)); }
    else { float n=normf(v,0.5f,1.0f); r=(uint8_t)(255.0f*n); g=(uint8_t)(255.0f*n); b=255; }
    return ((uint32_t)r<<16)|((uint32_t)g<<8)|(uint32_t)b;
}

static uint32_t color_gray(float v) {
    uint8_t c=(uint8_t)(255.0f*v);
    return ((uint32_t)c<<16)|((uint32_t)c<<8)|(uint32_t)c;
}

static uint32_t mag_to_pixel(float mag, config_t *cfg) {
    if (cfg->scale == 0) mag = 20.0f * log10f(mag);
    float n = normf(mag, cfg->mag_min, cfg->mag_max);
    switch (cfg->color_scheme) {
    case 1: return color_blue(n);
    case 2: return color_gray(n);
    default: return color_heat(n);
    }
}

// ---- Shared state ----
#define HIST_SIZE 2048
#define BUF_HEIGHT 1024

typedef struct {
    // Column history (circular buffer, each column = BUF_HEIGHT pixels)
    uint32_t (*columns)[BUF_HEIGHT]; // heap-allocated [HIST_SIZE][BUF_HEIGHT]
    int hist_index;
    pthread_mutex_t hist_mu;

    // Audio sample buffer
    float *sample_buf;
    int sample_count, sample_cap;
    pthread_mutex_t sample_mu;
    pthread_cond_t sample_cond;

    volatile int running;
    config_t cfg;
    fft_ctx *fft;
} app_state;

// ---- Audio thread ----
static void* audio_thread(void *arg) {
    app_state *s = (app_state*)arg;
    pa_sample_spec ss;
    ss.format = PA_SAMPLE_FLOAT32LE;
    ss.rate = s->cfg.sample_rate;
    ss.channels = 1;

    pa_buffer_attr attr;
    memset(&attr, 0xff, sizeof(attr));
    attr.fragsize = 1024; // match original audioprism

    int error;
    pa_simple *pa = pa_simple_new(NULL, "audioprism-go", PA_STREAM_RECORD,
                                   NULL, "audio in", &ss, NULL, &attr, &error);
    if (!pa) return NULL;

    float buf[128];
    while (s->running) {
        if (pa_simple_read(pa, buf, sizeof(buf), &error) < 0) continue;

        pthread_mutex_lock(&s->sample_mu);
        if (s->sample_count + 128 <= s->sample_cap) {
            memcpy(s->sample_buf + s->sample_count, buf, 128 * sizeof(float));
            s->sample_count += 128;
        }
        pthread_cond_signal(&s->sample_cond);
        pthread_mutex_unlock(&s->sample_mu);
    }

    pa_simple_free(pa);
    return NULL;
}

// ---- Spectrogram thread ----
static void* spectrogram_thread(void *arg) {
    app_state *s = (app_state*)arg;
    int N = s->cfg.dft_size;
    // samplesOverlap = overlap * N (matches original audioprism naming)
    int samples_overlap = (int)(s->cfg.overlap * N);
    int nbins = N/2 + 1;

    float *overlap_buf = (float*)calloc(N, sizeof(float));
    float *magnitudes = (float*)malloc(nbins * sizeof(float));
    float *local_buf = (float*)malloc(s->sample_cap * sizeof(float));
    int local_count = 0;

    // Column rate tracking
    int col_count = 0;
    struct timespec rate_start;
    clock_gettime(CLOCK_MONOTONIC, &rate_start);

    while (s->running) {
        pthread_mutex_lock(&s->sample_mu);
        while (s->sample_count == 0 && s->running)
            pthread_cond_wait(&s->sample_cond, &s->sample_mu);
        int got = s->sample_count;
        if (got > 0) {
            memcpy(local_buf + local_count, s->sample_buf, got * sizeof(float));
            local_count += got;
            s->sample_count = 0;
        }
        pthread_mutex_unlock(&s->sample_mu);

        // Match original: need at least samples_overlap new samples
        while (local_count >= samples_overlap && s->running) {
            // Shift old samples left by samples_overlap
            memmove(overlap_buf, overlap_buf + samples_overlap, (N - samples_overlap) * sizeof(float));
            // Copy samples_overlap new samples to fill the right side
            // But we copy (N - samples_overlap) samples from local_buf to overlap_buf + samples_overlap
            // Wait - original copies (N - samplesOverlap) samples, not samplesOverlap
            // That's because: overlap keeps N-samplesOverlap old + samplesOverlap from shift = N total
            // But we need N-samplesOverlap new samples? No...
            //
            // Original logic:
            //   memmove(overlap, overlap + samplesOverlap, (N - samplesOverlap) * sizeof(float))
            //   memcpy(overlap + samplesOverlap, audioSamples, (N - samplesOverlap) * sizeof(float))
            //   erase samplesOverlap from audioSamples
            //
            // So: shift left by samplesOverlap (keep last N-samplesOverlap old samples at front)
            //     copy N-samplesOverlap NEW samples into position [samplesOverlap..N-1]
            //     consume samplesOverlap from input
            //
            // Wait that doesn't add up. After memmove, overlap[0..N-samplesOverlap-1] has old tail.
            // Then memcpy puts N-samplesOverlap new samples at overlap[samplesOverlap..N-1].
            // But overlap[N-samplesOverlap..samplesOverlap-1] is a gap? No, N-samplesOverlap = samplesOverlap = 512.
            // So overlap[0..511] = old tail, overlap[512..1023] = new samples. That's correct for 50% overlap.
            // But it copies N-samplesOverlap = 512 new samples, and erases samplesOverlap = 512 from input.
            // These are the same number (512), so the step size = samplesOverlap = 512.
            memcpy(overlap_buf + samples_overlap, local_buf, (N - samples_overlap) * sizeof(float));
            memmove(local_buf, local_buf + samples_overlap, (local_count - samples_overlap) * sizeof(float));
            local_count -= samples_overlap;

            fft_execute(s->fft, overlap_buf, magnitudes, s->cfg.win_type);

            // Render column: map nbins to BUF_HEIGHT pixels
            uint32_t col[BUF_HEIGHT];
            float idx_scale = (float)(nbins) / (float)(BUF_HEIGHT);
            for (int y = 0; y < BUF_HEIGHT; y++) {
                int bin = (int)(y * idx_scale);
                if (bin >= nbins) bin = nbins - 1;
                col[y] = mag_to_pixel(magnitudes[bin], &s->cfg);
            }

            pthread_mutex_lock(&s->hist_mu);
            memcpy(s->columns[s->hist_index], col, BUF_HEIGHT * sizeof(uint32_t));
            s->hist_index = (s->hist_index + 1) % HIST_SIZE;
            pthread_mutex_unlock(&s->hist_mu);

            col_count++;
            struct timespec now;
            clock_gettime(CLOCK_MONOTONIC, &now);
            double elapsed = (now.tv_sec - rate_start.tv_sec) + (now.tv_nsec - rate_start.tv_nsec) / 1e9;
            if (elapsed >= 5.0) {
                fprintf(stderr, "Column rate: %.1f cols/sec (expected ~%.1f)\n",
                        col_count / elapsed,
                        (float)s->cfg.sample_rate / (float)samples_overlap);
                col_count = 0;
                rate_start = now;
            }
        }
    }

    free(overlap_buf);
    free(magnitudes);
    free(local_buf);
    return NULL;
}

// ---- Render: build texture from circular column buffer ----
static void build_texture(app_state *s, uint32_t *pixels, int w, int h) {
    pthread_mutex_lock(&s->hist_mu);
    int idx = s->hist_index;
    int start = (idx - w + HIST_SIZE) % HIST_SIZE;

    for (int x = 0; x < w; x++) {
        int col_idx = (start + x) % HIST_SIZE;
        for (int y = 0; y < h; y++) {
            // Scale from BUF_HEIGHT to display height, invert Y (low freq at bottom)
            int buf_y = (int)((float)y / (float)h * (float)BUF_HEIGHT);
            if (buf_y >= BUF_HEIGHT) buf_y = BUF_HEIGHT - 1;
            // Screen row (h-1-y) = low freq at bottom
            pixels[(h - 1 - y) * w + x] = s->columns[col_idx][buf_y];
        }
    }
    pthread_mutex_unlock(&s->hist_mu);
}

// ---- Main render loop ----
static int run_app(config_t *cfg) {
    if (SDL_Init(SDL_INIT_VIDEO) < 0) return 1;

    Uint32 wflags = SDL_WINDOW_RESIZABLE | SDL_WINDOW_OPENGL;
    if (cfg->fullscreen) wflags |= SDL_WINDOW_FULLSCREEN_DESKTOP;

    SDL_Window *win = SDL_CreateWindow("audioprism-go",
        SDL_WINDOWPOS_UNDEFINED, SDL_WINDOWPOS_UNDEFINED,
        cfg->width, cfg->height, wflags);
    if (!win) { SDL_Quit(); return 1; }

    SDL_Renderer *ren = SDL_CreateRenderer(win, -1, SDL_RENDERER_ACCELERATED);
    if (!ren) { SDL_DestroyWindow(win); SDL_Quit(); return 1; }

    int w = cfg->width, h = cfg->height;
    SDL_Texture *tex = SDL_CreateTexture(ren, SDL_PIXELFORMAT_RGB888,
        SDL_TEXTUREACCESS_STREAMING, w, h);
    if (!tex) { SDL_DestroyRenderer(ren); SDL_DestroyWindow(win); SDL_Quit(); return 1; }

    uint32_t *pixels = (uint32_t*)calloc(w * h, sizeof(uint32_t));

    // Init state
    app_state state;
    memset(&state, 0, sizeof(state));
    state.columns = (uint32_t(*)[BUF_HEIGHT])calloc(HIST_SIZE, BUF_HEIGHT * sizeof(uint32_t));
    state.cfg = *cfg;
    state.running = 1;
    state.fft = fft_create(cfg->dft_size);
    state.hist_index = 0;
    pthread_mutex_init(&state.hist_mu, NULL);

    state.sample_cap = cfg->sample_rate;
    state.sample_buf = (float*)calloc(state.sample_cap, sizeof(float));
    state.sample_count = 0;
    pthread_mutex_init(&state.sample_mu, NULL);
    pthread_cond_init(&state.sample_cond, NULL);

    pthread_t audio_tid, spect_tid;
    pthread_create(&audio_tid, NULL, audio_thread, &state);
    pthread_create(&spect_tid, NULL, spectrogram_thread, &state);

    int quit = 0;
    while (!quit) {
        SDL_Event e;
        while (SDL_PollEvent(&e)) {
            if (e.type == SDL_QUIT) quit = 1;
            if (e.type == SDL_WINDOWEVENT && e.window.event == SDL_WINDOWEVENT_RESIZED) {
                int nw = e.window.data1, nh = e.window.data2;
                if (nw > 0 && nh > 0 && (nw != w || nh != h)) {
                    w = nw; h = nh;
                    SDL_DestroyTexture(tex);
                    tex = SDL_CreateTexture(ren, SDL_PIXELFORMAT_RGB888,
                        SDL_TEXTUREACCESS_STREAMING, w, h);
                    free(pixels);
                    pixels = (uint32_t*)calloc(w * h, sizeof(uint32_t));
                }
            }
            if (e.type == SDL_KEYDOWN) {
                SDL_Keycode k = e.key.keysym.sym;
                if (k == SDLK_q || k == SDLK_ESCAPE) quit = 1;
                if (k == SDLK_c) state.cfg.color_scheme = (state.cfg.color_scheme + 1) % 3;
                if (k == SDLK_w) state.cfg.win_type = (state.cfg.win_type + 1) % 4;
                if (k == SDLK_l) {
                    state.cfg.scale = !state.cfg.scale;
                    if (state.cfg.scale) { state.cfg.mag_min = 0; state.cfg.mag_max = 1000; }
                    else { state.cfg.mag_min = 0; state.cfg.mag_max = 45; }
                }
                if (k == SDLK_f) {
                    Uint32 fl = SDL_GetWindowFlags(win);
                    SDL_SetWindowFullscreen(win, (fl & SDL_WINDOW_FULLSCREEN_DESKTOP) ? 0 : SDL_WINDOW_FULLSCREEN_DESKTOP);
                }
            }
        }

        build_texture(&state, pixels, w, h);
        SDL_UpdateTexture(tex, NULL, pixels, w * sizeof(uint32_t));
        SDL_RenderClear(ren);
        SDL_RenderCopy(ren, tex, NULL, NULL);
        SDL_RenderPresent(ren);

        SDL_Delay(5);
    }

    state.running = 0;
    pthread_cond_signal(&state.sample_cond);
    pthread_join(audio_tid, NULL);
    pthread_join(spect_tid, NULL);

    fft_destroy(state.fft);
    free(pixels);
    free(state.sample_buf);
    free(state.columns);
    SDL_DestroyTexture(tex);
    SDL_DestroyRenderer(ren);
    SDL_DestroyWindow(win);
    SDL_Quit();
    fftwf_cleanup();
    return 0;
}
*/
import "C"

import (
	"flag"
	"os"
	"runtime"
)

func main() {
	runtime.LockOSThread()

	width := flag.Int("width", 640, "window width")
	height := flag.Int("height", 480, "window height")
	sampleRate := flag.Int("sample-rate", 24000, "audio sample rate")
	dftSize := flag.Int("dft-size", 1024, "DFT size (power of 2)")
	overlap := flag.Float64("overlap", 0.50, "overlap ratio (0.05-0.95)")
	colors := flag.String("colors", "heat", "color scheme: heat, blue, grayscale (this build is the C one; turbo, viridis and magma are Go-side only)")
	window := flag.String("window", "hann", "window function: hann, hamming, bartlett, rectangular")
	magScale := flag.String("magnitude-scale", "log", "magnitude scale: log, linear")
	magMin := flag.Float64("magnitude-min", 0.0, "magnitude minimum")
	magMax := flag.Float64("magnitude-max", 45.0, "magnitude maximum")
	fullscreen := flag.Bool("fullscreen", false, "start in fullscreen")
	flag.Parse()

	var cfg C.config_t
	cfg.width = C.int(*width)
	cfg.height = C.int(*height)
	cfg.sample_rate = C.int(*sampleRate)
	cfg.dft_size = C.int(*dftSize)
	cfg.overlap = C.float(*overlap)
	cfg.mag_min = C.float(*magMin)
	cfg.mag_max = C.float(*magMax)

	switch *colors {
	case "blue":
		cfg.color_scheme = 1
	case "grayscale", "gray":
		cfg.color_scheme = 2
	}
	switch *window {
	case "hamming":
		cfg.win_type = 1
	case "bartlett":
		cfg.win_type = 2
	case "rectangular", "rect":
		cfg.win_type = 3
	}
	if *magScale == "linear" {
		cfg.scale = 1
	}
	if *fullscreen {
		cfg.fullscreen = 1
	}

	ret := C.run_app(&cfg)
	if ret != 0 {
		os.Exit(1)
	}
}
