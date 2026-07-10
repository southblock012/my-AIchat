package email

import (
	"fmt"
	"my-AIchat/config"

	"gopkg.in/gomail.v2"
)

const (
	CodeMsg     = "GopherAI验证码如下(验证码仅限于2分钟有效): "
	UserNameMsg = "GopherAI的账号如下，请保留好，后续可以用账号/邮箱登录 "
)

func SendMail(email string, code string, msg string) error {
	m := gomail.NewMessage()
	//发件人
	m.SetHeader("From", config.GetConfig().EmailConfig.Email)
	//收件人
	m.SetHeader("To", email)
	//主题
	m.SetHeader("Subject", "来自GopherAI的验证码")
	//内容
	m.SetBody("text/plain", msg+" "+code)
	//配置SMTP服务器
	d := gomail.NewDialer("smtp.qq.com", 587, config.GetConfig().EmailConfig.Email, config.GetConfig().EmailConfig.AuthCode)
	//发送邮件
	if err := d.DialAndSend(m); err != nil {
		fmt.Printf("DialAndSend err %v:\n", err)
		return err
	}
	fmt.Printf("send mail success\n")
	return nil
}
