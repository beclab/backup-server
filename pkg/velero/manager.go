package velero

import (
	"bytes"
	"context"
	"fmt"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/converter"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	informers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions"
	veleroinstall "github.com/vmware-tanzu/velero/pkg/install"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

type Manager interface {
	CRDsAreReady() (bool, error)

	InstallCRDs() error

	InstallCoreResources(ctx context.Context) error

	IsReady(ctx context.Context) (bool, error)

	Available(ctx context.Context) (bool, error)

	NewCredentials(c *sysv1.BackupConfigSpec) []byte

	ApplyBackupStorageLocation(ctx context.Context, bcSpec *sysv1.BackupConfigSpec) (*velerov1api.BackupStorageLocation, error)

	GetBackupStorageLocation(ctx context.Context, name string) (*velerov1api.BackupStorageLocation, error)

	SetBackupConfig(ctx context.Context, name string, bcSpec *sysv1.BackupConfigSpec) error

	GetBackupConfig(ctx context.Context, name string) (*sysv1.BackupConfig, error)

	ListBackupConfigs(ctx context.Context) (*sysv1.BackupConfigList, error)

	ListBackups(ctx context.Context) (*velerov1api.BackupList, error)

	GetVeleroBackup(ctx context.Context, name string) (*velerov1api.Backup, error)

	GetSysBackup(ctx context.Context, name string) (*sysv1.Backup, error)

	ListSysBackups(ctx context.Context, config string) (*sysv1.BackupList, error)

	ExistRunningBackup(ctx context.Context, config string) (bool, error)

	CreateBackup(ctx context.Context, config, name, owner string, retainDays int64) (*sysv1.Backup, error)

	DownloadBackup(ctx context.Context, name, downloadToFilePath string) (int64, error)

	DeleteBackup(ctx context.Context, name string, sb *sysv1.Backup) error

	AsyncOsDataBackup(name string)

	BackupStatus(ctx context.Context, name string) (bool, string, error)

	Namespace() string

	GetBackupSchedule(ctx context.Context, name string) (*velerov1api.Schedule, error)

	ApplyBackupSchedule(ctx context.Context, name, owner, schedule string, paused bool) (*velerov1api.Schedule, error)

	GetTerminusVersion(ctx context.Context, dc dynamic.Interface) (*string, error)

	BuildSysBackup(ctx context.Context, config, name, owner, bsl, backupType string, retainDays int64) (*sysv1.Backup, error)

	// BuildVeleroBackup(name, owner, backupType string, retainDays int64) *velerov1api.Backup

	DeleteVeleroBackup(ctx context.Context, namespace, name string) error

	BuildVeleroBackupX(ctx context.Context, name, jobTime, owner, backupType string, retainDays int64) *velerov1api.Backup
	CreateBackupX(ctx context.Context, namespace string, backup *velerov1api.Backup) error
	SetBackupConfigPBMName(ctx context.Context, backupConfig *sysv1.BackupConfig, pbmName string) error
	DeleteBackupX(ctx context.Context, namespace string, name string) error

	NewInformerFactory() (informers.SharedInformerFactory, error)
	UpdateSysBackupPhase(ctx context.Context, backup *velerov1api.Backup) error
	UpdateSysBackup(ctx context.Context, sysBackup *sysv1.Backup) error

	CanDeleteBackup(phase velerov1api.BackupPhase) bool
	CanCreateBackup(phase velerov1api.BackupPhase) bool
	GetBackupName(name string) (string, error)
	GetBackupJobLastTime(name string) uint64

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
		namespace: DefaultVeleroNamespace,
		factory:   factory,
		cron:      c,
	}
}

func (v *velero) Namespace() string {
	return v.namespace
}

func (v *velero) CRDsAreReady() (bool, error) {
	crds, err := allCRDs()
	if err != nil {
		return false, errors.WithStack(err)
	}
	fn, err := crdsAreReadyFunc(v.factory, crds)
	if err != nil {
		return false, errors.WithStack(err)
	}
	return fn()
}

