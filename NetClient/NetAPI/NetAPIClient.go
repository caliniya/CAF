package NetAPI

type NetAPIClient struct {
	TimeClient *TimeClient
}

func InitNetAPI() *NetAPIClient {
	return &NetAPIClient{TimeClient: NewTimeClient(DefaultTimeEndpoint)}
}
