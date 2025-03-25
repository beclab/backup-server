package v1

import (
	"strconv"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/config"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/response"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	"bytetrade.io/web3os/backup-server/pkg/storage"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	"github.com/emicklei/go-restful/v3"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type Handler struct {
	cfg                 *config.Config
	factory             client.Factory
	veleroBackupManager velero.Manager
	handler             handlers.Interface
}

func New(cfg *config.Config, factory client.Factory, handler handlers.Interface) *Handler {
	return &Handler{
		cfg:                 cfg,
		factory:             factory,
		veleroBackupManager: velero.NewManager(factory),
		handler:             handlers.NewHandler(factory),
	}
}

func (h *Handler) health(req *restful.Request, resp *restful.Response) {
	response.SuccessNoData(resp)
}

func (h *Handler) ready(req *restful.Request, resp *restful.Response) {
	resp.Write([]byte("ok"))
}

func (h *Handler) init(req *restful.Request, resp *restful.Response) {
	response.SuccessNoData(resp)
}

func (h *Handler) available(req *restful.Request, resp *restful.Response) {
	response.SuccessNoData(resp)
}

func (h *Handler) listBackup(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()
	owner := req.HeaderParameter(constant.DefaultOwnerHeaderKey)
	// p := req.QueryParameter("page")
	// l := req.QueryParameter("limit")

	backups, err := h.handler.GetBackupHandler().ListBackups(ctx, owner, 0, 5)
	if err != nil {
		log.Errorf("get backups error %v", err)
		response.HandleError(resp, err)
		return
	}

	labelsSelector := h.handler.GetBackupHandler().GetBackupIdForLabels(backups)
	var allSnapshots = new(sysv1.SnapshotList)
	for _, ls := range labelsSelector {
		snapshots, err := h.handler.GetSnapshotHandler().ListSnapshots(ctx, 0, ls, "")
		if err != nil {
			log.Errorf("get snapshots error %v", err)
			continue
		}
		if snapshots == nil || len(snapshots.Items) == 0 {
			continue
		}
		allSnapshots.Items = append(allSnapshots.Items, snapshots.Items...)
	}

	response.Success(resp, parseResponseBackupList(backups, allSnapshots))
}

func (h *Handler) get(req *restful.Request, resp *restful.Response) {
	ctx, id := req.Request.Context(), req.PathParameter("id")
	// owner := req.HeaderParameter(velero.BackupOwnerHeaderKey)
	owner := "zhaoyu001"
	_ = owner

	backup, err := h.handler.GetBackupHandler().GetBackupById(ctx, id)
	if err != nil {
		response.HandleError(resp, errors.WithMessage(err, "describe backup"))
		return
	}

	response.Success(resp, parseResponseBackupDetail(backup))
}

func (h *Handler) addBackup(req *restful.Request, resp *restful.Response) {
	var (
		err error
		b   BackupCreate
	)

	if err = req.ReadEntity(&b); err != nil {
		response.HandleError(resp, errors.WithStack(err))
		return
	}

	ctx := req.Request.Context()
	owner := req.HeaderParameter(constant.DefaultOwnerHeaderKey) // ! debug
	owner = "zhaoyu001"

	log.Debugf("received backup create request: %s", util.ToJSON(b))

	if b.Location == "" || b.LocationConfig == nil {
		response.HandleError(resp, errors.New("backup location is required"))
		return
	}

	if b.BackupPolicies == nil {
		response.HandleError(resp, errors.New("backup policy is required"))
		return
	}

	// if backup is exists
	backup, err := h.handler.GetBackupHandler().GetBackup(ctx, owner, b.Name) // new plan
	if err != nil {
		response.HandleError(resp, errors.Errorf("failed to get backup %q: %v", b.Name, err))
		return
	}

	if backup != nil {
		response.HandleError(resp, errors.New("the backup plan "+b.Name+" already exists"))
		return
	}

	if err = NewBackupPlan(owner, h.factory, h.veleroBackupManager, h.handler).Apply(ctx, &b); err != nil {
		response.HandleError(resp, errors.Errorf("failed to create backup %q: %v", b.Name, err))
		return
	}

	response.SuccessNoData(resp)
}

func (h *Handler) update(req *restful.Request, resp *restful.Response) {
	var (
		err error
		b   BackupCreate
	)

	if err = req.ReadEntity(&b); err != nil {
		response.HandleError(resp, err)
		return
	}

	name, owner := req.PathParameter("name"), req.HeaderParameter(constant.DefaultOwnerHeaderKey)
	ctx := req.Request.Context()
	b.Name = name

	log.Debugf("received backup update request: %s", util.PrettyJSON(b))

	format := "failed to update backup plan %q"

	backup, err := h.handler.GetBackupHandler().GetBackup(ctx, owner, name)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.WithMessage(err, "failed to update backup"))
		return
	}

	if backup == nil {
		response.HandleError(resp, errors.Errorf(format+", not found", name))
		return
	}

	if err = NewBackupPlan(owner, h.factory, h.veleroBackupManager, h.handler).Update(ctx, &b, backup); err != nil {
		response.HandleError(resp, errors.WithMessagef(err, format, name))
		return
	}

	r := &ResponseDescribeBackup{
		Name:           name,
		BackupPolicies: b.BackupPolicies,
	}

	response.Success(resp, r)
}

