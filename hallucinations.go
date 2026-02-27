package main

import (
	"math"
	"strings"
	"unicode/utf8"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// Known Whisper hallucination phrases (lowercase, trimmed).
// Sourced from Vexa production logs and common Whisper artifacts.
var hallucinationPhrases = map[string]struct{}{
	// Short filler / artifacts
	".": {}, "...": {}, "…": {}, "♪": {}, "🎵": {},

	// English — short
	"all right.":           {},
	"aw.":                  {},
	"aww.":                 {},
	"bye.":                 {},
	"bye bye.":             {},
	"bye-bye.":             {},
	"bye!":                 {},
	"can i go?":            {},
	"everything all right.": {},
	"god bless you.":       {},
	"good.":                {},
	"i don't know.":        {},
	"i love you.":          {},
	"i'm happy to be here.": {},
	"i'm just a form.":     {},
	"i'm sorry.":           {},
	"i'm so glad to be here.": {},
	"i am very good.":      {},
	"it's awesome.":        {},
	"it's horrible.":       {},
	"let's do that again.": {},
	"nice.":                {},
	"no.":                  {},
	"oh, my god.":          {},
	"ok.":                  {},
	"okay.":                {},
	"right here.":          {},
	"so":                   {},
	"oh":                   {},
	"that's it.":           {},
	"that's the whole thing.": {},
	"uh-huh.":              {},
	"we'll be right back.": {},
	"yeah.":                {},
	"yes.":                 {},
	"you":                  {},

	// English — thank you variants
	"thank you.":                  {},
	"thank you":                   {},
	"thank you all.":              {},
	"thank you so much.":          {},
	"thank you very much.":        {},
	"thank you for having me.":    {},
	"thank you for listening.":    {},
	"thank you for today.":        {},
	"thank you for your time.":    {},
	"thank you very much for coming.": {},
	"okay, thank you.":            {},
	"all right, thank you.":       {},
	"thanks.":                     {},
	"thanks for watching.":        {},
	"thanks for watching!":        {},

	// English — longer hallucination phrases
	"have a good night, guys.":     {},
	"i'll see you next time.":      {},
	"the end.":                     {},
	"the end":                      {},
	"goodbye.":                     {},
	"subscribe":                    {},
	"please subscribe":             {},
	"subscribe to my channel":      {},
	"like and subscribe":           {},

	// English — meta/subtitle artifacts
	"subtitles by the amara.org community": {},
	"subtitles made by":                    {},
	"translated by":                        {},
	"copyright":                            {},
	"music":                                {},
	"applause":                             {},
	"laughter":                             {},
	"silence":                              {},

	// Ukrainian
	"дякую":                          {},
	"дякую за перегляд":              {},
	"дякую за вашу підтримку":        {},
	"підписуйтесь на наш канал":     {},
	"звуки вибухів":                  {},
	"субтитрувальниця оля шор":      {},

	// Russian
	"продолжение следует...":                   {},
	"продолжение следует":                      {},
	"спасибо.":                                 {},
	"спасибо":                                  {},
	"спасибо за просмотр":                      {},
	"все спасибо":                              {},
	"до новых встреч":                          {},
	"субтитры создавал dimatorzok":             {},
	"субтитры сделал dimatorzok":               {},
	"субтитры делал dimatorzok":                {},
	"субтитры добавил dimatorzok":              {},
	"субтитры подогнал «симон»":                {},
	"динамичная музыка":                        {},
	"и":                                        {},
	"спасибо за субтитры алексею дубровскому!": {},
	"смотрите другие видео":                    {},
}

// Prefixes that indicate hallucination when they start a segment.
var hallucinationPrefixes = []string{
	// English
	"thank you so much for joining",
	"thank you for watching",
	"thanks for watching",
	"thank you, mr. president",
	"i'm going to try the switch",
	"i'm going to say it's good already",
	"i said good already",
	"so we're going to talk about this",
	"we're going to talk about this",
	"we're going to be a better",
	"i'm speaking, i'm speaking",
	"next slide, next slide",
	"i got this",
	"subtitles",
	"translated by",
	// Ukrainian
	"дякую за",
	"підписуйтесь",
	"субтитри",
	// Russian
	"подписывайтесь на",
	"подпишитесь на",
	"подпишись на",
	"спасибо за субтитры",
	"ставьте лайки",
	"редактор субтитров",
	"корректор а",
	"субтитры сделал",
	"субтитры делал",
	"субтитры добавил",
	"субтитры создавал",
	"канал субтитры",
	"смотрите продолжение",
	"всем привет и добро пожаловать",
}

const (
	noSpeechProbThreshold = 0.6
	avgLogprobThreshold   = -1.0
	compressionThreshold  = 2.4
	minSegmentChars       = 3
	minRealWords          = 1
)

// hasRepeatedChars returns true if any character appears 5+ times consecutively.
func hasRepeatedChars(s string) bool {
	var prev rune
	count := 1
	for _, r := range s {
		if r == prev {
			count++
			if count >= 5 {
				return true
			}
		} else {
			prev = r
			count = 1
		}
	}
	return false
}

// shouldSkipSegment returns true if the segment is likely a hallucination.
func shouldSkipSegment(segment whisper.Segment) bool {
	if segment.NoSpeechProb > noSpeechProbThreshold {
		return true
	}

	text := strings.TrimSpace(segment.Text)
	if utf8.RuneCountInString(text) < minSegmentChars {
		return true
	}

	if !hasRealWords(text, minRealWords) {
		return true
	}

	if isKnownHallucination(text) {
		return true
	}

	if hasRepeatedChars(text) {
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

// isKnownHallucination checks exact match and prefix match.
func isKnownHallucination(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if _, found := hallucinationPhrases[normalized]; found {
		return true
	}
	for _, prefix := range hallucinationPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

// hasRealWords returns true if text contains at least n words with 3+ characters
// that are not stopwords.
func hasRealWords(text string, n int) bool {
	count := 0
	for _, w := range strings.Fields(text) {
		if utf8.RuneCountInString(w) >= 3 && !isStopword(w) {
			count++
			if count >= n {
				return true
			}
		}
	}
	return false
}

var stopwords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "you": {}, "this": {},
	"that": {}, "with": {}, "from": {}, "have": {}, "are": {},
}

func isStopword(word string) bool {
	_, found := stopwords[strings.ToLower(word)]
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
	total := float64(len(text) - 1)
	unique := float64(len(seen))
	if unique == 0 {
		return 0
	}
	return total / unique
}
