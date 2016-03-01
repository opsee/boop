package cmd

import (
	"fmt"
	"regexp"
)

type BoopError struct {
	s           string
	userError   bool
	systemError bool
}

func (e BoopError) Error() string {
	return e.s
}

func (e BoopError) isUserError() bool {
	return e.userError
}

func NewUserError(a ...interface{}) BoopError {
	return BoopError{s: fmt.Sprintln(a...), userError: true}
}

func NewUserErrorF(format string, a ...interface{}) BoopError {
	return BoopError{s: fmt.Sprintf(format, a...), userError: true}
}

func NewSystemError(a ...interface{}) BoopError {
	return BoopError{s: fmt.Sprintln(a...), userError: false}
}

func NewSystemErrorF(format string, a ...interface{}) BoopError {
	return BoopError{s: fmt.Sprintf(format, a...), userError: false}
}

// catch some of the obvious user errors from Cobra.
// We don't want to show the usage message for every error.
// The below may be to generic. Time will show.
var userErrorRegexp = regexp.MustCompile("argument|flag|shorthand")

func IsUserError(err error) bool {
	if cErr, ok := err.(BoopError); ok && cErr.isUserError() {
		return true
	}

	return userErrorRegexp.MatchString(err.Error())
}
