package v1

import (
	"context"
	"strconv"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/config"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/response"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/modules/backup/v1/operator"
	"bytetrade.io/web3os/backup-server/pkg/storage"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	"github.com/emicklei/go-restful/v3"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Handler struct {
	cfg                 *config.Config
	factory             client.Factory
	veleroBackupManager velero.Manager
	backupOperator      *operator.BackupOperator
	snapshotOperator    *operator.SnapshotOperator
}

func New(cfg *config.Config, factory client.Factory) *Handler {
	return &Handler{
		cfg:                 cfg,
		factory:             factory,
		veleroBackupManager: velero.NewManager(factory),
		backupOperator:      operator.NewBackupOperator(factory),
		snapshotOperator:    operator.NewSnapshotOperator(factory),
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

func (h *Handler) list(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()
	owner := req.HeaderParameter(constant.DefaultOwnerHeaderKey)
	// p := req.QueryParameter("page")
	// l := req.QueryParameter("limit")

	// todo support page
	backups, err := h.backupOperator.ListBackups(ctx, owner, 0, 5)
	if err != nil {
		log.Errorf("get backups error %v", err)
		response.HandleError(resp, err)
		return
	}

	labelsSelector := h.backupOperator.GetBackupIdForLabels(backups)
	var allSnapshots = new(sysv1.SnapshotList)
	for _, ls := range labelsSelector {
		snapshots, err := h.snapshotOperator.ListSnapshots(ctx, 0, ls, "")
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

	backup, err := h.backupOperator.GetBackupById(ctx, id)
	if err != nil {
		response.HandleError(resp, errors.WithMessage(err, "describe backup"))
		return
	}

	response.Success(resp, parseResponseBackupDetail(backup))
}

func (h *Handler) add(req *restful.Request, resp *restful.Response) {
	var (
		err error
		b   BackupCreate
	)

	if err = req.ReadEntity(&b); err != nil {
		response.HandleError(resp, errors.WithStack(err))
		return
	}

	ctx := req.Request.Context()
	// ! debug
	// owner := req.HeaderParameter(constant.DefaultOwnerHeaderKey)
	owner := "zhaoyu001"

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
	backup, err := h.backupOperator.GetBackup(ctx, owner, b.Name) // new plan
	if err != nil {
		response.HandleError(resp, errors.Errorf("failed to get backup %q: %v", b.Name, err))
		return
	}

	if backup != nil {
		response.HandleError(resp, errors.New("the backup plan "+b.Name+" already exists"))
		return
	}

	// check is exist backup in progress
	// todo
	// if _, err = h.veleroBackupManager.ExistRunningBackup(ctx); err != nil {
	// 	response.HandleError(resp, errors.Errorf("failed to create backup %q: %v", b.Name, err))
	// 	return
	// }

	if err = NewBackupPlan(owner, h.factory, h.veleroBackupManager, h.backupOperator).Apply(ctx, &b); err != nil {
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

	backup, err := h.backupOperator.GetBackup(ctx, owner, name)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.WithMessage(err, "failed to update backup"))
		return
	}

	if backup == nil {
		response.HandleError(resp, errors.Errorf(format+", not found", name))
		return
	}

	if err = NewBackupPlan(owner, h.factory, h.veleroBackupManager, h.backupOperator).Update(ctx, &b, backup); err != nil {
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
	var snapshots, err = h.snapshotOperator.ListSnapshots(ctx, limit, labelSelector, "")
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

	snapshot, err := h.snapshotOperator.GetSnapshot(ctx, id)
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

func (h *Handler) getAllIncrementBackups(ctx context.Context, namespace, name, uid string) ([]*sysv1.Backup, error) {
	c, err := h.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	backups, err := c.SysV1().Backups(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var res []*sysv1.Backup

	for _, elem := range backups.Items {
		if elem.Spec.Extra != nil {
			extra := elem.Spec.Extra
			if backupType, ok := extra[velero.ExtraBackupType]; ok && backupType == velero.IncrementalBackup {
				refUid, ok1 := extra[velero.ExtraRefFullyBackupUid]
				refName, ok2 := extra[velero.ExtraRefFullyBackupName]
				if ok1 && ok2 && refUid == uid && refName == name {
					res = append(res, &elem)
				}
			}
		}
	}
	return res, nil
}

func (h *Handler) getSpaceRegions(req *restful.Request, resp *restful.Response) {
	// ctx := req.Request.Context()
	// owner := req.HeaderParameter(constant.DefaultOwnerHeaderKey)
	owner := "zhaoyu001"

	olaresId, err := h.snapshotOperator.GetOlaresId(owner)
	if err != nil {
		response.HandleError(resp, errors.WithMessagef(err, "get olares id error"))
		return
	}

	storage := storage.NewStorage(h.factory, owner, olaresId)
	regions, err := storage.GetRegions()
	if err != nil {
		response.HandleError(resp, err)
		return
	}

	response.Success(resp, regions)
}

func (h *Handler) restoreSnapshot(req *restful.Request, resp *restful.Response) {

}
