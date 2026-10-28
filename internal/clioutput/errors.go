// Package clioutput defines the CLI's versioned result and error contracts.
// It does not depend on Cobra, terminal rendering, or the download engine.
package clioutput

import (
	"context"
	"errors"
	"net"
)

// Code identifies an error without requiring consumers to parse its message.
type Code string

const (
	OperationFailed  Code = "operation_failed"
	InvalidArguments Code = "invalid_arguments"
	PartialFailure   Code = "partial_failure"
	NotFound         Code = "not_found"
	Unauthorized     Code = "unauthorized"
	Unavailable      Code = "unavailable"
	Conflict         Code = "conflict"
	Timeout          Code = "timeout"
	Interrupted      Code = "interrupted"
	Terminated       Code = "terminated"
)

// Failure is the public, safe-to-display description of an operation's error.
// Message must not contain credentials, raw URLs, or remote response bodies.
type Failure struct {
	Code      Code   `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// Error attaches a public failure to an underlying diagnostic. The diagnostic
// remains available to legacy callers through Error and errors.Is/As.
type Error struct {
	Failure Failure
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Failure.Message
}

func (e *Error) Unwrap() error { return e.Cause }

// NewError classifies an error at the point where its meaning is known.
func NewError(code Code, message string, cause error) error {
	return &Error{Failure: Failure{Code: code, Message: message, Retryable: code == Unavailable}, Cause: cause}
}

// Describe never exposes an unclassified diagnostic in machine output.
func Describe(err error) Failure {
	var classified *Error
	if errors.As(err, &classified) {
		return classified.Failure
	}
	if errors.Is(err, context.Canceled) {
		return Failure{Code: Interrupted, Message: "The operation was interrupted."}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Failure{Code: Timeout, Message: "The operation timed out."}
	}
	var operationError *net.OpError
	var dnsError *net.DNSError
	if errors.As(err, &operationError) || errors.As(err, &dnsError) {
		return Failure{Code: Unavailable, Message: "The Surge server could not be reached.", Retryable: true}
	}
	return Failure{Code: OperationFailed, Message: "The operation failed."}
}

// ExitCode applies to the v1 contract. Legacy commands keep their existing
// success/1 behavior until they explicitly adopt the contract.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	switch Describe(err).Code {
	case InvalidArguments:
		return 2
	case PartialFailure:
		return 3
	case NotFound:
		return 4
	case Unauthorized:
		return 5
	case Unavailable:
		return 6
	case Conflict:
		return 7
	case Timeout:
		return 8
	case Interrupted:
		return 130
	case Terminated:
		return 143
	default:
		return 1
	}
}
