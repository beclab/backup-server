package operator

import (
	"context"
	"fmt"
	"sort"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/converter"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backups-sdk/pkg/utils"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type BackupOperator struct {
	factory client.Factory
}

type OwnerBackupListCache struct {
	Pages map[int]*Backuplist
}

type Backuplist struct {
	token string
	List  *sysv1.BackupList
}

var owenerBackupListCache map[string]*OwnerBackupListCache

func init() {
	owenerBackupListCache = make(map[string]*OwnerBackupListCache)
}

func NewBackupOperator(f client.Factory) *BackupOperator {
	return &BackupOperator{
		factory: f,
	}
}

func (o *BackupOperator) ListBackups(ctx context.Context, owner string, page int64, limit int64) (*sysv1.BackupList, error) {
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

func (o *BackupOperator) GetBackupById(ctx context.Context, backupId string) (*sysv1.Backup, error) {
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

func (o *BackupOperator) GetBackup(ctx context.Context, owner string, backupName string) (*sysv1.Backup, error) {
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

func (o *BackupOperator) CreateBackup(ctx context.Context, owner string, backupName string, backupSpec *sysv1.BackupSpec) (*sysv1.Backup, error) {
	var backupId = utils.NewUUID()
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
