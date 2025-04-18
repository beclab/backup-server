package v1

import (
	"net/http"

	"bytetrade.io/web3os/backup-server/pkg/apiserver/config"
	apiruntime "bytetrade.io/web3os/backup-server/pkg/apiserver/runtime"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
)

var ModuleVersion = apiruntime.ModuleVersion{Name: "backup", Version: "v1"}

func AddContainer(cfg *config.Config, container *restful.Container) error {
	tags := []string{"backup", "monitor"}

	ws := apiruntime.NewWebService(cfg, ModuleVersion)
	//ws.Consumes(restful.MIME_JSON)
	ws.Produces(restful.MIME_JSON)
	var factory = client.ClientFactory()

	handler := New(cfg, factory, handlers.NewHandler(factory))

	ws.Route(ws.GET("/monitor/ready").
		To(handler.ready).
		Doc("Ready status.").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "ok"))

	ws.Route(ws.POST("/monitor/message").
		To(handler.receiveWebsocketMessage).
		Doc("receive websocket message for sidecar").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "Success to receive websocket message", nil))

	container.Add(ws)

	return nil
}
