package handlers

import (
	"context"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RestoreHandler struct {
	factory client.Factory
}

func NewRestoreHandler(f client.Factory) *RestoreHandler {
	return &RestoreHandler{
		factory: f,
	}
}

func (o *RestoreHandler) ListRestores() {

}

func (o *RestoreHandler) CreateRestore(ctx context.Context, restoreTypeName string, restoreType map[string]string) (*sysv1.Restore, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	var startAt = time.Now().UnixMilli()
	var phase = constant.Pending.String()

	var restore = &sysv1.Restore{
		TypeMeta: metav1.TypeMeta{
			Kind:       constant.KindRestore,
			APIVersion: sysv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      restoreType["snapshotId"],
			Namespace: constant.DefaultOsSystemNamespace,
			Labels:    map[string]string{},
		},
		Spec: sysv1.RestoreSpec{
			RestoreType: map[string]string{
				restoreTypeName: util.ToJSON(restoreType),
			},
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
