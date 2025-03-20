package storage

import "bytetrade.io/web3os/backup-server/pkg/apiserver/response"

type AccountResponse struct {
	response.Header
	Data *AccountResponseData `json:"data,omitempty"`
}

type AccountResponseRawData struct {
	ExpiresAt    int64  `json:"expires_at"`
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	Endpoint     string `json:"endpoint"`
	Bucket       string `json:"bucket"`
	UserId       string `json:"userid"`
	Available    bool   `json:"available"`
	CreateAt     int64  `json:"create_at"`
}

type AccountResponseData struct {
	Name     string                  `json:"name"`
	Type     string                  `json:"type"`
	RawData  *AccountResponseRawData `json:"raw_data"`
	CloudUrl string                  `json:"cloudUrl"`
}

type AccountsResponse struct {
	response.Header
	Data []*AccountsResponseRawData `json:"data,omitempty"`
}

type AccountsResponseRawData struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Available bool   `json:"available"`
	CreateAt  int64  `json:"create_at"`
}

type StorageSpaceConfig struct {
	OlaresDid              string
	OlaresSpaceAccessToken string
	Expired                int64
}

type StorageIntegrationConfig struct {
	Type      string `json:"type"`
	OlaresDid string `json:"olares_did"`
	AccessKey string `json:"access_token"`
	SecretKey string `json:"secret_key"`
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	Available bool   `json:"available"`
	ExpiresAt int64  `json:"expires_at"`
}
