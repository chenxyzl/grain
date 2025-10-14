package message

import "strconv"

type Code int32

const (
	codeOk            Code = 0  //ok
	CodeErr           Code = -1 //all err
	CodeActorNotFound Code = -2 //actor not found
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
