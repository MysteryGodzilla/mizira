// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"regexp"
	"strings"
)

// loopWord is one word for loop detection: letters, digits and apostrophes.
var loopWord = regexp.MustCompile(`[\p{L}\p{N}']+`)

const (
	maxLoopPeriod = 8  // longest repeating phrase, in words, the guard looks for
	minLoopReps   = 4  // a phrase must repeat this many times in a row
	minLoopWords  = 12 // and cover at least this many words, so "ha ha ha" is fine
)

// loopStart reports where text has started repeating itself: a phrase of up to maxLoopPeriod words
// said minLoopReps times or more in a row at the end ("...i... i... blushes ...i... i... blushes").
// The offset is the end of the phrase's first saying, so the cut keeps one copy of it: never empty.
func loopStart(text string) (int, bool) {
	spans := loopWord.FindAllStringIndex(text, -1)
	words := make([]string, len(spans))
	for i, s := range spans {
		words[i] = strings.ToLower(text[s[0]:s[1]])
	}
	n := len(words)
	for period := 1; period <= maxLoopPeriod && period*minLoopReps <= n; period++ {
		reps := 1
		for start := n - 2*period; start >= 0 && sameWords(words[start:start+period], words[n-period:]); start -= period {
			reps++
		}
		if reps >= minLoopReps && reps*period >= minLoopWords {
			// The stream may stop mid-phrase, so the unit found is shifted; walk back to where the
			// repetition really began, and keep one whole copy from there.
			start := n - reps*period
			for start > 0 && words[start-1] == words[start-1+period] {
				start--
			}
			return spans[start+period-1][1], true
		}
	}
	return 0, false
}

func sameWords(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
