package logutil

import (
	"fmt"
	"os"

	"github.com/mgutz/ansi"
)

var (
	EnableDebug = false

	Yel   = ansi.ColorFunc("yellow")
	Green = ansi.ColorFunc("green")
	Red   = ansi.ColorFunc("red")
	Bold  = ansi.ColorFunc("white+b")
	Gray  = ansi.ColorFunc("black+h")
	Cyan  = ansi.ColorFunc("cyan")
)

// Prints to stderr.
func Debugf(format string, a ...interface{}) {
	if !EnableDebug {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "%s: ", Gray("debug"))
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", a...)
}

// Prints to stderr.
func Errorf(format string, a ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, "%s: ", Red("error"))
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", a...)
}

// Prints to stderr.
func Infof(format string, a ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, "%s: ", Yel("info"))
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", a...)
}
