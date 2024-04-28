package v1

import (
	"net/http"

	"bytetrade.io/web3os/backup-server/pkg/apiserver/config"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/response"
	apiruntime "bytetrade.io/web3os/backup-server/pkg/apiserver/runtime"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
)

var ModuleVersion = apiruntime.ModuleVersion{Name: "backup", Version: "v1"}

func AddContainer(cfg *config.Config, container *restful.Container) error {
	tags := []string{"backup"}

	ws := apiruntime.NewWebService(cfg, ModuleVersion)
	//ws.Consumes(restful.MIME_JSON)
	ws.Produces(restful.MIME_JSON)

	handler := New(cfg, client.ClientFactory())

	ws.Route(ws.GET("/ready").
		To(handler.ready).
		Doc("Ready status.").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "ok"))

	ws.Route(ws.GET("/init").
		To(handler.init).
		Returns(http.StatusOK, "", response.Response{}))

	ws.Route(ws.GET("/available").
		To(handler.available).
		Doc("backup server status").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	ws.Route(ws.GET("/plans").
		To(handler.list).
		Doc("list backup plans").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", ""))

	ws.Route(ws.GET("/plans/{name}").
		To(handler.get).
		Param(ws.PathParameter("name", "backup plan name").DataType("string").Required(true)).
		Param(ws.HeaderParameter(velero.BackupOwnerHeaderKey, "backup owner").
			DataType("string").Required(true)).
		Doc("describe backup plan").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", ""))

	ws.Route(ws.POST("/plans").
		To(handler.add).
		Reads(BackupCreate{}).
		Param(ws.HeaderParameter(velero.BackupOwnerHeaderKey, "backup owner").
			DataType("string").Required(true)).
		Doc("add backup plan").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	ws.Route(ws.DELETE("/plans/{name}").
		To(handler.deleteBackupPlan).
		Param(ws.PathParameter("name", "backup name").DataType("string").Required(true)).
		Doc("delete backup").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	ws.Route(ws.PUT("/plans/{name}").
		To(handler.update).
		Reads(BackupCreate{}).
		Param(ws.PathParameter("name", "backup plan name").DataType("string").Required(true)).
		Param(ws.HeaderParameter(velero.BackupOwnerHeaderKey, "backup owner").
			DataType("string").Required(true)).
		Doc("update backup plan").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	ws.Route(ws.GET("/plans/{plan_name}/snapshots").
		Param(ws.QueryParameter("limit", "limit of snapshots").Required(false)).
		To(handler.listSnapshots).
		Doc("list backup snapshots").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	ws.Route(ws.GET("/plans/{plan_name}/snapshots/{name}").
		Param(ws.PathParameter("name", "snapshot name").DataType("string").Required(true)).
		To(handler.getSnapshot).
		Doc("get backup snapshot details").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	ws.Route(ws.DELETE("/plans/{plan_name}/snapshots/{name}").
		Param(ws.PathParameter("name", "snapshot name").DataType("string").Required(true)).
		To(handler.deleteSnapshot).
		Doc("delete backup snapshot").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	ws.Route(ws.GET("/plans/{plan_name}/backups").
		To(handler.listBackups).
		Doc("list backups").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	container.Add(ws)

	return nil
}
