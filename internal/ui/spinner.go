package ui

import (
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type Spinner struct {
	message string
	stop    chan struct{}
	done    sync.WaitGroup
	isTTY   bool
}

func NewSpinner(message string) *Spinner {
	return &Spinner{
		message: message,
		stop:    make(chan struct{}),
		isTTY:   term.IsTerminal(int(os.Stderr.Fd())),
	}
}

func (s *Spinner) Start() {
	if !s.isTTY {
		// Not a TTY, just print the message once
		fmt.Fprint(os.Stderr, s.message+"...")
		return
	}

	s.done.Add(1)
	go func() {
		defer s.done.Done()
		i := 0
		for {
			select {
			case <-s.stop:
				fmt.Fprintf(os.Stderr, "\r\033[K") // Clear the line
				return
			default:
				fmt.Fprintf(os.Stderr, "\r%s %s", spinnerFrames[i%len(spinnerFrames)], s.message)
				i++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()
}

func (s *Spinner) Stop() {
	if !s.isTTY {
		fmt.Fprintln(os.Stderr, " done.")
		return
	}
	close(s.stop)
	s.done.Wait()
}
