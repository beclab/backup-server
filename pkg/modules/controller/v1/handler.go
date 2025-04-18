package v1

import (
	"bytes"
	"fmt"
	"io"

	"bytetrade.io/web3os/backup-server/pkg/apiserver/config"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/response"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/emicklei/go-restful/v3"
)

type Handler struct {
	cfg     *config.Config
	factory client.Factory
	handler handlers.Interface
}

func New(cfg *config.Config, factory client.Factory, handler handlers.Interface) *Handler {
	return &Handler{
		cfg:     cfg,
		factory: factory,
		handler: handlers.NewHandler(factory),
	}
}

func (h *Handler) ready(req *restful.Request, resp *restful.Response) {
	resp.Write([]byte("ok"))
}

func (h *Handler) receiveWebsocketMessage(req *restful.Request, resp *restful.Response) {
	for key, values := range req.Request.Header {
		for _, value := range values {
			log.Infof("%s: %s\n", key, value)
		}
	}

	body, err := io.ReadAll(req.Request.Body)
	if err != nil {
		log.Errorf("read body err:%s", err.Error())
		response.HandleError(resp, fmt.Errorf("receive message error: %v", err))
		return
	}

	log.Infof("body: %s", string(body))
	req.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	response.Success(resp, "hello world")
}
