package v1

import (
	"fmt"
	"strings"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/config"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/response"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	"bytetrade.io/web3os/backup-server/pkg/storage"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/emicklei/go-restful/v3"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

func (h *Handler) health(_ *restful.Request, resp *restful.Response) {
	response.SuccessNoData(resp)
}

func (h *Handler) ready(req *restful.Request, resp *restful.Response) {
	resp.Write([]byte("ok"))
}

func (h *Handler) listBackup(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()
	owner := req.HeaderParameter(constant.BflUserKey)
	limit := req.QueryParameter("limit")
	offset := req.QueryParameter("offset")

	backups, err := h.handler.GetBackupHandler().ListBackups(ctx, owner, offset, util.ParseToInt64(limit))
	if err != nil {
		log.Errorf("get backups error: %v", err)
		response.HandleError(resp, errors.WithMessage(err, "get backup error"))
		return
	}

	labelsSelector := h.handler.GetBackupHandler().GetBackupIdForLabels(backups)
	var allSnapshots = new(sysv1.SnapshotList)
	for _, ls := range labelsSelector {
		snapshots, err := h.handler.GetSnapshotHandler().ListSnapshots(ctx, "", 0, ls, "")
		if err != nil {
			log.Errorf("get snapshots error: %v", err)
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
	ctx, backupId := req.Request.Context(), req.PathParameter("id")
	owner := req.HeaderParameter(constant.BflUserKey)
	_ = owner

	backup, err := h.handler.GetBackupHandler().GetById(ctx, backupId)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.WithMessage(err, fmt.Sprintf("get backup %s error", backupId)))
		return
	}

	if apierrors.IsNotFound(err) {
		response.HandleError(resp, fmt.Errorf("backup %s not found", backupId))
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
	owner := req.HeaderParameter(constant.BflUserKey)

	log.Debugf("received backup create request: %s", util.ToJSON(b))

	if b.Location == "" || b.LocationConfig == nil {
		response.HandleError(resp, errors.New("backup location is required"))
		return
	}

	if b.BackupPolicies == nil || b.BackupPolicies.SnapshotFrequency == "" || b.BackupPolicies.TimesOfDay == "" {
		response.HandleError(resp, errors.New("backup policy is required"))
		return
	}

	var getLabel = "name=" + util.MD5(b.Name) + ",owner=" + owner
	backup, err := h.handler.GetBackupHandler().GetByLabel(ctx, getLabel)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.Errorf("failed to get backup %q: %v", b.Name, err))
		return
	}

	if backup != nil {
		response.HandleError(resp, errors.New("backup plan "+b.Name+" already exists"))
		return
	}

	var policy = fmt.Sprintf("%s_%s_%d_%d", b.BackupPolicies.SnapshotFrequency, b.BackupPolicies.TimesOfDay, b.BackupPolicies.DayOfWeek, b.BackupPolicies.DateOfMonth)
	getLabel = "owner=" + owner + ",policy=" + util.MD5(policy)
	backup, err = h.handler.GetBackupHandler().GetByLabel(ctx, getLabel)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.Errorf("failed to get backup %q: %v", b.Name, err))
		return
	}

	if backup != nil {
		response.HandleError(resp, errors.New("there are other backup tasks at the same time"))
		return
	}

	if err = NewBackupPlan(owner, h.factory, h.handler).Apply(ctx, &b); err != nil {
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

	backupId, owner := req.PathParameter("id"), req.HeaderParameter(constant.BflUserKey)
	ctx := req.Request.Context()
	b.Name = backupId

	log.Debugf("received backup update request: %s", util.PrettyJSON(b))

	format := "failed to update backup plan %q"

	backup, err := h.handler.GetBackupHandler().GetById(ctx, backupId)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.WithMessage(err, fmt.Sprintf("get backup %s error", backupId)))
		return
	}

	if apierrors.IsNotFound(err) {
		response.HandleError(resp, fmt.Errorf("backup %s not found", backupId))
		return
	}

	if err = NewBackupPlan(owner, h.factory, h.handler).Update(ctx, &b, backup); err != nil {
		response.HandleError(resp, errors.WithMessagef(err, format, backupId))
		return
	}

	r := &ResponseDescribeBackup{ // TODO
		Name:           backupId,
		BackupPolicies: b.BackupPolicies,
	}

	response.Success(resp, r)
}

