package main

import (
	"testing"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

func TestIsKnownHallucination(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"Thank you.", true},
		{"thank you.", true},
		{"THANK YOU.", true},
		{"Thanks for watching!", true},
		{"Bye.", true},
		{".", true},
		{"...", true},
		// Prefix matches
		{"Thank you so much for joining us today.", true},
		{"Thank you for watching and see you next time.", true},
		{"I'm speaking, I'm speaking, I'm speaking.", true},
		{"Subtitles by someone", true},
		// Real text should pass
		{"Hello, how are you?", false},
		{"The quick brown fox", false},
		{"We need to discuss the project timeline.", false},
		// Ukrainian
		{"Дякую за перегляд", true},
		{"Підписуйтесь на наш канал", true},
		{"Дякую за вашу підтримку", true},
		{"Субтитри зроблені кимось", true}, // prefix match
		// Russian
		{"Спасибо.", true},
		{"Продолжение следует...", true},
		{"Подписывайтесь на мой канал", true},  // prefix match
		{"Ставьте лайки и подписывайтесь", true}, // prefix match
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := isKnownHallucination(tt.text)
			if got != tt.want {
				t.Errorf("isKnownHallucination(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestHasRealWords(t *testing.T) {
	tests := []struct {
		text string
		n    int
		want bool
	}{
		{"Hello world", 1, true},
		{"the and for", 1, false},    // all stopwords
		{"a b c", 1, false},          // all < 3 chars
		{"discussion today", 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := hasRealWords(tt.text, tt.n)
			if got != tt.want {
				t.Errorf("hasRealWords(%q, %d) = %v, want %v", tt.text, tt.n, got, tt.want)
			}
		})
	}
}

func TestHasRepeatedChars(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"aaaaaa", true},
		{"hahahahahaha", false}, // 'h' repeats but not consecutively 5+
		{"normal text", false},
		{"oooooh no", true}, // 5+ 'o'
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := hasRepeatedChars(tt.text)
			if got != tt.want {
				t.Errorf("hasRepeatedChars(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestCompressionRatio(t *testing.T) {
	repetitive := "the the the the the the the the the the"
	normal := "The quick brown fox jumps over the lazy dog"

	repRatio := compressionRatio(repetitive)
	normRatio := compressionRatio(normal)

	if repRatio <= normRatio {
		t.Errorf("Expected repetitive ratio (%.2f) > normal ratio (%.2f)", repRatio, normRatio)
	}
}

func TestShouldSkipSegment(t *testing.T) {
	tests := []struct {
		name    string
		segment whisper.Segment
		want    bool
	}{
		{
			name: "high no_speech_prob",
			segment: whisper.Segment{
				Text:         "Some real text here",
				NoSpeechProb: 0.8,
				Tokens:       []whisper.Token{{P: 0.9}},
			},
			want: true,
		},
		{
			name: "known hallucination",
			segment: whisper.Segment{
				Text:         "Thank you.",
				NoSpeechProb: 0.1,
				Tokens:       []whisper.Token{{P: 0.9}},
			},
			want: true,
		},
		{
			name: "too short",
			segment: whisper.Segment{
				Text:         "ok",
				NoSpeechProb: 0.1,
				Tokens:       []whisper.Token{{P: 0.9}},
			},
			want: true,
		},
		{
			name: "prefix hallucination",
			segment: whisper.Segment{
				Text:         "Thank you so much for joining us today and every day.",
				NoSpeechProb: 0.1,
				Tokens:       []whisper.Token{{P: 0.9}},
			},
			want: true,
		},
		{
			name: "good segment",
			segment: whisper.Segment{
				Text:         "Hello, this is a test of the transcription system.",
				NoSpeechProb: 0.1,
				Tokens:       []whisper.Token{{P: 0.9}, {P: 0.85}, {P: 0.92}},
			},
			want: false,
		},
		{
			name: "low confidence tokens",
			segment: whisper.Segment{
				Text:         "some random words here",
				NoSpeechProb: 0.3,
				Tokens:       []whisper.Token{{P: 0.01}, {P: 0.02}, {P: 0.01}},
			},
			want: true,
		},
		{
			name: "repeated characters",
			segment: whisper.Segment{
				Text:         "aaaaaaaaaa something",
				NoSpeechProb: 0.1,
				Tokens:       []whisper.Token{{P: 0.9}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipSegment(tt.segment)
			if got != tt.want {
				t.Errorf("shouldSkipSegment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWordErrorRate(t *testing.T) {
	tests := []struct {
		ref, hyp string
		maxWER   float64
	}{
		{"hello world", "hello world", 0.001},
		{"hello world", "hello", 0.51},
		{"the cat sat on the mat", "the cat sat on the mat", 0.001},
		{"", "", 0.001},
	}

	for _, tt := range tests {
		wer := wordErrorRate(tt.ref, tt.hyp)
		if wer > tt.maxWER {
			t.Errorf("wordErrorRate(%q, %q) = %.2f, want <= %.2f", tt.ref, tt.hyp, wer, tt.maxWER)
		}
	}
}
