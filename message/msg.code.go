package message

import "strconv"

type Code int32

const (
	CodeErr           Code = -1 //all err
	CodeActorNotFound Code = -2 //actor not found
	//CodeAskNotRunning means a blocking Ask was attempted outside the actor's
	//running phase — from Started() or from PreStop(). Check for it to distinguish
	//this programming error from runtime failures such as a timeout.
	CodeAskNotRunning Code = -3
)

func (x *ErrCode) Error() string {
	return "ErrCode, code:" + strconv.Itoa(int(x.Code)) + ", des:" + x.Des
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