func (h *Handler) deleteBackupPlan(req *restful.Request, resp *restful.Response) {
	ctx, backupId := req.Request.Context(), req.PathParameter("id")

	log.Debugf("delete backup %q", backupId)

	backup, err := h.handler.GetBackupHandler().GetById(ctx, backupId)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.WithMessagef(err, fmt.Sprintf("get backup %s error", backupId)))
		return
	}

	if apierrors.IsNotFound(err) {
		response.HandleError(resp, fmt.Errorf("backup %s not found", backupId))
		return
	}

	if err := h.handler.GetBackupHandler().Delete(ctx, backup); err != nil {
		response.HandleError(resp, errors.WithMessagef(err, "delete backup %s error", backupId))
		return
	}

	response.SuccessNoData(resp)
}

func (h *Handler) enabledBackupPlan(req *restful.Request, resp *restful.Response) {
	var (
		err error
		b   BackupEnabled
	)

	if err = req.ReadEntity(&b); err != nil {
		response.HandleError(resp, errors.WithStack(err))
		return
	}

	if !util.ListContains([]string{constant.BackupPause, constant.BackupResume}, strings.ToLower(b.Event)) {
		response.HandleError(resp, errors.WithMessagef(err, "backup event invalid, event: %s", b.Event))
		return
	}

	ctx, backupId := req.Request.Context(), req.PathParameter("id")

	log.Debugf("backup: %s, event: %s", backupId, b.Event)

	backup, err := h.handler.GetBackupHandler().GetById(ctx, backupId)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.WithMessagef(err, "get backup %s error", backupId))
		return
	}

	if apierrors.IsNotFound(err) {
		response.HandleError(resp, fmt.Errorf("backup %s not found", backupId))
		return
	}

	if err := h.handler.GetBackupHandler().Enabled(ctx, backup, strings.ToLower(b.Event)); err != nil {
		response.HandleError(resp, errors.WithMessagef(err, "trigger backup %s error", backupId))
		return
	}

	response.SuccessNoData(resp)
}

func (h *Handler) addSnapshot(req *restful.Request, resp *restful.Response) {
	var (
		err error
		b   CreateSnapshot
	)

	if err = req.ReadEntity(&b); err != nil {
		response.HandleError(resp, errors.WithStack(err))
		return
	}

	if b.Event != "create" {
		response.HandleError(resp, errors.Errorf("snapshot event invalid, event: %s", b.Event))
		return
	}

	ctx, backupId := req.Request.Context(), req.PathParameter("id")

	if backupId == "" {
		response.HandleError(resp, errors.New("backupId is empty"))
		return
	}

	backup, err := h.handler.GetBackupHandler().GetById(ctx, backupId)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.WithMessagef(err, "get backup %s error", backupId))
		return
	}

	if apierrors.IsNotFound(err) {
		response.HandleError(resp, fmt.Errorf("backup %s not found", backupId))
		return
	}

	if backup.Spec.Deleted {
		response.HandleError(resp, errors.WithMessagef(err, "backup %s is deleted", backupId))
		return
	}

	var location string
	for k := range backup.Spec.Location {
		location = k
		break
	}

	_, err = h.handler.GetSnapshotHandler().Create(ctx, backup, location)
	if err != nil {
		response.HandleError(resp, errors.WithMessagef(err, "create snapshot NOW error: %v, backupId: %s", err, backupId))
		return
	}

	response.SuccessNoData(resp)
}

func (h *Handler) listSnapshots(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()
	backupId := req.PathParameter("id")
	limit := req.QueryParameter("limit")
	offset := req.QueryParameter("offset")

	var labelSelector = "backup-id=" + backupId
	var snapshots, err = h.handler.GetSnapshotHandler().ListSnapshots(ctx, offset, util.ParseToInt64(limit), labelSelector, "")
	if err != nil {
		response.HandleError(resp, errors.WithMessage(err, fmt.Sprintf("get backup %s snapshots error", backupId)))
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
	snapshotId := req.PathParameter("snapshotId")

	snapshot, err := h.handler.GetSnapshotHandler().GetById(ctx, snapshotId)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.WithMessage(err, fmt.Sprintf("snapshot %s get error", snapshotId)))
		return
	}

	if apierrors.IsNotFound(err) {
		response.HandleError(resp, fmt.Errorf("snapshot %s not found", snapshotId))
		return
	}

	response.Success(resp, parseResponseSnapshotDetail(snapshot))
}

