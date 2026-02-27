package main

import (
	"math"
	"strings"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// Known Whisper hallucination phrases (lowercase, trimmed).
// Sourced from common patterns in Whisper outputs on silence/noise.
var hallucinationPhrases = map[string]struct{}{
	"thank you.":                          {},
	"thank you":                           {},
	"thanks for watching.":                {},
	"thanks for watching!":                {},
	"thanks for watching":                 {},
	"thank you for watching.":             {},
	"thank you for watching!":             {},
	"thank you for watching":              {},
	"bye.":                                {},
	"bye":                                 {},
	"bye bye.":                            {},
	"bye bye":                             {},
	"goodbye.":                            {},
	"goodbye":                             {},
	"you":                                 {},
	"the end.":                            {},
	"the end":                             {},
	"i'm sorry.":                          {},
	"so":                                  {},
	"oh":                                  {},
	"subscribe":                           {},
	"please subscribe":                    {},
	"subscribe to my channel":             {},
	"like and subscribe":                  {},
	"subtitles by the amara.org community": {},
	"subtitles made by":                   {},
	"translated by":                       {},
	"copyright":                           {},
	"music":                               {},
	"applause":                            {},
	"laughter":                            {},
	"silence":                             {},
	".":                                   {},
	"...":                                 {},
	"…":                                   {},
	"♪":                                   {},
	"🎵":                                  {},
}

const (
	noSpeechProbThreshold = 0.6
	avgLogprobThreshold   = -1.0
	compressionThreshold  = 2.4
)

// shouldSkipSegment returns true if the segment is likely a hallucination.
func shouldSkipSegment(segment whisper.Segment) bool {
	if segment.NoSpeechProb > noSpeechProbThreshold {
		return true
	}

	text := strings.TrimSpace(segment.Text)
	if text == "" {
		return true
	}

	if isKnownHallucination(text) {
		return true
	}

	if avgLogprob(segment) < avgLogprobThreshold {
		return true
	}

	if compressionRatio(text) > compressionThreshold {
		return true
	}

	return false
}

// isKnownHallucination checks against the curated phrase list.
func isKnownHallucination(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	_, found := hallucinationPhrases[normalized]
	return found
}

// avgLogprob computes the average log probability across text tokens.
func avgLogprob(segment whisper.Segment) float64 {
	if len(segment.Tokens) == 0 {
		return 0
	}
	var sum float64
	var count int
	for _, t := range segment.Tokens {
		if t.P > 0 {
			sum += math.Log(float64(t.P))
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// compressionRatio estimates text repetitiveness using a simple
// character bigram compression ratio.
func compressionRatio(text string) float64 {
	if len(text) == 0 {
		return 0
	}
	seen := make(map[string]struct{})
	for i := 0; i < len(text)-1; i++ {
		seen[text[i:i+2]] = struct{}{}
	}
	// Ratio of total bigrams to unique bigrams.
	// Highly repetitive text has a high ratio.
	total := float64(len(text) - 1)
	unique := float64(len(seen))
	if unique == 0 {
		return 0
	}
	return total / unique
}
