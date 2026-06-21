package code

//code响应状态码

type Code int64

const (
	CodeSuccess Code = 1000 //成功

	CodeInvalidParams    Code = 2001 //请求参数错误
	CodeUserExist        Code = 2002 //用户名已存在
	CodeUserNotExist     Code = 2003 //用户不存在
	CodeInvalidPassword  Code = 2004 //用户名或密码错误
	CodeNotMatchPassword Code = 2005 //两次密码不一致
	CodeInvalidToken     Code = 2006 //无效的Token
	CodeNotLogin         Code = 2007 //用户未登录
	CodeInvalidCaptcha   Code = 2008 //验证码错误
	CodeRecordNotFound   Code = 2009 //记录不存在
	CodeIllegalPassword  Code = 2010 //密码不合法

	CodeForbidden Code = 3001 //权限不足

	CodeServerBusy Code = 4001 //服务繁忙

	AIModelNotFind    Code = 5001 //模型不存在
	AIModelCannotOpen Code = 5002 //无法打开模型
	AIModelFail       Code = 5003 //模型运行失败

	TTSFail Code = 6001 //语音服务失败
)

var msg = map[Code]string{
	CodeSuccess: "success",

	CodeInvalidParams:    "请求参数错误",
	CodeUserExist:        "用户名已存在",
	CodeUserNotExist:     "用户不存在",
	CodeInvalidPassword:  "用户名或密码错误",
	CodeNotMatchPassword: "两次密码不一致",
	CodeInvalidToken:     "无效的Token",
	CodeNotLogin:         "用户未登录",
	CodeInvalidCaptcha:   "验证码错误",
	CodeRecordNotFound:   "记录不存在",
	CodeIllegalPassword:  "密码不合法",

	CodeForbidden: "权限不足",

	CodeServerBusy: "服务繁忙",

	AIModelNotFind:    "模型不存在",
	AIModelCannotOpen: "无法打开模型",
	AIModelFail:       "模型运行失败",
	TTSFail:           "语音服务失败",
}

//ToInt64 转换为int64类型
func (code Code) ToInt64() int64 {
	return int64(code)
}

// Msg 获取响应消息
func (code Code) Msg() string {
	if m, ok := msg[code]; ok {
		return m
	}
	return msg[CodeServerBusy]
}
