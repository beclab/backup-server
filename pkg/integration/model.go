package integration

import "bytetrade.io/web3os/backup-server/pkg/apiserver/response"

var _ IntegrationToken = &IntegrationSpace{}

type IntegrationSpace struct {
	Name        string
	Type        string
	OlaresDid   string
	AccessToken string
	ExpiresAt   int64
	Available   bool
	Location    string
}

func (i *IntegrationSpace) GetType() string {
	return i.Type
}

var _ IntegrationToken = &IntegrationCloud{}

type IntegrationCloud struct {
	Name      string
	Type      string
	AccessKey string
	SecretKey string
	Endpoint  string
	Bucket    string
	Available bool
	Location  string
}

func (i *IntegrationCloud) GetType() string {
	return i.Type
}

//

type accountResponse struct {
	response.Header
	Data *accountResponseData `json:"data,omitempty"`
}

type accountResponseData struct {
	Name     string                  `json:"name"`
	Type     string                  `json:"type"`
	RawData  *accountResponseRawData `json:"raw_data"`
	CloudUrl string                  `json:"cloudUrl"`
}

type accountResponseRawData struct {
	ExpiresAt    int64  `json:"expires_at"`
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	Endpoint     string `json:"endpoint"`
	Bucket       string `json:"bucket"`
	UserId       string `json:"userid"`
	Available    bool   `json:"available"`
	CreateAt     int64  `json:"create_at"`
}
