package operator

import (
	"context"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RestoreOperator struct {
	factory client.Factory
}

func NewRestoreOperator(f client.Factory) *RestoreOperator {
	return &RestoreOperator{
		factory: f,
	}
}

func (o *RestoreOperator) ListRestores() {

}

func (o *RestoreOperator) CreateRestore(ctx context.Context, snapshotId string) (*sysv1.Restore, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	var startAt = time.Now().UnixMilli()
	var phase = constant.SnapshotPhasePending.String()

	var restore = &sysv1.Restore{
		TypeMeta: metav1.TypeMeta{
			Kind:       constant.KindRestore,
			APIVersion: sysv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshotId,
			Namespace: constant.DefaultOsSystemNamespace,
			Labels:    map[string]string{},
		},
		Spec: sysv1.RestoreSpec{
			StartAt: startAt,
			Phase:   &phase,
		},
	}

	created, err := c.SysV1().Restores(constant.DefaultOsSystemNamespace).Create(ctx, restore, metav1.CreateOptions{FieldManager: constant.RestoreController})
	if err != nil {
		return nil, err
	}

	return created, nil
}