func (v *velero) createResource(r *unstructured.Unstructured) error {
	c, err := v.factory.ClientForUnstructured(r)
	if err != nil {
		return errors.Errorf("client for unstructured, %v", err)
	}

	log.Debugf("create resource %q\n%s", r.GetKind(), util.PrettyJSON(r))

	if _, err = c.Create(r); apierrors.IsAlreadyExists(err) {
		return errors.Errorf("resource %s/%s already exists", r.GetKind(), r.GetName())
	} else if err != nil {
		log.Errorf("unable to create resource %q, %v\nresources:\n%s", r.GetKind(), err, util.PrettyJSON(r))
		return errors.Errorf("unable to create resource %q: %v", r.GetKind(), err)
	}
	return nil
}

func (v *velero) InstallCRDs() error {
	crds, err := allCRDs()
	if err != nil {
		return errors.WithMessage(err, "install allCRDs")
	}

	rg := veleroinstall.GroupResources(crds)

	for _, r := range rg.CRDResources {
		if err = v.createResource(r); err != nil {
			return err
		}
	}

	return nil
}

func (v *velero) InstallCoreResources(ctx context.Context) (err error) {
	namespace := v.namespace
	resources := newUnstructuredList()

	// namespace
	ns := BuildNamespace(namespace)
	if err = appendUnstructured(resources, ns); err != nil {
		return err
	}

	// clusterrolebinding, serviceaccount
	crb := BuildClusterRoleBinding(namespace)
	if err = appendUnstructured(resources, crb); err != nil {
		return err
	}
	sa := BuildServiceAccount(namespace, nil)
	if err = appendUnstructured(resources, sa); err != nil {
		return err
	}

	for _, r := range resources.Items {
		if err = v.createResource(&r); err != nil {
			return err
		}
	}
	return nil
}

func (v *velero) GetBackupStorageLocation(ctx context.Context, name string) (*velerov1api.BackupStorageLocation, error) {
	vc, err := v.factory.Client()
	if err != nil {
		return nil, err
	}

	bsl, err := vc.VeleroV1().BackupStorageLocations(v.namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Errorf("get backupStorageLocation: %v", err)
	}
	return bsl, nil
}

func (v *velero) ApplyBackupStorageLocation(ctx context.Context, bcSpec *sysv1.BackupConfigSpec) (*velerov1api.BackupStorageLocation, error) {
	bsl := BuildBackupStorageLocation(v.namespace, bcSpec)

	r, err := newUnstructuredResourceWithGVK(bsl.GroupVersionKind(), bsl)
	if err != nil {
		return nil, err
	}

	dc, err := v.factory.DynamicClient()
	if err != nil {
		return nil, err
	}

	obj, err := dc.Resource(BackupStorageLocationGVR).Namespace(v.namespace).
		Apply(ctx, r.GetName(), r, metav1.ApplyOptions{Force: true, FieldManager: ApplyPatchFieldManager})
	if err != nil {
		return nil, errors.Errorf("apply backupStorageLocation: %v", err)
	}

	var res velerov1api.BackupStorageLocation
	if err = converter.FromUnstructured(obj.Object, &res); err != nil {
		return nil, err
	}
	return &res, nil
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

// ???
// should always return true
func (v *velero) Available(ctx context.Context) (bool, error) {
	// vc, err := v.factory.Client()
	// if err != nil {
	// 	return false, err
	// }

	// bc, err := v.DefaultBackupConfigSpec()
	// if err != nil {
	// 	return false, err
	// }

	// bsl, err := vc.VeleroV1().BackupStorageLocations(v.namespace).
	// 	Get(ctx, bc.StorageLocation, metav1.GetOptions{})
	// if err != nil {
	// 	return false, err
	// }
	// return bsl.Status.Phase == velerov1api.BackupStorageLocationPhaseAvailable, nil

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
