package operator

import (
	"context"

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

func NewBackupOperator(f client.Factory) *BackupOperator {
	return &BackupOperator{
		factory: f,
	}
}

func (o *BackupOperator) GetBackup(ctx context.Context, name string) (*sysv1.Backup, error) {
	client, err := o.factory.DynamicClient()
	if err != nil {
		return nil, err
	}

	obj, err := client.Resource(constant.BackupGVR).Namespace(constant.DefaultOsSystemNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	var backup sysv1.Backup
	if err = converter.FromUnstructured(obj.Object, &backup); err != nil {
		return nil, err
	}
	return &backup, nil

}

func (o *BackupOperator) CreateBackup(ctx context.Context, backupSpec *sysv1.BackupSpec) (*sysv1.Backup, error) {
RETRY:
	var backup = &sysv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupSpec.Name,
			Namespace: constant.DefaultOsSystemNamespace,
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

	_, err = dynamicClient.Resource(constant.BackupGVR).Namespace(constant.DefaultOsSystemNamespace).Apply(ctx, res.GetName(), &res, metav1.ApplyOptions{Force: true})

	if err != nil && apierrors.IsConflict(err) {
		goto RETRY
	} else if err != nil {
		return nil, errors.WithStack(err)
	}

	return backup, nil
}
