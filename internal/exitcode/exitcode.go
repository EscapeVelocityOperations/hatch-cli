// Package exitcode carries an explicit process exit status through a command
// error (h-59ej).
//
// By default the CLI exits 0 on errors when it is not attached to a terminal,
// so agents parse stderr instead (ec970bb). Commands whose failure must be
// visible to scripts and CI — hatch deploy when the build or rollout fails —
// wrap their error in an *Error, which main honors even outside a TTY.
package exitcode

import "errors"

// Error is an error with the exit status the process must end with.
type Error struct {
	Code int
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }

func (e *Error) Unwrap() error { return e.Err }

// Wrap returns err with exit status code; nil stays nil.
func Wrap(code int, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Code: code, Err: err}
}

// From returns the exit status carried by err, if any.
func From(err error) (int, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Code, true
	}
	return 0, false
}
