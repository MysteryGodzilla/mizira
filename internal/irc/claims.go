// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"regexp"
	"strings"
)

// ClaimKind is an action a reply can claim the bot took.
type ClaimKind string

const (
	ClaimRemember ClaimKind = "remember"
	ClaimIgnore   ClaimKind = "ignore"
	ClaimForget   ClaimKind = "forget"
)

// ClaimTool is the tool that has to run for a claim to be true.
var ClaimTool = map[ClaimKind]string{
	ClaimRemember: "memory__remember",
	ClaimIgnore:   "irc__ignore",
	ClaimForget:   "memory__forget",
}

// A claim is a first-person promise or report ("I'll remember", "i've stopped replying to bob"),
// with up to four words between: "I'll try my best to remember". A bare "I remember" is recall,
// not a claim.
const (
	claimSubject = `\b(?:i'?ll|i will|i'?ve|i have|i'?m going to|i'?m gonna|let me|i just|i)\b`
	claimGap     = `(?:\s+[\w']+){0,4}?\s+`
)

var claimPatterns = []struct {
	kind ClaimKind
	re   *regexp.Regexp
}{
	{ClaimRemember, regexp.MustCompile(claimSubject + claimGap +
		`(?:remember|save|saved|store|stored|note|noted|make a note|keep (?:that|this|it) in mind)\b`)},
	{ClaimForget, regexp.MustCompile(claimSubject + claimGap +
		`(?:forget|forgot|forgotten|erase|erased|delete|deleted)\b`)},
	{ClaimIgnore, regexp.MustCompile(claimSubject + claimGap +
		`(?:ignore|ignored|ignoring|mute|muted|block|blocked|stop(?:ped)? (?:responding|replying|talking) to)\b`)},
}

// heldNotDone is "I have <something> saved": what is already stored, not a new save. "I've saved"
// with nothing between is still a claim.
var heldNotDone = regexp.MustCompile(`^i(?:'ve| have) (?:got )?(?:a few|a couple|a lot|some|several|lots|many|nothing|anything|things|stuff|it|that|them|this|those|these|your|his|her|their|\d+)\b`)

// claimNegation in the matched span turns a claim into a refusal: "I won't save that".
var claimNegation = regexp.MustCompile(`\b(?:not|never|can'?t|cannot|won'?t|don'?t|didn'?t)\b|n't\b`)

// DetectClaim reports the first action a reply line claims to have taken.
func DetectClaim(line string) (ClaimKind, bool) {
	l := strings.ToLower(strings.ReplaceAll(line, "’", "'"))
	for _, p := range claimPatterns {
		for _, loc := range p.re.FindAllStringIndex(l, -1) {
			span := l[loc[0]:loc[1]]
			if bareRecall(span) || heldNotDone.MatchString(span) || claimNegation.MatchString(span) ||
				askedNotClaimed(l, loc[0], loc[1]) {
				continue
			}
			return p.kind, true
		}
	}
	return "", false
}

// bareRecall is "i remember …" or "i still remember …": memory, not a promise. "i can remember
// that for you" is an offer, and "i saved" still counts.
func bareRecall(span string) bool {
	if !strings.HasPrefix(span, "i ") || !strings.HasSuffix(span, "remember") {
		return false
	}
	return !strings.Contains(span, " can ") && !strings.Contains(span, " could ")
}

// askingModal just before the claim's "I" makes it a question: "should i", "want me to".
var askingModal = regexp.MustCompile(`\b(?:should|shall|can|could|may|would|want me to)\s*$`)

// askedNotClaimed reports a claim phrased as a question: "which one should i forget?" asks, it
// doesn't claim. A tag ("i'll remember that, okay?") is still a claim.
func askedNotClaimed(line string, start, end int) bool {
	if !askingModal.MatchString(line[:start]) {
		return false
	}
	rest := line[end:]
	if i := strings.IndexAny(rest, ".!?"); i >= 0 {
		return rest[i] == '?'
	}
	return false
}
