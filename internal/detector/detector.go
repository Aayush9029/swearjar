package detector

import (
	"sort"
	"strings"
	"unicode"

	goaway "github.com/TwiN/go-away"
)

type Match struct {
	Word  string `json:"word"`
	Group string `json:"group"`
	Index int    `json:"index"`
}

type Result struct {
	Count   int     `json:"count"`
	Matches []Match `json:"matches"`
}

type Detector struct {
	words             []string
	exact             map[string]bool
	falseNegatives    []string
	falsePositiveList []string
}

func New() *Detector {
	words := unique(append(goaway.DefaultFalseNegatives, goaway.DefaultProfanities...))
	sort.Slice(words, func(i, j int) bool {
		return len(words[i]) > len(words[j])
	})
	exact := make(map[string]bool, len(words))
	for _, word := range words {
		exact[word] = true
	}
	return &Detector{
		words:             words,
		exact:             exact,
		falseNegatives:    goaway.DefaultFalseNegatives,
		falsePositiveList: goaway.DefaultFalsePositives,
	}
}

func (d *Detector) Detect(text string) Result {
	if strings.TrimSpace(text) == "" {
		return Result{}
	}

	var matches []Match
	for _, token := range tokens(text) {
		normalized := normalize(token.text)
		if normalized == "" {
			continue
		}
		for _, word := range d.matchToken(normalized) {
			matches = append(matches, Match{
				Word:  word,
				Group: word,
				Index: token.start,
			})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Index < matches[j].Index
	})
	return Result{Count: len(matches), Matches: matches}
}

func (d *Detector) matchToken(token string) []string {
	if token == "" {
		return nil
	}

	if d.exact[token] {
		return []string{token}
	}

	for _, word := range d.falseNegatives {
		if strings.Contains(token, word) {
			return []string{word}
		}
	}

	candidate, hadFalsePositive := d.withoutFalsePositives(token)
	if candidate == "" {
		return nil
	}
	if d.exact[candidate] {
		return []string{candidate}
	}
	if knownWord(candidate) {
		return nil
	}
	for _, word := range d.words {
		if d.matchesCompound(candidate, word) {
			return []string{word}
		}
	}
	if hadFalsePositive {
		return nil
	}

	return nil
}

func (d *Detector) withoutFalsePositives(token string) (string, bool) {
	changed := false
	for _, word := range d.falsePositiveList {
		next := strings.ReplaceAll(token, word, "")
		if next != token {
			changed = true
			token = next
		}
	}
	return token, changed
}

func (d *Detector) matchesCompound(token, word string) bool {
	if len(word) < 4 || len(token) <= len(word) {
		return false
	}
	if len(token)-len(word) < 2 {
		return false
	}
	if !strings.Contains(token, word) {
		return false
	}
	prefix := strings.HasPrefix(token, word)
	suffix := strings.HasSuffix(token, word)
	if !prefix && !suffix {
		return false
	}
	return len(token)-len(word) <= 6
}

type token struct {
	text  string
	start int
}

func tokens(text string) []token {
	var out []token
	var b strings.Builder
	start := -1
	for index, r := range text {
		if !isTokenRune(r) {
			if b.Len() > 0 {
				out = append(out, token{text: b.String(), start: start})
				b.Reset()
				start = -1
			}
			continue
		}
		if start == -1 {
			start = index
		}
		b.WriteRune(r)
	}
	if b.Len() > 0 {
		out = append(out, token{text: b.String(), start: start})
	}
	return out
}

func isTokenRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	_, ok := goaway.DefaultCharacterReplacements[r]
	return ok
}

func normalize(s string) string {
	s = strings.TrimFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if replacement, ok := goaway.DefaultCharacterReplacements[r]; ok {
			r = replacement
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return collapseRepeats(b.String())
}

func collapseRepeats(s string) string {
	var b strings.Builder
	var previous rune
	repeat := 0
	for _, r := range s {
		if r == previous {
			repeat++
			if repeat > 1 {
				continue
			}
		} else {
			repeat = 0
		}
		b.WriteRune(r)
		previous = r
	}
	return b.String()
}

func unique(words []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(words))
	for _, word := range words {
		word = strings.ToLower(strings.TrimSpace(word))
		if word == "" || seen[word] || moderationOnlyTerms[word] {
			continue
		}
		seen[word] = true
		out = append(out, word)
	}
	return out
}

var moderationOnlyTerms = map[string]bool{
	"anal":      true,
	"anus":      true,
	"balls":     true,
	"ballsack":  true,
	"blowjob":   true,
	"boner":     true,
	"boob":      true,
	"butt":      true,
	"choad":     true,
	"clitoris":  true,
	"cum":       true,
	"dildo":     true,
	"fellate":   true,
	"fellatio":  true,
	"felching":  true,
	"flange":    true,
	"horny":     true,
	"incest":    true,
	"jizz":      true,
	"labia":     true,
	"masturbat": true,
	"muff":      true,
	"naked":     true,
	"nipple":    true,
	"nips":      true,
	"nude":      true,
	"pedophile": true,
	"penis":     true,
	"poop":      true,
	"porn":      true,
	"prostitut": true,
	"pube":      true,
	"pussie":    true,
	"rimjob":    true,
	"scrotum":   true,
	"sex":       true,
	"spunk":     true,
	"tits":      true,
	"tittie":    true,
	"titty":     true,
	"turd":      true,
	"vagina":    true,
}
