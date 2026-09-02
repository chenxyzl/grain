package message

import (
	"errors"
	"strconv"
)

type Code int32

const (
	CodeErr           Code = -1 //all err
	CodeActorNotFound Code = -2 //actor not found
	//CodeAskNotRunning: a blocking Ask was attempted outside the actor's running phase (from
	//Started() or PreStop()). Distinguishes that programming error from runtime failures.
	CodeAskNotRunning Code = -3
)

// Error makes a Code an error value in its own right, so it can be an errors.Is target:
//
//	if errors.Is(err, message.CodeActorNotFound) { ... }
//
// Exported *ErrCode sentinels are deliberately not offered instead: ErrCode is generated with
// EXPORTED Code and Des fields, so `err.Des = ctx + err.Des` corrupts one process-wide (see
// grain/message.go); an immutable int32 has nothing to corrupt, the same reason syscall.Errno
// is a value type. The text has no description — a real failure's Des comes from its *ErrCode.
func (c Code) Error() string {
	return "ErrCode, code:" + strconv.Itoa(int(c))
}

func (x *ErrCode) Error() string {
	return "ErrCode, code:" + strconv.Itoa(int(x.Code)) + ", des:" + x.Des
}

// Is matches by CODE only, ignoring Des; that is what makes errors.Is work on an ErrCode
// instead of comparing raw int32s. target may be a Code (usual) or another *ErrCode, where only
// codes are compared, so a different description still matches. Anything else does not match.
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

// CodeOf pulls the Code out of an error chain, reporting false if there is none. Use it to
// switch over several codes at once, where a chain of errors.Is calls would be clumsy.
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
