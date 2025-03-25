package v1

import (
	"net/http"

	"bytetrade.io/web3os/backup-server/pkg/apiserver/config"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/response"
	apiruntime "bytetrade.io/web3os/backup-server/pkg/apiserver/runtime"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
)

var ModuleVersion = apiruntime.ModuleVersion{Name: "backup", Version: "v1"}

func AddContainer(cfg *config.Config, container *restful.Container) error {
	tags := []string{"backup"}

	ws := apiruntime.NewWebService(cfg, ModuleVersion)
	//ws.Consumes(restful.MIME_JSON)
	ws.Produces(restful.MIME_JSON)
	var factory = client.ClientFactory()

	handler := New(cfg, factory, handlers.NewHandler(factory))

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

	ws.Route(ws.GET("/plans/backup").
		To(handler.listBackup).
		Param(ws.QueryParameter("page", "page of plans").Required(false)).
		Param(ws.QueryParameter("limit", "page size of plans").Required(false)).
		Doc("list backup plans").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", ""))

	ws.Route(ws.GET("/plans/backup/{id}").
		To(handler.get).
		Param(ws.PathParameter("id", "backup plan id").DataType("string").Required(true)).
		Param(ws.HeaderParameter(constant.DefaultOwnerHeaderKey, "backup owner").
			DataType("string").Required(true)).
		Doc("describe backup plan").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", ""))

	// + add
	ws.Route(ws.POST("/plans/backup").
		To(handler.addBackup).
		Reads(BackupCreate{}).
		Param(ws.HeaderParameter(constant.DefaultOwnerHeaderKey, "backup owner").
			DataType("string").Required(false)).
		Doc("add backup plan").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	ws.Route(ws.DELETE("/plans/backup/{id}"). // todo
							To(handler.deleteBackupPlan).
							Param(ws.PathParameter("name", "backup plan id").DataType("string").Required(true)).
							Doc("delete backup").Metadata(restfulspec.KeyOpenAPITags, tags).
							Returns(http.StatusOK, "", "success"))

	ws.Route(ws.PUT("/plans/backup/{id}"). // todo
						To(handler.update).
						Reads(BackupCreate{}).
						Param(ws.PathParameter("id", "backup plan id").DataType("string").Required(true)).
						Param(ws.HeaderParameter(constant.DefaultOwnerHeaderKey, "backup owner").
							DataType("string").Required(true)).
						Doc("update backup plan").Metadata(restfulspec.KeyOpenAPITags, tags).
						Returns(http.StatusOK, "", "success"))

	ws.Route(ws.GET("/plans/backup/{id}/snapshots").
		Param(ws.PathParameter("id", "backup plan id").DataType("string").Required(true)).
		// Param(ws.QueryParameter("limit", "limit of snapshots").Required(false)). // todo
		To(handler.listSnapshots).
		Doc("list backup snapshots").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	ws.Route(ws.GET("/plans/backup/snapshots/{id}").
		Param(ws.PathParameter("id", "snapshot id").DataType("string").Required(true)).
		To(handler.getSnapshot).
		Doc("get backup snapshot details").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	ws.Route(ws.DELETE("/plans/backup/snapshots/{id}").
		Param(ws.PathParameter("id", "snapshot id").DataType("string").Required(true)).
		To(handler.deleteSnapshot).
		Doc("delete backup snapshot").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	// ws.Route(ws.GET("/plans/{plan_name}/backups").
	// 	To(handler.listBackups).
	// 	Doc("list backups").Metadata(restfulspec.KeyOpenAPITags, tags).
	// 	Returns(http.StatusOK, "", "success"))

	ws.Route(ws.GET("/plans/restore").
		To(handler.listRestore).
		Param(ws.QueryParameter("page", "page of plans").Required(false)).
		Param(ws.QueryParameter("limit", "page size of plans").Required(false)).
		Doc("list restore plans").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", ""))

	ws.Route(ws.POST("/plans/restore").
		To(handler.addRestore).
		Reads(RestoreCreate{}).
		Param(ws.HeaderParameter(constant.DefaultOwnerHeaderKey, "backup owner")).
		Doc("create restore task").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	// space regions
	ws.Route(ws.GET("/plans/regions").
		To(handler.getSpaceRegions).
		Doc("get space regions").Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(http.StatusOK, "", "success"))

	container.Add(ws)

	return nil
}
