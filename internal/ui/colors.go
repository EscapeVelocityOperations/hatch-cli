package ui

import (
	"fmt"
	"os"
	"sync"

	"golang.org/x/term"
)

var (
	colorEnabled bool
	colorOnce    sync.Once
)

func initColors() {
	colorOnce.Do(func() {
		colorEnabled = term.IsTerminal(int(os.Stdout.Fd()))
	})
}

const (
	reset  = "\033[0m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	bold   = "\033[1m"
	dim    = "\033[2m"
)

func colorize(color, s string) string {
	initColors()
	if colorEnabled {
		return color + s + reset
	}
	return s
}

func Red(s string) string    { return colorize(red, s) }
func Green(s string) string  { return colorize(green, s) }
func Yellow(s string) string { return colorize(yellow, s) }
func Blue(s string) string   { return colorize(blue, s) }
func Bold(s string) string   { return colorize(bold, s) }
func Dim(s string) string    { return colorize(dim, s) }

func Success(msg string) { fmt.Println(Green("✓ " + msg)) }
func Error(msg string)   { fmt.Println(Red("✗ " + msg)) }
func Warn(msg string)    { fmt.Println(Yellow("! " + msg)) }
func Info(msg string)    { fmt.Println(Blue("→ " + msg)) }
