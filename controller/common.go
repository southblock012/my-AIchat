package controller

import "my-AIchat/common/code"

type Response struct {
	StatusCode code.Code `json:"status_code"`
	StatusMsg  string    `json:"status_msg,omitempty"`
}

// SetCode 设置响应状态码和消息
func (r *Response) SetCode(code code.Code) Response {
	if nil == r {
		r = new(Response)
	}
	r.StatusCode = code
	r.StatusMsg = code.Msg()
	return *r
}

// SetSuccess 设置成功响应
func (r *Response) SetSuccess() {
	r.SetCode(code.CodeSuccess)
}
