package session

import (
	"my-AIchat/common/code"
	"my-AIchat/model"
	"net/http"
)

func GetUserSessionsByUserName(userName string) ([]model.Session, error) {

}

func CreateSessionAndSendMessage(userName string, userQuestion, modelType string) (string, string, code.Code) {

}

func CreateStreamSessionOnly(userName string) (string, code.Code) {

}

func StreamMessageToExistingSession(userName, sessionID, userQuestion, modelType string, writer http.ResponseWriter) code.Code {

}

func ChatSend(userName, sessionID, userQuestion, modelType string) (string, code.Code) {

}

func ChatStreamSend(userName, sessionID, userQuestion, modelType string, writer http.ResponseWriter) code.Code {

}
