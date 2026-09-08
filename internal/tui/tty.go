package tui

import (
	"os"

	"golang.org/x/term"
)

func HasTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
