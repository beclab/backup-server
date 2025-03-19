package velero

import (
	"bytes"
	"context"
	"fmt"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Manager interface {
	IsReady(ctx context.Context) (bool, error)

	NewCredentials(c *sysv1.BackupConfigSpec) []byte

	SetBackupConfig(ctx context.Context, name string, bcSpec *sysv1.BackupConfigSpec) error

	GetBackupConfig(ctx context.Context, name string) (*sysv1.BackupConfig, error)

	ListBackupConfigs(ctx context.Context) (*sysv1.BackupConfigList, error)

	Namespace() string

	// BuildVeleroBackup(name, owner, backupType string, retainDays int64) *velerov1api.Backup

	FormatSysBackupTimeofDay(backupConfig *sysv1.BackupConfig) (err error)
}

type velero struct {
	namespace string
	factory   client.Factory
	cron      *cron.Cron
}

var _ Manager = &velero{}

func NewManager(factory client.Factory) Manager {
	c := cron.New()
	c.Start()

	return &velero{
		namespace: "",
		factory:   factory,
		cron:      c,
	}
}

func (v *velero) Namespace() string {
	return v.namespace
}

func (v *velero) applyBackupConfig(ctx context.Context, name string, bcSpec *sysv1.BackupConfigSpec) (*sysv1.BackupConfig, error) {
RETRY:
	bc := BuildBackupConfig(v.namespace, name, bcSpec)

	r, err := newUnstructuredResourceWithGVK(bc.GroupVersionKind(), bc)
	if err != nil {
		return nil, err
	}

	dc, err := v.factory.DynamicClient()
	if err != nil {
		return nil, err
	}

	_, err = dc.Resource(BackupConfigGVR).Namespace(v.namespace).
		Apply(ctx, r.GetName(), r, metav1.ApplyOptions{Force: true, FieldManager: ApplyPatchFieldManager})
	if err != nil && apierrors.IsConflict(err) {
		goto RETRY
	} else if err != nil {
		return nil, errors.WithStack(err)
	}
	return bc, nil
}

func (v *velero) SetBackupConfig(ctx context.Context, name string, bcSpec *sysv1.BackupConfigSpec) (err error) {
	if err = validateBackupConfig(bcSpec); err != nil {
		return errors.WithStack(err)
	}
	SetDefaultBackupConfigSpec(bcSpec)
	log.Infof("got backup config: %s", util.PrettyJSON(bcSpec))

	bc, err := v.applyBackupConfig(ctx, name, bcSpec)
	if err != nil {
		return err
	}
	log.Infof("applied backupconfig: %s", util.ToJSON(bc))

	return
}

func (v *velero) GetBackupConfig(ctx context.Context, name string) (*sysv1.BackupConfig, error) {
	c, err := v.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	bc, err := c.SysV1().BackupConfigs(v.namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return bc, nil
}

func (v *velero) ListBackupConfigs(ctx context.Context) (*sysv1.BackupConfigList, error) {
	c, err := v.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}
	bcs, err := c.SysV1().BackupConfigs(v.namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return bcs, nil
}

func (v *velero) NewCredentials(c *sysv1.BackupConfigSpec) []byte {
	var buf bytes.Buffer

	buf.WriteString("[default]\n")
	buf.WriteString(fmt.Sprintf("  aws_access_key_id = %s\n", c.AccessKey))
	buf.WriteString(fmt.Sprintf("  aws_secret_access_key = %s\n", c.SecretKey))

	return buf.Bytes()
}

func (v *velero) IsReady(ctx context.Context) (bool, error) {
	kc, err := v.factory.KubeClient()
	if err != nil {
		return false, err
	}

	// deployment
	done, err := deploymentReadyFunc(ctx, kc, v.namespace)()
	if !done && err != nil {
		return false, err
	}

	return true, nil
}

func (v *velero) FormatSysBackupTimeofDay(backupConfig *sysv1.BackupConfig) (err error) {
	if backupConfig == nil || backupConfig.Spec.BackupPolicy == nil {
		return nil
	}
	backupConfig.Spec.BackupPolicy.TimesOfDay, err = util.ParseLocalToTimestamp(backupConfig.Spec.BackupPolicy.TimesOfDay)
	if err != nil {
		return err
	}
	return nil
}
