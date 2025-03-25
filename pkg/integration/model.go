package integration

import "bytetrade.io/web3os/backup-server/pkg/apiserver/response"

var _ IntegrationToken = &IntegrationSpace{}

type IntegrationSpace struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	OlaresDid   string `json:"olares_did"`
	AccessToken string `json:"access_token"`
	ExpiresAt   int64  `json:"expires_at"`
	Available   bool   `json:"available"`
	Location    string `json:"location"`
}

func (i *IntegrationSpace) GetType() string {
	return i.Type
}

var _ IntegrationToken = &IntegrationCloud{}

type IntegrationCloud struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	Available bool   `json:"available"`
	Location  string `json:"location"`
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
