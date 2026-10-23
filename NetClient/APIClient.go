package netclient

import (
	"errors"
	"net/http"
)

var Client *http.Client

func InitAPIClient() error {

	Client = &http.Client{}

	return errors.New("1111111111")
}
