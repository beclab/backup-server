package monitor

import (
	"encoding/json"
	"net/http"
	"strings"

	httpx "bytetrade.io/web3os/backup-server/pkg/util/http"
)

var _ MonitorInterface = &Monitor{}

type MonitorInterface interface {
	BroadcastMessage(owner string, payload interface{}) error
}

var monitor *Monitor

type Monitor struct {
}

func NewMonitor() MonitorInterface {
	if monitor == nil {
		monitor = &Monitor{}
	}
	return monitor
}

func (w *Monitor) BroadcastMessage(owner string, payload interface{}) error {
	connRes, err := getConnections()
	if err != nil {
		return err
	}

	if len(connRes.Data) == 0 {
		return nil
	}

	users := make([]string, len(connRes.Data))
	for i := range connRes.Data {
		users[i] = connRes.Data[i].Name
	}

	requestBody := BroadcastRequest{
		Payload: payload,
		ConnID:  "",
		Users:   users,
	}

	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	bodyStr, err := httpx.SendHttpRequestWithMethod(http.MethodPost, broadcastUrl, strings.NewReader(string(requestBodyJSON)))
	if err != nil {
		return err
	}

	var response Response
	err = json.Unmarshal([]byte(bodyStr), &response)
	if err != nil {
		return err
	}

	return nil
}

func getConnections() (*ConnResponse, error) {
	bodyStr, err := httpx.SendHttpRequestWithMethod(http.MethodGet, getConnListUrl, nil)
	if err != nil {
		return nil, err
	}

	var response ConnResponse
	err = json.Unmarshal([]byte(bodyStr), &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}
