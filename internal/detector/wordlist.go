package detector

import (
	"bufio"
	"os"
	"strings"
	"sync"
	"unicode"
)

var (
	commonWordsOnce sync.Once
	commonWords     map[string]bool
)

func knownWord(word string) bool {
	commonWordsOnce.Do(loadCommonWords)
	return commonWords[word]
}

func loadCommonWords() {
	commonWords = map[string]bool{}
	for _, path := range []string{"/usr/share/dict/words", "/usr/dict/words"} {
		if loadWords(path) == nil && len(commonWords) > 0 {
			return
		}
	}
}

func loadWords(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		word := normalizeDictionaryWord(scanner.Text())
		if len(word) >= 4 {
			commonWords[word] = true
		}
	}
	return scanner.Err()
}

func normalizeDictionaryWord(word string) string {
	word = strings.ToLower(strings.TrimSpace(word))
	var b strings.Builder
	for _, r := range word {
		if unicode.IsLetter(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