func (h *Handler) cancelSnapshot(req *restful.Request, resp *restful.Response) {
	var (
		err error
		b   SnapshotCancel
	)

	if err = req.ReadEntity(&b); err != nil {
		response.HandleError(resp, errors.WithStack(err))
		return
	}

	if b.Event != constant.BackupCancel {
		response.HandleError(resp, errors.WithMessagef(err, "snapshot event invalid, event: %s", b.Event))
		return
	}

	ctx := req.Request.Context()
	backupId := req.PathParameter("id")
	snapshotId := req.PathParameter("snapshotId")
	_ = ctx

	log.Debugf("backup: %s, snapshot: %s, event: %s", backupId, snapshotId, b.Event)

	_, err = h.handler.GetBackupHandler().GetById(ctx, backupId)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.WithMessage(err, fmt.Sprintf("get backup %s error", backupId)))
		return
	}

	if apierrors.IsNotFound(err) {
		response.HandleError(resp, fmt.Errorf("backup %s not found", backupId))
		return
	}

	snapshot, err := h.handler.GetSnapshotHandler().GetById(ctx, snapshotId)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.WithMessagef(err, "get snapshot %s error", snapshotId))
		return
	}

	if snapshot == nil {
		response.HandleError(resp, fmt.Errorf("snapshot %s not exists", backupId))
		return
	}

	// Failed
	var phase = *snapshot.Spec.Phase
	if util.ListContains([]string{
		constant.Failed.String(), constant.Completed.String()}, phase) {
		log.Infof("snapshot %s phase %s no need to Cancel", snapshotId, phase)
		response.SuccessNoData(resp)
		return
	}

	if err := h.handler.GetSnapshotHandler().UpdatePhase(ctx, snapshotId, constant.Canceled.String()); err != nil {
		response.HandleError(resp, errors.WithMessagef(err, "update snapshot %s Canceled error", snapshotId))
		return
	}

	response.SuccessNoData(resp)
}

func (h *Handler) getSpaceRegions(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()
	owner := req.HeaderParameter(constant.BflUserKey)

	olaresId, err := h.handler.GetSnapshotHandler().GetOlaresId(owner)
	if err != nil {
		response.HandleError(resp, errors.WithMessagef(err, "get olares id error"))
		return
	}

	var storageRegion = &storage.StorageRegion{
		Handlers: h.handler,
	}
	regions, err := storageRegion.GetRegions(ctx, owner, olaresId)
	if err != nil {
		response.HandleError(resp, err)
		return
	}

	response.Success(resp, regions)
}

func (h *Handler) listRestore(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()
	owner := req.HeaderParameter(constant.BflUserKey)
	limit := req.QueryParameter("limit")

	restores, err := h.handler.GetRestoreHandler().ListRestores(ctx, owner, 0, util.ParseToInt64(limit))
	if err != nil {
		log.Errorf("get restores error: %v", err)
		response.HandleError(resp, err)
		return
	}

	result := parseResponseRestoreList(restores)
	if result == nil || len(result) == 0 {
		response.SuccessNoData(resp)
		return
	}

	response.Success(resp, result)
}

