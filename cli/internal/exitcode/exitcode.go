// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

// Package exitcode defines the CLI exit code constants and an error
// type that maps domain errors to exit codes. See
// docs/analysis-cli-contract.md Section 1.
package exitcode

import "fmt"

// Exit codes. Codes 0-2 follow standard Unix conventions; 3-6 are
// Renotify-specific and map 1:1 to ErrorResponse.code values.
const (
	Success     = 0 // Command completed normally.
	Error       = 1 // General error (daemon unreachable, I/O, config invalid).
	Usage       = 2 // Invalid flags, missing args (Cobra default).
	Timeout     = 3 // Blocking ask expired without response.
	RateLimited = 4 // Per-flow notification rate limit exceeded.
	Unroutable  = 5 // No mobile client connected.
	NotFound    = 6 // Referenced flow, notification, or token not found.
)

// CodedError is an error that carries an exit code. The CLI's
// top-level error handler inspects this to choose the process exit
// code.
type CodedError struct {
	Code    int
	Message string
}

func (e *CodedError) Error() string {
	return e.Message
}

// Errorf creates a CodedError with a formatted message.
func Errorf(code int, format string, args ...any) *CodedError {
	return &CodedError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// ExitCode extracts the exit code from an error. If the error is a
// CodedError, its code is returned. Otherwise, Error (1) is
// returned as the default.
func ExitCode(err error) int {
	if err == nil {
		return Success
	}
	if coded, ok := err.(*CodedError); ok {
		return coded.Code
	}
	return Error
}