func (h *Handler) listSnapshots(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()

	var limit int64 = 10

	backupId := req.PathParameter("id")
	q := req.QueryParameter("limit")
	if q != "" {
		v, err := strconv.ParseInt(q, 10, 64)
		if err != nil {
			log.Warnf("list snapshot, invalid limit parameter: %q", q)
		} else {
			limit = v
		}
	}

	var labelSelector = "backup-id=" + backupId
	var snapshots, err = h.handler.GetSnapshotHandler().ListSnapshots(ctx, limit, labelSelector, "")
	if err != nil {
		response.HandleError(resp, errors.WithMessage(err, "get snapshots error"))
		return
	}

	if snapshots == nil || len(snapshots.Items) == 0 {
		response.HandleError(resp, errors.New("snapshots not exists"))
		return
	}

	response.Success(resp, parseResponseSnapshotList(snapshots))
}

func (h *Handler) getSnapshot(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()
	id := req.PathParameter("id")

	snapshot, err := h.handler.GetSnapshotHandler().GetSnapshot(ctx, id)
	if err != nil {
		response.HandleError(resp, errors.Errorf("snapshot %s not found", id))
		return
	}

	if snapshot == nil {
		response.HandleError(resp, errors.WithMessage(err, "snapshots not exists"))
		return
	}

	response.Success(resp, parseResponseSnapshotDetail(snapshot))
}

func (h *Handler) deleteSnapshot(req *restful.Request, resp *restful.Response) {
	// 	ctx := req.Request.Context()
	// 	name := req.PathParameter("name")

	// 	b, err := h.veleroBackupManager.GetSysBackup(ctx, name)
	// 	if err != nil {
	// 		response.HandleError(resp, err)
	// 		return
	// 	}

	// 	plan := req.PathParameter("plan_name")

	// 	if bcName, ok := b.Labels[velero.LabelBackupConfig]; ok && bcName != "" && bcName == plan {
	// 		sc, err := h.factory.Sysv1Client()
	// 		if err != nil {
	// 			response.HandleError(resp, errors.WithMessagef(err, "delete snapshot %q", name))
	// 			return
	// 		}

	// 		// to delete full backup, must delete all increment backup first
	// 		refbackups, err := h.getAllIncrementBackups(ctx, b.Namespace, name, string(b.UID))
	// 		if err != nil {
	// 			response.HandleError(resp, errors.WithMessagef(err, "delete snapshot %q", name))
	// 			return
	// 		}

	// 		if len(refbackups) > 0 {
	// 			response.HandleError(resp, errors.WithMessagef(errors.New("has more increment backups to be deleted"), "more increment backups refer to %q", name))
	// 			return
	// 		}

	// 		err = sc.SysV1().Backups(b.Namespace).
	// 			Delete(ctx, name, metav1.DeleteOptions{})
	// 		if err != nil {
	// 			response.HandleError(resp, errors.WithMessagef(err, "delete snapshot %q", name))
	// 			return
	// 		}
	// 	}

	// response.SuccessNoData(resp)
}

