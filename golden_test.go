package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// TestGolden runs transcription on test audio files and compares to expected output.
// Requires a whisper model and CGO build. Run with: make build && go test -run TestGolden -v
func TestGolden(t *testing.T) {
	modelPath := os.Getenv("WHISPER_MODEL")
	if modelPath == "" {
		modelPath = "models/ggml-large-v3.bin"
	}
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Skipf("Model not found at %s; set WHISPER_MODEL or run make setup", modelPath)
	}

	model, err := whisper.New(modelPath)
	if err != nil {
		t.Fatalf("Failed to load model: %v", err)
	}
	defer model.Close()

	entries, err := filepath.Glob("testdata/golden/*.mp3")
	if err != nil {
		t.Fatalf("Failed to glob test files: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("No test MP3 files found in testdata/golden/")
	}

	for _, mp3Path := range entries {
		name := strings.TrimSuffix(filepath.Base(mp3Path), ".mp3")
		expectedPath := strings.TrimSuffix(mp3Path, ".mp3") + ".expected.txt"

		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			continue // skip MP3s without expected files
		}

		t.Run(name, func(t *testing.T) {
			expected, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("Failed to read expected file: %v", err)
			}
			expectedText := strings.TrimSpace(string(expected))

			samples, err := convertToSamples(mp3Path)
			if err != nil {
				t.Fatalf("Failed to convert audio: %v", err)
			}

			chunks, err := segmentByVAD(samples)
			if err != nil {
				t.Fatalf("VAD segmentation failed: %v", err)
			}

			var texts []string
			for _, chunk := range chunks {
				ctx, err := model.NewContext()
				if err != nil {
					t.Fatalf("Failed to create context: %v", err)
				}
				if err := ctx.SetLanguage("auto"); err != nil {
					t.Fatalf("Failed to set language: %v", err)
				}

				segmentCb := func(segment whisper.Segment) {
					if shouldSkipSegment(segment) {
						return
					}
					texts = append(texts, strings.TrimSpace(segment.Text))
				}
				if err := ctx.Process(chunk.samples, nil, segmentCb, nil); err != nil {
					t.Fatalf("Process failed: %v", err)
				}
			}

			actualText := strings.Join(texts, " ")

			if expectedText == "" {
				// Silence test: output must be empty
				if actualText != "" {
					t.Errorf("Expected empty transcript for silence, got: %q", actualText)
				}
				return
			}

			// For speech tests, compute WER
			wer := wordErrorRate(expectedText, actualText)
			t.Logf("WER: %.1f%% (%d words expected)", wer*100, len(strings.Fields(expectedText)))
			t.Logf("Expected: %s", expectedText)
			t.Logf("Actual:   %s", actualText)

			if wer > 0.3 {
				t.Errorf("WER %.1f%% exceeds threshold 30%%", wer*100)
			}
		})
	}
}

// wordErrorRate computes the word error rate between reference and hypothesis
// using the Levenshtein distance on word sequences.
func wordErrorRate(reference, hypothesis string) float64 {
	ref := strings.Fields(strings.ToLower(reference))
	hyp := strings.Fields(strings.ToLower(hypothesis))

	if len(ref) == 0 {
		if len(hyp) == 0 {
			return 0
		}
		return 1
	}

	// Levenshtein on words
	m := len(ref)
	n := len(hyp)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
		dp[i][0] = i
	}
	for j := 0; j <= n; j++ {
		dp[0][j] = j
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			cost := 1
			if ref[i-1] == hyp[j-1] {
				cost = 0
			}
			dp[i][j] = min(dp[i-1][j]+1, min(dp[i][j-1]+1, dp[i-1][j-1]+cost))
		}
	}

	return float64(dp[m][n]) / float64(m)
}
