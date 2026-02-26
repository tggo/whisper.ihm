package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"time"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	mp3 "github.com/hajimehoshi/go-mp3"
	"github.com/oov/audio/resampler"
)

type audioSegment struct {
	samples  []float32
	startSec float64
}

func main() {
	modelPath := flag.String("model", "models/ggml-large-v3.bin", "Path to GGML model")
	lang := flag.String("lang", "auto", "Language code (default: auto-detect)")
	threads := flag.Int("threads", runtime.NumCPU(), "Number of threads")
	help := flag.Bool("help", false, "Show help")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: whisper-ihm [flags] <input.mp3>\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}
	inputPath := flag.Arg(0)

	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: input file %q not found\n", inputPath)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Converting audio to 16kHz mono...\n")
	samples, err := convertToSamples(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error converting audio: %v\n", err)
		os.Exit(1)
	}
	totalSec := float64(len(samples)) / 16000.0
	fmt.Fprintf(os.Stderr, "Audio loaded: %.1f seconds\n", totalSec)

	fmt.Fprintf(os.Stderr, "Loading model %s...\n", *modelPath)
	model, err := whisper.New(*modelPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading model: %v\n", err)
		os.Exit(1)
	}
	defer model.Close()

	fmt.Fprintf(os.Stderr, "Detecting speech segments...\n")
	chunks, err := segmentByVAD(samples)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error in VAD segmentation: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Found %d speech chunk(s)\n", len(chunks))

	for i, chunk := range chunks {
		ctx, err := model.NewContext()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating context: %v\n", err)
			os.Exit(1)
		}
		if err := ctx.SetLanguage(*lang); err != nil {
			fmt.Fprintf(os.Stderr, "Error setting language %q: %v\n", *lang, err)
			os.Exit(1)
		}
		ctx.SetThreads(uint(*threads))

		offset := time.Duration(chunk.startSec * float64(time.Second))
		segmentCb := func(segment whisper.Segment) {
			fmt.Printf("[%s -> %s] %s\n",
				formatDuration(segment.Start+offset),
				formatDuration(segment.End+offset),
				segment.Text,
			)
		}
		if err := ctx.Process(chunk.samples, nil, segmentCb, nil); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing chunk %d: %v\n", i+1, err)
			os.Exit(1)
		}
	}

	fmt.Fprintf(os.Stderr, "Done.\n")
}

func segmentByVAD(samples []float32) ([]audioSegment, error) {
	const (
		sampleRate   = 16000
		hopSize      = 256   // 16ms frames
		threshold    = 0.45  // VAD sensitivity
		silenceGap   = 31    // ~500ms of silence to split (sampleRate * 0.5 / hopSize)
		paddingSamps = 3200  // 200ms padding (sampleRate * 0.2)
	)

	vad, err := NewVad(hopSize, threshold)
	if err != nil {
		return nil, fmt.Errorf("create vad: %w", err)
	}
	defer vad.Close()

	totalFrames := len(samples) / hopSize
	frame := make([]int16, hopSize)

	type rawSegment struct {
		startFrame int
		endFrame   int
	}

	var segments []rawSegment
	inSpeech := false
	speechStart := 0
	silenceCount := 0

	for f := 0; f < totalFrames; f++ {
		off := f * hopSize
		for i := 0; i < hopSize; i++ {
			v := samples[off+i]
			if v > 1.0 {
				v = 1.0
			} else if v < -1.0 {
				v = -1.0
			}
			frame[i] = int16(v * math.MaxInt16)
		}

		_, isSpeech, err := vad.Process(frame)
		if err != nil {
			return nil, fmt.Errorf("vad process frame %d: %w", f, err)
		}

		if isSpeech {
			if !inSpeech {
				speechStart = f
				inSpeech = true
			}
			silenceCount = 0
		} else if inSpeech {
			silenceCount++
			if silenceCount >= silenceGap {
				segments = append(segments, rawSegment{speechStart, f - silenceCount})
				inSpeech = false
				silenceCount = 0
			}
		}
	}
	if inSpeech {
		segments = append(segments, rawSegment{speechStart, totalFrames - 1})
	}

	// If no speech detected, return the whole audio as one chunk
	if len(segments) == 0 {
		return []audioSegment{{samples: samples, startSec: 0}}, nil
	}

	result := make([]audioSegment, 0, len(segments))
	for _, seg := range segments {
		startSamp := seg.startFrame*hopSize - paddingSamps
		if startSamp < 0 {
			startSamp = 0
		}
		endSamp := seg.endFrame*hopSize + hopSize + paddingSamps
		if endSamp > len(samples) {
			endSamp = len(samples)
		}
		result = append(result, audioSegment{
			samples:  samples[startSamp:endSamp],
			startSec: float64(startSamp) / sampleRate,
		})
	}
	return result, nil
}

func convertToSamples(inputPath string) ([]float32, error) {
	f, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	d, err := mp3.NewDecoder(f)
	if err != nil {
		return nil, fmt.Errorf("decode mp3: %w", err)
	}

	pcm, err := io.ReadAll(d)
	if err != nil {
		return nil, fmt.Errorf("read pcm: %w", err)
	}

	// go-mp3 outputs stereo int16 LE: each frame is 4 bytes [L_lo, L_hi, R_lo, R_hi]
	numFrames := len(pcm) / 4
	mono := make([]float32, numFrames)
	for i := 0; i < numFrames; i++ {
		l := int16(binary.LittleEndian.Uint16(pcm[i*4:]))
		r := int16(binary.LittleEndian.Uint16(pcm[i*4+2:]))
		mono[i] = (float32(l) + float32(r)) / (2 * 32768.0)
	}

	// Resample from source rate to 16kHz
	srcRate := d.SampleRate()
	const dstRate = 16000
	if srcRate == dstRate {
		return mono, nil
	}
	outLen := int(float64(len(mono))*float64(dstRate)/float64(srcRate)) + 256
	out := make([]float32, outLen)
	_, written := resampler.Resample32(mono, srcRate, out, dstRate, 4)
	return out[:written], nil
}

func formatDuration(d time.Duration) string {
	total := d.Milliseconds()
	if total < 0 {
		total = 0
	}
	ms := total % 1000
	total /= 1000
	s := total % 60
	total /= 60
	m := total % 60
	h := total / 60
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}
