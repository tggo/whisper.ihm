package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	mp3 "github.com/hajimehoshi/go-mp3"
	"github.com/oov/audio/resampler"
)

type audioSegment struct {
	samples  []float32
	startSec float64
}

type transcriptSegment struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Text  string `json:"text"`
}

var defaultModelPath = "models/ggml-large-v3.bin"

var modelSizes = map[string]struct{ file, size string }{
	"tiny":           {"ggml-tiny.bin", "75 MB"},
	"tiny.en":        {"ggml-tiny.en.bin", "75 MB"},
	"base":           {"ggml-base.bin", "142 MB"},
	"base.en":        {"ggml-base.en.bin", "142 MB"},
	"small":          {"ggml-small.bin", "466 MB"},
	"small.en":       {"ggml-small.en.bin", "466 MB"},
	"medium":         {"ggml-medium.bin", "1.5 GB"},
	"medium.en":      {"ggml-medium.en.bin", "1.5 GB"},
	"large-v2":       {"ggml-large-v2.bin", "3 GB"},
	"large-v3":       {"ggml-large-v3.bin", "3 GB"},
	"large-v3-turbo": {"ggml-large-v3-turbo.bin", "1.6 GB"},
}

func main() {
	modelPath := flag.String("model", "", "Path to GGML model (overrides -size)")
	size := flag.String("size", "large-v3", "Model size: tiny, base, small, medium, large-v2, large-v3, large-v3-turbo (append .en for English-only)")
	lang := flag.String("lang", "auto", "Language code (default: auto-detect)")
	translate := flag.Bool("translate", false, "Translate to English")
	prompt := flag.String("prompt", "", "Initial prompt to guide transcription")
	format := flag.String("format", "txt", "Output format: txt, json, srt, md")
	output := flag.String("output", "", "Output file (default: stdout)")
	threads := flag.Int("threads", runtime.NumCPU(), "Number of threads")
	help := flag.Bool("help", false, "Show help")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: whisper-ihm [flags] <input.mp3>\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *help {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "\nAvailable models:\n")
		printModelList()
		os.Exit(0)
	}

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}
	inputPath := flag.Arg(0)

	// Resolve model path
	resolvedModel := *modelPath
	if resolvedModel == "" {
		info, ok := modelSizes[*size]
		if !ok {
			fmt.Fprintf(os.Stderr, "Unknown model size %q. Available models:\n", *size)
			printModelList()
			os.Exit(1)
		}
		resolvedModel = filepath.Join(filepath.Dir(defaultModelPath), info.file)
	}

	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: input file %q not found\n", inputPath)
		os.Exit(1)
	}

	if _, err := os.Stat(resolvedModel); os.IsNotExist(err) {
		if *modelPath != "" {
			fmt.Fprintf(os.Stderr, "Error: model not found at %s\n", resolvedModel)
			os.Exit(1)
		}
		info := modelSizes[*size]
		fmt.Fprintf(os.Stderr, "Model not found at %s\n", resolvedModel)
		fmt.Fprintf(os.Stderr, "Downloading %s (~%s)...\n", info.file, info.size)
		if err := downloadModel(resolvedModel, info.file); err != nil {
			fmt.Fprintf(os.Stderr, "Error downloading model: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Fprintf(os.Stderr, "Converting audio to 16kHz mono...\n")
	samples, err := convertToSamples(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error converting audio: %v\n", err)
		os.Exit(1)
	}
	totalSec := float64(len(samples)) / 16000.0
	fmt.Fprintf(os.Stderr, "Audio loaded: %.1f seconds\n", totalSec)

	fmt.Fprintf(os.Stderr, "Loading model %s...\n", resolvedModel)
	model, err := whisper.New(resolvedModel)
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

	var segments []transcriptSegment

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
		ctx.SetTranslate(*translate)
		if *prompt != "" {
			ctx.SetInitialPrompt(*prompt)
		}

		offset := time.Duration(chunk.startSec * float64(time.Second))
		segmentCb := func(segment whisper.Segment) {
			segments = append(segments, transcriptSegment{
				Start: formatDuration(segment.Start + offset),
				End:   formatDuration(segment.End + offset),
				Text:  segment.Text,
			})
		}
		if err := ctx.Process(chunk.samples, nil, segmentCb, nil); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing chunk %d: %v\n", i+1, err)
			os.Exit(1)
		}
	}

	// Write output
	out := os.Stdout
	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		out = f
	}

	switch strings.ToLower(*format) {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(segments); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
			os.Exit(1)
		}
	case "srt":
		for i, seg := range segments {
			fmt.Fprintf(out, "%d\n%s --> %s\n%s\n\n",
				i+1,
				srtTimestamp(seg.Start),
				srtTimestamp(seg.End),
				seg.Text,
			)
		}
	case "md", "markdown":
		fmt.Fprintf(out, "# Transcript\n\n")
		fmt.Fprintf(out, "| Time | Text |\n")
		fmt.Fprintf(out, "|------|------|\n")
		for _, seg := range segments {
			fmt.Fprintf(out, "| %s → %s | %s |\n", seg.Start, seg.End, seg.Text)
		}
	default: // txt
		for _, seg := range segments {
			fmt.Fprintf(out, "[%s -> %s] %s\n", seg.Start, seg.End, seg.Text)
		}
	}

	if *output != "" {
		fmt.Fprintf(os.Stderr, "Output written to %s\n", *output)
	}
	fmt.Fprintf(os.Stderr, "Done.\n")
}

func srtTimestamp(ts string) string {
	// Convert 00:00:00.000 to 00:00:00,000
	return strings.Replace(ts, ".", ",", 1)
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

const modelBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/"

func printModelList() {
	order := []string{"tiny", "tiny.en", "base", "base.en", "small", "small.en",
		"medium", "medium.en", "large-v2", "large-v3", "large-v3-turbo"}
	for _, name := range order {
		info := modelSizes[name]
		fmt.Fprintf(os.Stderr, "  %-16s %s (%s)\n", name, info.file, info.size)
	}
}

func downloadModel(dest, filename string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}

	resp, err := http.Get(modelBaseURL + filename)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	total := resp.ContentLength
	var written int64
	buf := make([]byte, 1024*1024)
	lastPct := -1
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				f.Close()
				os.Remove(tmp)
				return wErr
			}
			written += int64(n)
			if total > 0 {
				pct := int(written * 100 / total)
				if pct != lastPct {
					fmt.Fprintf(os.Stderr, "\rDownloading... %d%%", pct)
					lastPct = pct
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			f.Close()
			os.Remove(tmp)
			return readErr
		}
	}
	fmt.Fprintf(os.Stderr, "\n")

	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
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
