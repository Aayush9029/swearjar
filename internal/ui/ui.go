package ui

import (
	"fmt"
	"os"
)

const (
	Green  = "\033[32m"
	Red    = "\033[31m"
	Yellow = "\033[33m"
	Cyan   = "\033[36m"
	Blue   = "\033[34m"
	Dim    = "\033[2m"
	Bold   = "\033[1m"
	Reset  = "\033[0m"
)

func IsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func Header(msg string) {
	if !IsTTY() {
		return
	}
	fmt.Fprintf(os.Stderr, "%s%s⚡ %s%s\n", Cyan, Bold, msg, Reset)
}

func Status(msg string) {
	if IsTTY() {
		fmt.Fprintf(os.Stderr, "%s→ %s%s\n", Dim, msg, Reset)
	}
}

func Success(msg string) {
	fmt.Fprintf(os.Stderr, "%s✓ %s%s\n", Green, msg, Reset)
}

func Fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s✗ %s%s\n", Red, fmt.Sprintf(format, args...), Reset)
	os.Exit(1)
}
