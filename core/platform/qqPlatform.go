package platform

import (
	//"context"
	"errors"
)

var LoginFaildErr = errors.New("");

type QQPlatfron interface{
	StartLogin(int) int;
}