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
	ClaimSearch   ClaimKind = "search"
	ClaimDraw     ClaimKind = "draw"
)

// ClaimTool is the tool that has to run for a claim to be true.
var ClaimTool = map[ClaimKind]string{
	ClaimRemember: "memory__remember",
	ClaimIgnore:   "irc__ignore",
	ClaimForget:   "memory__forget",
	ClaimSearch:   "websearch__web_search",
	ClaimDraw:     "imagegen__picture",
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
	// A promise or "I'm searching": said without the call, the person waits for results that never
	// come. "I searched" and "I was looking it up" report an earlier search, so they don't count.
	{ClaimSearch, regexp.MustCompile(`\b(?:i'?ll|i will|i'?m|i am|let me)\b` + claimGap +
		`(?:search|searching|look (?:it|that|this|them|those) up|looking (?:it|that|this|them|those) up|look up|looking up)\b`)},
	// "Give me a sec to get the prompt right" once ended a turn with no picture.
	{ClaimDraw, regexp.MustCompile(`\b(?:i'?ll|i will|i'?m|i am|let me)\b` + claimGap +
		`(?:draw|drawing|sketch|sketching|paint|painting|make (?:it|that|this|one|you one|an? (?:new |fresh )?(?:image|picture|pic|drawing|portrait)))\b`)},
}

// offerModal in a search claim makes it an offer, not a report: "i can look it up for you".
var offerModal = regexp.MustCompile(`\b(?:can|could|might|may)\b`)

// heldNotDone is "I have <something> saved": what is already stored, not a new save. "I've saved"
// with nothing between is still a claim.
var heldNotDone = regexp.MustCompile(`^i(?:'ve| have) (?:got )?(?:a few|a couple|a lot|some|several|lots|many|nothing|anything|things|stuff|it|that|them|this|those|these|your|his|her|their|\d+)\b`)

// describesHoldings just before the claim's "I" makes it a description of what is already held:
// "that's all I have stored", "what I've saved about you".
var describesHoldings = regexp.MustCompile(`\b(?:all|what|everything|anything|whatever)\s*$`)

// claimNegation in the matched span turns a claim into a refusal: "I won't save that".
var claimNegation = regexp.MustCompile(`\b(?:not|never|can'?t|cannot|won'?t|don'?t|didn'?t)\b|n't\b`)

// DetectClaim reports the first action a reply line claims to have taken.
func DetectClaim(line string) (ClaimKind, bool) {
	l := strings.ToLower(strings.ReplaceAll(line, "’", "'"))
	for _, p := range claimPatterns {
		for _, loc := range p.re.FindAllStringIndex(l, -1) {
			span := l[loc[0]:loc[1]]
			if bareRecall(span) || heldNotDone.MatchString(span) || claimNegation.MatchString(span) ||
				((p.kind == ClaimSearch || p.kind == ClaimDraw) && offerModal.MatchString(span)) ||
				askedNotClaimed(l, loc[0], loc[1]) || describesHoldings.MatchString(l[:loc[0]]) {
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

// sentenceEnd splits a reply into sentences, keeping each one's closing punctuation.
var sentenceEnd = regexp.MustCompile(`[^.!?]+[.!?]*`)

// WithoutClaims drops the sentences of text that claim an action, keeping the rest as written.
func WithoutClaims(text string) string {
	var kept []string
	for _, s := range sentenceEnd.FindAllString(text, -1) {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		if _, claims := DetectClaim(s); !claims {
			kept = append(kept, s)
		}
	}
	return strings.Join(kept, " ")
}

// pictureAsk is a request for a picture anywhere in a message: "draw us at the beach", "make an image
// of hori", "image of drunk hori in shibuya".
var pictureAsk = regexp.MustCompile(`(?i)\b(?:draw|sketch|paint)\b|\b(?:make|generate|create|do|give|try)\b[^.!?]{0,40}\b(?:image|picture|pic|portrait|drawing)s?\b|^(?:[\w-]+[,:]?\s+)?(?:an?\s+|another\s+|new\s+|fresh\s+)?(?:image|picture|pic|portrait)s?\s+(?:of|with)\b`)

// AsksForPicture reports a message asking for a picture.
func AsksForPicture(text string) bool {
	return pictureAsk.MatchString(strings.TrimSpace(text))
}

var anyLink = regexp.MustCompile(`(?i)\bhttps?://\S+`)

// HasLink reports a line carrying a web link.
func HasLink(line string) bool {
	return anyLink.MatchString(line)
}