func (h *Handler) deleteBackupPlan(req *restful.Request, resp *restful.Response) {
	// 	ctx, name := req.Request.Context(), req.PathParameter("name")

	// 	log.Debugf("delete backup %q", name)

	// 	sc, err := h.factory.Sysv1Client()
	// 	if err != nil {
	// 		response.HandleError(resp, errors.WithMessagef(err, "new client"))
	// 		return
	// 	}
	// 	ns := h.veleroBackupManager.Namespace()
	// 	if err = sc.SysV1().BackupConfigs(ns).
	// 		Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
	// 		log.Warnf("deleting bc %q, %v", name, err)
	// 	}

	// 	response.SuccessNoData(resp)
}

func (h *Handler) getSpaceRegions(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()
	owner := req.HeaderParameter(constant.DefaultOwnerHeaderKey)
	owner = "zhaoyu001"

	olaresId, err := h.handler.GetSnapshotHandler().GetOlaresId(owner)
	if err != nil {
		response.HandleError(resp, errors.WithMessagef(err, "get olares id error"))
		return
	}

	storage := storage.NewStorage(h.factory, owner)
	regions, err := storage.GetRegions(ctx, olaresId)
	if err != nil {
		response.HandleError(resp, err)
		return
	}

	response.Success(resp, parseResponseSpaceRegions(regions))
}

func (h *Handler) listRestore(req *restful.Request, resp *restful.Response) {
	response.SuccessNoData(resp)
}

func (h *Handler) addRestore(req *restful.Request, resp *restful.Response) {
	var (
		err error
		b   RestoreCreate
	)

	if err = req.ReadEntity(&b); err != nil {
		response.HandleError(resp, errors.WithStack(err))
		return
	}

	ctx := req.Request.Context()
	owner := req.HeaderParameter(constant.DefaultOwnerHeaderKey)
	owner = "zhaoyu001"
	_ = owner

	snapshot, err := h.handler.GetSnapshotHandler().GetSnapshot(ctx, b.SnapshotId)
	if err != nil {
		response.HandleError(resp, errors.Errorf("failed to get snapshot %s: %v", b.SnapshotId, err))
		return
	}

	if snapshot == nil {
		response.HandleError(resp, errors.Errorf("snapshot %s not exists", b.SnapshotId))
		return
	}

	backup, err := h.handler.GetBackupHandler().GetBackupById(ctx, snapshot.Spec.BackupId)
	if err != nil {
		response.HandleError(resp, errors.Errorf("failed to get backup %s: %v", snapshot.Spec.BackupId, err))
		return
	}

	if backup == nil {
		response.HandleError(resp, errors.Errorf("backup %s not exists", snapshot.Spec.BackupId))
		return
	}

	var restoreType = make(map[string]string)
	restoreType["path"] = b.Path

	if b.SnapshotId != "" {
		restoreType["snapshotId"] = b.SnapshotId
	} else if b.BackupUrl != "" {
		restoreType["backupUrl"] = b.BackupUrl
	} else {
		response.HandleError(resp, errors.Errorf("restore type invalid, snapshotId: %s, backupUrl: %s",
			b.SnapshotId, b.BackupUrl))
		return
	}

	_, err = h.handler.GetRestoreHandler().CreateRestore(ctx, constant.BackupTypeFile, restoreType)
	if err != nil {
		response.HandleError(resp, errors.Errorf("create restore task failed: %v", err))
		return
	}

	// todo
	response.SuccessNoData(resp)

}
