package operator

import (
	"context"
	"fmt"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/converter"
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
		return nil, nil
	}

	return backups, nil
}

func (o *BackupOperator) GetBackup(ctx context.Context, name string, owner string) (*sysv1.Backup, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	backups, err := c.SysV1().Backups(constant.DefaultOsSystemNamespace).List(ctx, metav1.ListOptions{
		// FieldSelector: "metadata.name=" + name + ",spec.owner=" + owner,
	})

	if err != nil {
		return nil, err
	}

	if backups == nil || backups.Items == nil || len(backups.Items) == 0 {
		return nil, nil
	}

	return &backups.Items[0], nil
}

func (o *BackupOperator) GetBackupById(ctx context.Context, id string) (*sysv1.Backup, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	backups, err := c.SysV1().Backups(constant.DefaultOsSystemNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "id=" + id,
	})

	if err != nil {
		return nil, err
	}

	if backups == nil || backups.Items == nil || len(backups.Items) == 0 {
		return nil, errors.WithStack(fmt.Errorf("backup %s not found", id))
	}

	return &backups.Items[0], nil
}

func (o *BackupOperator) CreateBackup(ctx context.Context, backupSpec *sysv1.BackupSpec) (*sysv1.Backup, error) {
RETRY:
	var backup = &sysv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupSpec.Name,
			Namespace: constant.DefaultOsSystemNamespace,
			Labels: map[string]string{
				"id": backupSpec.Id,
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
		Apply(ctx, res.GetName(), &res, metav1.ApplyOptions{Force: true, FieldManager: "backup-controller"})

	if err != nil && apierrors.IsConflict(err) {
		goto RETRY
	} else if err != nil {
		return nil, errors.WithStack(err)
	}

	return backup, nil
}
