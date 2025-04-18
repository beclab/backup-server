package monitor

const (
	broadcastUrl   = "http://localhost:40010/tapr/ws/conn/send"
	getConnListUrl = "http://localhost:40010/tapr/ws/conn/list"
)

type Connection struct {
	ID        string `json:"id"`
	UserAgent string `json:"userAgent"`
}

type UserConnection struct {
	Name  string       `json:"name"`
	Conns []Connection `json:"conns"`
}

type ConnResponse struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    []UserConnection `json:"data"`
}

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type BroadcastRequest struct {
	Payload interface{} `json:"payload"`
	ConnID  string      `json:"conn_id"`
	Users   []string    `json:"users"`
}
