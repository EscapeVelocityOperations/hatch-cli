package ui

import "fmt"

const (
	reset  = "\033[0m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	bold   = "\033[1m"
	dim    = "\033[2m"
)

func Red(s string) string    { return red + s + reset }
func Green(s string) string  { return green + s + reset }
func Yellow(s string) string { return yellow + s + reset }
func Blue(s string) string   { return blue + s + reset }
func Bold(s string) string   { return bold + s + reset }
func Dim(s string) string    { return dim + s + reset }

func Success(msg string) { fmt.Println(Green("✓ " + msg)) }
func Error(msg string)   { fmt.Println(Red("✗ " + msg)) }
func Warn(msg string)    { fmt.Println(Yellow("! " + msg)) }
func Info(msg string)    { fmt.Println(Blue("→ " + msg)) }
