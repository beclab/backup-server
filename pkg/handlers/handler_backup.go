package handlers

import (
	"context"
	"fmt"
	"sort"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/converter"
	"bytetrade.io/web3os/backup-server/pkg/integration"
	"bytetrade.io/web3os/backup-server/pkg/notify"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/util/uuid"
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

func (o *BackupHandler) NotifyToSpace(backup *sysv1.Backup) error {
	integrationName := GetBackupIntegrationName(constant.BackupLocationSpace.String(), backup.Spec.Location)
	if integrationName == "" {
		return fmt.Errorf("space integrationName not exists, config: %s", util.ToJSON(backup.Spec.Location))
	}
	olaresSpaceToken, err := integration.IntegrationManager().GetIntegrationSpaceToken(integrationName)
	if err != nil {
		return err
	}

	location, _, _ := GetBackupLocationConfig(backup)
	var notifyBackupObj = &notify.Backup{
		UserId:         olaresSpaceToken.OlaresDid,
		Token:          olaresSpaceToken.AccessToken,
		BackupId:       backup.Name,
		Name:           backup.Spec.Name,
		BackupPath:     GetBackupPath(backup),
		BackupLocation: location,
	}

	if err := notify.SendNewBackup(constant.DefaultSyncServerURL, notifyBackupObj); err != nil {
		return fmt.Errorf("notify backup obj error %v", err)
	}

	return o.UpdatePushState(context.Background(), backup.Name, true)
}

func (o *BackupHandler) UpdatePushState(ctx context.Context, backupId string, pushState bool) error {
	var ctx1, cancel = context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	sc, err := o.factory.Sysv1Client()
	if err != nil {
		return err
	}

	b, err := o.GetBackupById(ctx1, backupId)
	if err != nil {
		return err
	}
	b.Spec.Push = pushState

RETRY:
	_, err = sc.SysV1().Backups(constant.DefaultOsSystemNamespace).Update(ctx, b, metav1.UpdateOptions{
		FieldManager: constant.BackupController,
	})

	if err != nil && apierrors.IsConflict(err) {
		log.Warnf("update backup %s spec retry", b.Spec.Name)
		goto RETRY
	} else if err != nil {
		return errors.WithStack(fmt.Errorf("update backup error: %v", err))
	}

	return nil
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

func (o *BackupHandler) GetBackupById(ctx context.Context, backupId string) (*sysv1.Backup, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	backups, err := c.SysV1().Backups(constant.DefaultOsSystemNamespace).Get(ctx, backupId, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	if backups == nil {
		return nil, nil
	}

	return backups, nil
}

func (o *BackupHandler) GetBackup(ctx context.Context, owner string, backupName string) (*sysv1.Backup, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	backups, err := c.SysV1().Backups(constant.DefaultOsSystemNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "name=" + util.MD5(backupName) + ",owner=" + owner,
	})

	if err != nil {
		return nil, err
	}

	if backups == nil || backups.Items == nil || len(backups.Items) == 0 {
		return nil, nil
	}

	return &backups.Items[0], nil
}

func (o *BackupHandler) CreateBackup(ctx context.Context, owner string, backupName string, backupSpec *sysv1.BackupSpec) (*sysv1.Backup, error) {
	var backupId = uuid.NewUUID()
RETRY:
	var backup = &sysv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupId,
			Namespace: constant.DefaultOsSystemNamespace,
			Labels: map[string]string{
				"owner": owner,
				"name":  util.MD5(backupName),
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
