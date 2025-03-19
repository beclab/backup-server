package storage

import "bytetrade.io/web3os/backup-server/pkg/apiserver/response"

type AccountResponse struct {
	response.Header
	Data *AccountResponseData `json:"data,omitempty"`
}

type AccountResponseRawData struct {
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	ExpiresAt    int64  `json:"expires_at"`
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

type AllAccountResponse struct {
	response.Header
	Data []*AccountsResponseRawData `json:"data,omitempty"`
}

type AccountsResponseRawData struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Available bool   `json:"available"`
	CreateAt  int64  `json:"create_at"`
}
