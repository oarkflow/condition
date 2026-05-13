package condition

import "fmt"

type ErrorKind string

const (
	ErrLex      ErrorKind = "lex"
	ErrParse    ErrorKind = "parse"
	ErrEval     ErrorKind = "eval"
	ErrMissing  ErrorKind = "missing_fact"
	ErrType     ErrorKind = "type_mismatch"
	ErrFunction ErrorKind = "unknown_function"
	ErrOperator ErrorKind = "unknown_operator"
	ErrRule     ErrorKind = "invalid_rule"
	ErrRuleSet  ErrorKind = "invalid_ruleset"
)

type Error struct {
	Kind    ErrorKind
	Message string
	Pos     int
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Pos > 0 {
		return fmt.Sprintf("%s error at %d: %s", e.Kind, e.Pos, e.Message)
	}
	return fmt.Sprintf("%s error: %s", e.Kind, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func newError(kind ErrorKind, pos int, format string, args ...any) *Error {
	return &Error{Kind: kind, Pos: pos, Message: fmt.Sprintf(format, args...)}
}
