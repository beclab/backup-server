package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/converter"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/util/uuid"
	"github.com/emicklei/go-restful/v3"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type BackupHandler struct {
	factory  client.Factory
	handlers Interface
}

func NewBackupHandler(f client.Factory, handlers Interface) *BackupHandler {
	return &BackupHandler{
		factory:  f,
		handlers: handlers,
	}
}

func (o *BackupHandler) GetBackupPassword(ctx context.Context, backup *sysv1.Backup) (password string, err error) {
	password = "123"
	return

	var owner = backup.Spec.Owner
	var backupName = backup.Spec.Name

	settingsUrl := fmt.Sprintf("http://settings-service.user-space-%s/api/backup/password", owner)
	client := resty.New().SetTimeout(5 * time.Second).SetDebug(true)

	req := &proxyRequest{
		Op:       "getAccount",
		DataType: "backupPassword",
		Version:  "v1",
		Group:    "service.settings",
		Data:     backupName,
	}

	terminusNonce, err := util.GenTerminusNonce("")
	if err != nil {
		err = fmt.Errorf("generate nonce error: %v", err)
		return
	}

	log.Info("fetch password from settings, ", settingsUrl)
	resp, err := client.R().SetContext(ctx).
		SetHeader(restful.HEADER_ContentType, restful.MIME_JSON).
		SetHeader("Terminus-Nonce", terminusNonce).
		SetBody(req).
		SetResult(&passwordResponse{}).
		Post(settingsUrl)

	if err != nil {
		err = fmt.Errorf("request settings password api error: %v", err)
		return
	}

	if resp.StatusCode() != http.StatusOK {
		err = fmt.Errorf("request settings password api response not ok, status: %d, msg: %s", resp.StatusCode(), string(resp.Body()))
		return
	}

	pwdResp := resp.Result().(*passwordResponse)
	if pwdResp.Code != 0 {
		err = fmt.Errorf("request settings password api response error, code: %d, msg: %s", pwdResp.Code, pwdResp.Message)
		return
	}

	if pwdResp.Data == nil {
		err = fmt.Errorf("request settings password api response error, code: %d, msg: %s", pwdResp.Code, pwdResp.Message)
		return
	}

	password = pwdResp.Data.Value
	return
}

func (o *BackupHandler) Delete(ctx context.Context, backup *sysv1.Backup) error {
	backup.Spec.Deleted = true
	return o.update(ctx, backup)
}

func (o *BackupHandler) Enabled(ctx context.Context, backup *sysv1.Backup, data string) error {
	var enabled bool
	if data == constant.BackupResume {
		enabled = true
	} else {
		enabled = false
	}
	backup.Spec.BackupPolicy.Enabled = enabled
	return o.update(ctx, backup)
}

func (o *BackupHandler) UpdateNotifyState(ctx context.Context, backupId string, notified bool) error {
	backup, err := o.GetById(ctx, backupId)
	if err != nil {
		return err
	}
	backup.Spec.Notified = notified

	return o.update(ctx, backup)

}

func (o *BackupHandler) ListBackups(ctx context.Context, owner string, page int64, limit int64) (*sysv1.BackupList, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	backups, err := c.SysV1().Backups(constant.DefaultOsSystemNamespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		return nil, err
	}

	if backups == nil || backups.Items == nil || len(backups.Items) == 0 {
		return nil, fmt.Errorf("backups not exists")
	}

	sort.Slice(backups.Items, func(i, j int) bool {
		return !backups.Items[i].ObjectMeta.CreationTimestamp.Before(&backups.Items[j].ObjectMeta.CreationTimestamp)
	})

	return backups, nil
}

