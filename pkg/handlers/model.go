package handlers

import "bytetrade.io/web3os/backup-server/pkg/apiserver/response"

type proxyRequest struct {
	Op       string      `json:"op"`
	DataType string      `json:"datatype"`
	Version  string      `json:"version"`
	Group    string      `json:"group"`
	Param    interface{} `json:"param,omitempty"`
	Data     string      `json:"data,omitempty"`
	Token    string
}

type passwordResponse struct {
	response.Header
	Data *passwordResponseData `json:"data,omitempty"`
}

type passwordResponseData struct {
	Env   string `json:"env"`
	Name  string `json:"name"`
	Value string `json:"value"`
}
