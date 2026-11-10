package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type Spinner struct {
	mu      sync.Mutex
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
		fmt.Fprint(os.Stderr, withEllipsis(s.message))
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
				s.mu.Lock()
				msg := s.message
				s.mu.Unlock()
				fmt.Fprintf(os.Stderr, "\r\033[K%s %s", spinnerFrames[i%len(spinnerFrames)], msg)
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

// Update replaces the message of a started spinner. Outside a TTY the new
// message goes on its own line.
func (s *Spinner) Update(message string) {
	s.mu.Lock()
	changed := message != s.message
	s.message = message
	s.mu.Unlock()
	if !s.isTTY && changed {
		fmt.Fprint(os.Stderr, "\n"+withEllipsis(message))
	}
}

// withEllipsis appends "..." unless the message already ends with it.
func withEllipsis(message string) string {
	if strings.HasSuffix(message, "...") {
		return message
	}
	return message + "..."
}