func (o *BackupHandler) GetById(ctx context.Context, id string) (*sysv1.Backup, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	backup, err := c.SysV1().Backups(constant.DefaultOsSystemNamespace).Get(ctx, id, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	if backup == nil {
		return nil, apierrors.NewNotFound(sysv1.Resource("Backup"), id)
	}

	return backup, nil
}

func (o *BackupHandler) GetByLabel(ctx context.Context, label string) (*sysv1.Backup, error) {
	var getCtx, cancel = context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}
	backups, err := c.SysV1().Backups(constant.DefaultOsSystemNamespace).List(getCtx, metav1.ListOptions{
		LabelSelector: label,
	})

	if err != nil {
		return nil, err
	}

	if backups == nil || backups.Items == nil || len(backups.Items) == 0 {
		return nil, apierrors.NewNotFound(sysv1.Resource("Backup"), label)
	}

	return &backups.Items[0], nil
}

func (o *BackupHandler) Create(ctx context.Context, owner string, backupName string, backupSpec *sysv1.BackupSpec) (*sysv1.Backup, error) {
	var policy = fmt.Sprintf("%s_%s_%d_%d", backupSpec.BackupPolicy.SnapshotFrequency, backupSpec.BackupPolicy.TimesOfDay, backupSpec.BackupPolicy.DayOfWeek, backupSpec.BackupPolicy.DateOfMonth)
	var backupId = uuid.NewUUID()
RETRY:
	var backup = &sysv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupId,
			Namespace: constant.DefaultOsSystemNamespace,
			Labels: map[string]string{
				"owner":  owner,
				"name":   util.MD5(backupName),
				"policy": util.MD5(policy),
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Backup",
			APIVersion: sysv1.SchemeGroupVersion.String(),
		},
		Spec: *backupSpec,
	}

	obj, err := converter.ToUnstructured(backup)
	if err != nil {
		return nil, err
	}

	res := unstructured.Unstructured{Object: obj}
	res.SetGroupVersionKind(backup.GroupVersionKind())

	dynamicClient, err := o.factory.DynamicClient()
	if err != nil {
		return nil, err
	}

	_, err = dynamicClient.Resource(constant.BackupGVR).Namespace(constant.DefaultOsSystemNamespace).
		Apply(ctx, res.GetName(), &res, metav1.ApplyOptions{Force: true, FieldManager: constant.BackupController})

	if err != nil && apierrors.IsConflict(err) {
		goto RETRY
	} else if err != nil {
		return nil, errors.WithStack(err)
	}

	return backup, nil
}

func (o *BackupHandler) GetBackupIdForLabels(backups *sysv1.BackupList) []string {
	var labels []string

	for _, backup := range backups.Items {
		labels = append(labels, fmt.Sprintf("backup-id=%s", backup.Name))
	}
	return labels
}

func (o *BackupHandler) update(ctx context.Context, backup *sysv1.Backup) error {
	sc, err := o.factory.Sysv1Client()
	if err != nil {
		return err
	}

	var getCtx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

RETRY:
	_, err = sc.SysV1().Backups(constant.DefaultOsSystemNamespace).Update(getCtx, backup, metav1.UpdateOptions{
		FieldManager: constant.BackupController,
	})

	if err != nil && apierrors.IsConflict(err) {
		log.Warnf("update backup %s spec retry", backup.Spec.Name)
		goto RETRY
	} else if err != nil {
		return errors.WithStack(fmt.Errorf("update backup error: %v", err))
	}

	return nil
}

func (o *BackupHandler) delete(ctx context.Context, backup *sysv1.Backup) error {
	sc, err := o.factory.Sysv1Client()
	if err != nil {
		return err
	}

	var getCtx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

RETRY:
	err = sc.SysV1().Backups(constant.DefaultOsSystemNamespace).Delete(getCtx, backup.Name, metav1.DeleteOptions{})

	if err != nil && apierrors.IsConflict(err) {
		log.Warnf("delete backup %s spec retry", backup.Spec.Name)
		goto RETRY
	} else if err != nil {
		return errors.WithStack(fmt.Errorf("delete backup error: %v", err))
	}

	return nil
}
