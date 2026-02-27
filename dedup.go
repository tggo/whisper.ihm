package main

import (
	"strings"
	"time"
)

// deduplicateSegments removes duplicate and overlapping segments.
// It filters:
//   - segments with identical text and overlapping time ranges (keep the longer one)
//   - segments fully contained within a longer segment with different text (keep the longer one)
func deduplicateSegments(segments []transcriptSegment) []transcriptSegment {
	if len(segments) <= 1 {
		return segments
	}

	var result []transcriptSegment

	for _, seg := range segments {
		text := strings.TrimSpace(seg.Text)
		if text == "" {
			continue
		}

		segStart := parseDuration(seg.Start)
		segEnd := parseDuration(seg.End)
		segDur := segEnd - segStart

		replaced := false
		skip := false

		for i, prev := range result {
			prevStart := parseDuration(prev.Start)
			prevEnd := parseDuration(prev.End)
			prevDur := prevEnd - prevStart

			// Same text — keep the one with wider time span
			if strings.TrimSpace(prev.Text) == text {
				if segStart >= prevStart && segEnd <= prevEnd {
					// Current is contained in prev — skip current
					skip = true
					break
				}
				if prevStart >= segStart && prevEnd <= segEnd {
					// Prev is contained in current — replace prev
					result[i] = seg
					replaced = true
					break
				}
			}

			// Different text, temporal overlap — keep longer segment
			if overlaps(segStart, segEnd, prevStart, prevEnd) {
				if segStart >= prevStart && segEnd <= prevEnd && segDur < prevDur && len(text) < len(strings.TrimSpace(prev.Text)) {
					skip = true
					break
				}
				if prevStart >= segStart && prevEnd <= segEnd && prevDur < segDur && len(strings.TrimSpace(prev.Text)) < len(text) {
					result[i] = seg
					replaced = true
					break
				}
			}
		}

		if !skip && !replaced {
			result = append(result, seg)
		}
	}

	return result
}

func overlaps(aStart, aEnd, bStart, bEnd time.Duration) bool {
	return aStart < bEnd && bStart < aEnd
}

// parseDuration parses "HH:MM:SS.mmm" back to time.Duration.
func parseDuration(s string) time.Duration {
	// Format: 00:00:00.000
	var h, m, sec, ms int
	// Use fmt.Sscanf-like parsing
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return 0
	}
	h = atoi(parts[0])
	m = atoi(parts[1])
	secParts := strings.SplitN(parts[2], ".", 2)
	sec = atoi(secParts[0])
	if len(secParts) == 2 {
		ms = atoi(secParts[1])
	}
	return time.Duration(h)*time.Hour +
		time.Duration(m)*time.Minute +
		time.Duration(sec)*time.Second +
		time.Duration(ms)*time.Millisecond
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
