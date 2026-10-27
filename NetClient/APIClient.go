package netclient

import (
	"net/http"
	"strings"
)

type APIClient struct {
	Client *http.Client
	Config *Options

	SendText   string
	ReturnText string
}

type Options struct {
	ServerAddress string
	APIKey        string
	Model         string
}

func (c *APIClient) InitAPIClient() error {
	c.Client = &http.Client{}

	return nil
}

func (c *APIClient) SetConfig(cfg *Options) {
	c.Config = cfg
}

func (c *APIClient) Set(text string) {
	re, err := http.NewRequest("POST", c.Config.ServerAddress, strings.NewReader(text))
	if err != nil {
		return
	}
	c.post(re)
}

func (c *APIClient) post(req *http.Request) {
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+c.Config.APIKey)
}

func (c *APIClient) Send() error {
	return nil
}
