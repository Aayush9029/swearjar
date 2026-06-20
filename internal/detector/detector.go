package detector

import (
	"strings"
	"unicode"

	goaway "github.com/TwiN/go-away"
)

type Match struct {
	Word   string `json:"word"`
	Group  string `json:"group"`
	Source string `json:"source"`
	Index  int    `json:"index"`
}

type Result struct {
	Count   int     `json:"count"`
	Matches []Match `json:"matches"`
}

type Detector struct {
	engine *goaway.ProfanityDetector
}

func New() *Detector {
	return &Detector{engine: goaway.NewProfanityDetector()}
}

func (d *Detector) Detect(text string) Result {
	if strings.TrimSpace(text) == "" {
		return Result{}
	}
	censored := d.engine.Censor(text)
	if censored == text && !d.engine.IsProfane(text) {
		return Result{}
	}

	ranges := profanityRanges([]rune(censored))
	matches := make([]Match, 0, len(ranges))
	original := []rune(text)
	for _, r := range ranges {
		if r.start < 0 || r.start >= len(original) {
			continue
		}
		end := min(r.end, len(original))
		snippet := string(original[r.start:end])
		word := d.engine.ExtractProfanity(snippet)
		if word == "" {
			from := max(0, r.start-3)
			to := min(len(original), r.end+3)
			word = d.engine.ExtractProfanity(string(original[from:to]))
		}
		if word == "" {
			word = fallbackWord(snippet)
		}
		if word == "" {
			continue
		}
		matches = append(matches, Match{
			Word:   word,
			Group:  word,
			Source: "go-away",
			Index:  r.start,
		})
	}
	return Result{Count: len(matches), Matches: matches}
}

type runeRange struct {
	start int
	end   int
}

func profanityRanges(censored []rune) []runeRange {
	var runs []runeRange
	for i := 0; i < len(censored); {
		if censored[i] != '*' {
			i++
			continue
		}
		start := i
		for i < len(censored) && censored[i] == '*' {
			i++
		}
		runs = append(runs, runeRange{start: start, end: i})
	}
	if len(runs) < 2 {
		return runs
	}

	merged := []runeRange{runs[0]}
	for _, run := range runs[1:] {
		last := &merged[len(merged)-1]
		if canMergeSpacedLetters(censored[last.start:run.end]) && run.start-last.end <= 4 {
			last.end = run.end
			continue
		}
		merged = append(merged, run)
	}
	return merged
}

func canMergeSpacedLetters(rs []rune) bool {
	stars := 0
	inRun := false
	for _, r := range rs {
		if r == '*' {
			if inRun {
				return false
			}
			stars++
			inRun = true
			continue
		}
		inRun = false
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return stars > 1
}

func fallbackWord(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
