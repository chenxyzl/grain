package message

import (
	"errors"
	"strconv"
)

type Code int32

const (
	CodeErr           Code = -1 //all err
	CodeActorNotFound Code = -2 //actor not found
	//CodeAskNotRunning means a blocking Ask was attempted outside the actor's
	//running phase — from Started() or from PreStop(). Check for it to distinguish
	//this programming error from runtime failures such as a timeout.
	CodeAskNotRunning Code = -3
)

// Error makes a Code an error value in its own right, so it can be the target of
// errors.Is:
//
//	if errors.Is(err, message.CodeActorNotFound) { ... }
//
// The obvious alternative — exporting shared *ErrCode sentinels — is deliberately not
// offered. ErrCode is a generated struct with EXPORTED Code and Des fields, so a caller
// doing the natural `err.Des = ctx + err.Des` would corrupt the sentinel process-wide
// (see the warning in grain/message.go). A Code is an immutable int32: there is nothing
// to corrupt. It is the same reason syscall.Errno is a value type.
//
// The text carries no description because a Code has none; a real failure's Des comes
// from the *ErrCode that wraps it.
func (c Code) Error() string {
	return "ErrCode, code:" + strconv.Itoa(int(c))
}

func (x *ErrCode) Error() string {
	return "ErrCode, code:" + strconv.Itoa(int(x.Code)) + ", des:" + x.Des
}

// Is matches by CODE only, ignoring Des, and is what makes errors.Is work on an
// ErrCode. Without it, the only way to test for a specific failure was to compare raw
// ints — `err.Code == int32(message.CodeActorNotFound)` — which does not unwrap, and
// leaks the int32/Code conversion into every call site.
//
// target may be a Code (the usual form) or another *ErrCode; in the latter case only
// the codes are compared, so two ErrCodes with the same code and different
// descriptions match. Anything else does not match.
func (x *ErrCode) Is(target error) bool {
	if x == nil {
		return false
	}
	switch t := target.(type) {
	case Code:
		return x.Code == int32(t)
	case *ErrCode:
		return t != nil && x.Code == t.Code
	}
	return false
}

// CodeOf pulls the Code out of an error chain, reporting false if there is none. Use it
// to switch over several codes at once, where a chain of errors.Is calls would be
// clumsy:
//
//	switch code, _ := message.CodeOf(err); code {
//	case message.CodeActorNotFound: ...
//	case message.CodeAskNotRunning: ...
//	}
func CodeOf(err error) (Code, bool) {
	var e *ErrCode
	if errors.As(err, &e) && e != nil {
		return Code(e.Code), true
	}
	var c Code
	if errors.As(err, &c) {
		return c, true
	}
	return 0, false
}

func WithErr(des string) *ErrCode {
	return &ErrCode{Code: int32(CodeErr), Des: des}
}

func WithErrCode(code Code, des ...string) *ErrCode {
	var allDes string
	for _, de := range des {
		if allDes == "" {
			allDes = de
		} else {
			allDes = allDes + "\n" + de
		}
	}
	return &ErrCode{Code: int32(code), Des: allDes}
}
