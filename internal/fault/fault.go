package fault

import "errors"

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
	Status  int    `json:"-"`
}

func (problem *Error) Error() string {
	return problem.Message
}

func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func As(err error) *Error {
	var problem *Error
	if errors.As(err, &problem) {
		return problem
	}
	return New(500, "internal_error", "操作未完成，请重试或查看服务端诊断。")
}