func (h *Handler) addRestore(req *restful.Request, resp *restful.Response) {
	var (
		err error
		b   RestoreCreate
	)

	if err = req.ReadEntity(&b); err != nil {
		log.Errorf("add restore read entity error: %v", err)
		response.HandleError(resp, errors.WithStack(err))
		return
	}

	ctx := req.Request.Context()
	owner := req.HeaderParameter(constant.BflUserKey)

	if !b.verify() {
		log.Errorf("add restore params invalid, params: %s", util.ToJSON(b))
		response.HandleError(resp, errors.Errorf("restore params invalid"))
		return
	}

	var restoreTypeName = constant.RestoreTypeUrl
	var backupName, snapshotId, resticSnapshotId, location string
	var backupUrlObj *handlers.RestoreBackupUrlDetail

	if b.SnapshotId != "" {
		snapshotId = b.SnapshotId
		restoreTypeName = constant.RestoreTypeSnapshot
		snapshot, err := h.handler.GetSnapshotHandler().GetById(ctx, b.SnapshotId)
		if err != nil && !apierrors.IsNotFound(err) {
			response.HandleError(resp, errors.WithMessage(err, fmt.Sprintf("get snapshot %s error", snapshotId)))
			return
		}

		if apierrors.IsNotFound(err) {
			response.HandleError(resp, fmt.Errorf("snapshot %s not found", snapshotId))
			return
		}

		backup, err := h.handler.GetBackupHandler().GetById(ctx, snapshot.Spec.BackupId)
		if err != nil && !apierrors.IsNotFound(err) {
			response.HandleError(resp, errors.WithMessage(err, fmt.Sprintf("get backup %s error", snapshot.Spec.BackupId)))
			return
		}

		if apierrors.IsNotFound(err) {
			response.HandleError(resp, fmt.Errorf("backup %s not found", snapshot.Spec.BackupId))
			return

		}

		backupName = backup.Spec.Name
		resticSnapshotId = *snapshot.Spec.SnapshotId
	} else {
		// parse and split BackupURL
		backupUrlObj, backupName, resticSnapshotId, location, err = handlers.ParseRestoreBackupUrlDetail(b.BackupUrl)
		if err != nil {
			log.Errorf("parse BackupURL error %v, url: %s", b.BackupUrl)
			response.HandleError(resp, errors.Errorf("parse backupURL error: %v", err))
			return
		}
	}

	clusterId, err := handlers.GetClusterId()
	if err != nil {
		response.HandleError(resp, errors.Errorf("get cluster id error: %v", err))
		return
	}

	var restoreType = &handlers.RestoreType{
		Owner:            owner,
		Type:             restoreTypeName,
		Path:             strings.TrimSpace(b.Path),
		BackupName:       backupName,
		BackupUrl:        backupUrlObj, // if snapshot,it will be nil
		Password:         util.Base64encode([]byte(strings.TrimSpace(b.Password))),
		SnapshotId:       snapshotId,
		ResticSnapshotId: resticSnapshotId,
		ClusterId:        clusterId,
		Location:         location, // TODO filesystem
	}

	_, err = h.handler.GetRestoreHandler().CreateRestore(ctx, constant.BackupTypeFile, restoreType)
	if err != nil {
		response.HandleError(resp, errors.Errorf("create restore task failed: %v", err))
		return
	}

	response.SuccessNoData(resp)
}

func (h *Handler) getRestore(req *restful.Request, resp *restful.Response) {
	ctx, restoreId := req.Request.Context(), req.PathParameter("id")
	owner := req.HeaderParameter(constant.BflUserKey)
	_ = owner

	restore, err := h.handler.GetRestoreHandler().GetById(ctx, restoreId)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.WithMessagef(err, "get restore %s error", restoreId))
		return
	}

	if apierrors.IsNotFound(err) {
		response.HandleError(resp, fmt.Errorf("restore %s not found", restoreId))
		return
	}

	response.Success(resp, parseResponseRestoreDetail(nil, nil, restore))
}

func (h *Handler) cancelRestore(req *restful.Request, resp *restful.Response) {
	var (
		err error
		b   RestoreCancel
	)

	if err = req.ReadEntity(&b); err != nil {
		response.HandleError(resp, errors.WithStack(err))
		return
	}

	if b.Event != constant.BackupCancel {
		response.HandleError(resp, errors.WithMessagef(err, "restore event invalid, event: %s", b.Event))
		return
	}

	ctx := req.Request.Context()
	restoreId := req.PathParameter("id")
	_ = ctx

	log.Debugf("restore: %s, event: %s", restoreId, b.Event)

	restore, err := h.handler.GetRestoreHandler().GetById(ctx, restoreId)
	if err != nil && !apierrors.IsNotFound(err) {
		response.HandleError(resp, errors.WithMessagef(err, "get restore %s error", restoreId))
		return
	}

	if apierrors.IsNotFound(err) {
		response.HandleError(resp, fmt.Errorf("restore %s not found", restoreId))
		return
	}

	var phase = *restore.Spec.Phase
	if util.ListContains([]string{
		constant.Failed.String(), constant.Completed.String()}, phase) {
		log.Infof("restore %s phase %s no need to Cancel", restoreId, phase)
		response.SuccessNoData(resp)
		return
	}

	if err := h.handler.GetRestoreHandler().UpdatePhase(ctx, restoreId, constant.Canceled.String()); err != nil {
		response.HandleError(resp, errors.WithMessagef(err, "update restore %s Canceled error", restoreId))
		return
	}

	response.SuccessNoData(resp)
}
