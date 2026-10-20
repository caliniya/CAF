package platform

import (
	//"context"
	"errors"
)

var LoginFaildErr = errors.New("");

type QQPlatfrom interface{
	StartLogin(int) int;
}