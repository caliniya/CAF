package NetAPI

type NetAPIClient struct {
	Time TimeClient
}

func InitNetAPI() *NetAPIClient {
	return &NetAPIClient{Time: *NewTimeClient(DefaultTimeEndpoint)}
}
