package v1

import (
	"context"
	"strings"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TerminusCloud = "terminus-cloud"

	S3 = "s3"
)

type BackupPlan struct {
	owner   string
	c       *BackupCreate
	factory client.Factory
	manager velero.Manager
}

func NewBackupPlan(owner string, factory client.Factory, manager velero.Manager) *BackupPlan {
	return &BackupPlan{
		owner:   owner,
		factory: factory,
		manager: manager,
	}
}

func (o *BackupPlan) Apply(ctx context.Context, c *BackupCreate) error {
	var err error
	o.c = c

	if err = o.validate(ctx); err != nil {
		return errors.WithStack(err)
	}
	if err = o.apply(ctx, c.Name); err != nil {
		return err
	}
	return nil
}

func (o *BackupPlan) validate(ctx context.Context) error {
	var (
		ok  bool
		err error
	)

	if ok, err = o.manager.CRDsAreReady(); err != nil {
		return err
	} else if !ok {
		return errors.New("backup crd not ready")
	}
	if o.c.Name == "" {
		return errors.New("name is required")
	}
	if o.owner == "" {
		return errors.New("owner is required")
	}
	if o.c.Location != "" {
		if !util.ListContains([]string{TerminusCloud, S3}, o.c.Location) {
			return errors.New("invalid backup location, must be 'terminus-cloud' or 's3'")
		}

		if o.c.Location == TerminusCloud {
			// cloud version must be set the default backup bucket option
			isCloud := util.EnvOrDefault("TERMINUS_IS_CLOUD_VERSION", "false")
			if isCloud == "true" {
				if velero.DefaultBackupBucket == "" {
					return errors.New("backup bucket is required for cloud version")
				}
				if velero.DefaultBackupKeyPrefix == "" {
					return errors.New("backup key prefix is required for cloud version")
				}
			}
		} else if o.c.Location == S3 {
			if o.c.Config == nil {
				return errors.New("no s3 config")
			}
			if o.c.Config.Region == "" {
				return errors.New("s3 config 'region' is required")
			}
			if o.c.Config.Bucket == "" {
				return errors.New("s3 config 'bucket' is required")
			}
			if o.c.Config.AccessKey == "" {
				return errors.New("s3 config 'accessKey' is required")
			}
			if o.c.Config.SecretKey == "" {
				return errors.New("s3 config 'secretKey' is required")
			}
		}
	}
	if o.c.BackupPolicies != nil {
		if o.c.BackupPolicies.SnapshotFrequency == "" || o.c.BackupPolicies.TimesOfDay == "" {
			return errors.Errorf("backup policy %q, snapshot frequency or times of day is empty", o.c.Name)
		}

		// support hour-minute and timestamp
		if !strings.Contains(o.c.BackupPolicies.TimesOfDay, ":") {
			timeInUTC, err := util.ParseTimestampToLocal(o.c.BackupPolicies.TimesOfDay)
			if err != nil {
				return errors.New("invalid times of day format, eg: '48600000'")
			}
			o.c.BackupPolicies.TimesOfDay = timeInUTC
		} else {
			timeSplit := strings.Split(o.c.BackupPolicies.TimesOfDay, ":")
			if !strings.Contains(o.c.BackupPolicies.TimesOfDay, ":") || len(timeSplit) != 2 {
				return errors.New("invalid times of day format, eg: '07:30'")
			}
		}

	}
	if o.c.Password != "" && o.c.Password != o.c.ConfirmPassword {
		return errors.New("password and confirm password are different")
	}

	return nil
}

func (o *BackupPlan) mergeConfig(name string) *sysv1.BackupConfigSpec {
	bc := &sysv1.BackupConfigSpec{
		Provider:        velero.TerminusCloud,
		Owner:           o.owner,
		Bucket:          TerminusCloud,
		Location:        TerminusCloud,
		Region:          TerminusCloud,
		StorageLocation: TerminusCloud,
	}
	if o.c.BackupPolicies != nil {
		bc.BackupPolicy = o.c.BackupPolicies
	}
	return bc

	// location := o.c.Location
	// if strings.Contains(location, velero.TerminusCloud) {

	// }

	// c := o.c.Config
	// bc := &sysv1.BackupConfigSpec{
	// 	Region:          c.Region,
	// 	Bucket:          c.Bucket,
	// 	Prefix:          c.Prefix,
	// 	S3Url:           c.S3Url,
	// 	AccessKey:       c.AccessKey,
	// 	SecretKey:       c.SecretKey,
	// 	Owner:           o.owner,
	// 	Location:        o.c.Location,
	// 	StorageLocation: name,
	// }
	// if o.c.BackupPolicies != nil {
	// 	bc.BackupPolicy = o.c.BackupPolicies
	// }

	// if c.Provider == "" {
	// 	bc.Provider = velero.AWS
	// } else if c.Provider != "" {
	// 	bc.Provider = c.Provider
	// }
	// return bc
}

func (o *BackupPlan) apply(ctx context.Context, name string) error {
	var (
		err        error
		configSpec *sysv1.BackupConfigSpec
	)

	configSpec = o.mergeConfig(name)
	if o.c != nil {
		// backup password setting has moved into 'settings'
		// if o.c.Password != "" {
		// 	repositoryPassword := util.EncodeStringToBase64(o.c.Password)
		// 	if configSpec != nil && configSpec.RepositoryPassword != repositoryPassword {
		// 		log.Debugf("now password is changed to %q", o.c.Password)
		// 		configSpec.RepositoryPassword = repositoryPassword
		// 		passwordChanged = true
		// 	}
		// }

		if o.c.Location != "" && configSpec.Location != o.c.Location {
			return errors.New("change location is not allowed")
		}
	}

	log.Debugf("merged bc spec: %s", util.PrettyJSON(configSpec))
	if err = o.manager.SetBackupConfig(ctx, name, configSpec); err != nil {
		return err
	}

	return nil
}

func (o *BackupPlan) hasAvailableFullyBackup(ctx context.Context) bool {
	sc, err := o.factory.Sysv1Client()
	if err != nil {
		log.Warnf("new sc client: %v", err)
		return false
	}

	l, err := sc.SysV1().Backups(o.manager.Namespace()).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Errorf("list sys backups: %v", err)
		return false
	}

	for _, b := range l.Items {
		if b.Spec.Extra != nil {
			if v, ok := b.Spec.Extra[velero.ExtraBackupType]; !(ok && v == velero.FullyBackup) {
				continue
			}
			phase, middlewarePhase := b.Spec.Phase, b.Spec.MiddleWarePhase
			if b.Spec.Size != nil && (phase != nil && *phase == velero.Succeed) &&
				(middlewarePhase != nil && util.ListContains([]string{velero.Succeed, velero.Success}, *middlewarePhase)) {
				return true
			}
		}
	}
	return false
}

func (o *BackupPlan) hasInProgressBackup(ctx context.Context) (bool, error) {
	sc, err := o.factory.Sysv1Client()
	if err != nil {
		return false, err
	}

	ns := o.manager.Namespace()
	l, err := sc.SysV1().Backups(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	for _, item := range l.Items {
		if util.ListContains([]string{velero.Pending, velero.Started, velero.Running}, *item.Spec.Phase) {
			return true, errors.Errorf("backup %q in progress", item.Name)
		}
	}
	return false, nil
}

// func (o *BackupPlan) createFullySysBackup(ctx context.Context, config, name, owner string) error {
// 	if _, err := o.hasInProgressBackup(ctx); err != nil {
// 		return err
// 	}

// 	sc, err := o.factory.Sysv1Client()
// 	if err != nil {
// 		return err
// 	}

// 	var sb *sysv1.Backup
// 	sb, err = sc.SysV1().Backups(o.manager.Namespace()).Get(ctx, name, metav1.GetOptions{})
// 	if err != nil && apierrors.IsNotFound(err) {
// 		sb, err = o.manager.CreateBackup(ctx, config, name, owner, velero.DefaultBackupTTL)
// 		if err != nil {
// 			return errors.WithStack(err)
// 		}
// 		log.Debugf("created fully backup %q", sb.Name)
// 	}
// 	return nil
// }

func (o *BackupPlan) GetLatest(ctx context.Context, name string) (*ListBackupsDetails, error) {
	bc, err := o.manager.GetBackupConfig(ctx, name)
	if err != nil {
		return nil, err
	}

	if err = o.manager.FormatSysBackupTimeofDay(bc); err != nil {
		log.Errorf("convert time to timestamp error: %v", err)
	}

	rs := ListBackupsDetails{
		Name:              name,
		SnapshotFrequency: bc.Spec.BackupPolicy.SnapshotFrequency,
	}

	l, err := o.manager.ListSysBackups(ctx, name)
	if err != nil {
		return nil, err
	}

	var latestSysBackup *sysv1.Backup
	if l != nil && l.Items != nil && len(l.Items) > 0 {
		latestSysBackup = &l.Items[0]
	}

	if latestSysBackup != nil && latestSysBackup.Spec.Size != nil {
		rs.Size = latestSysBackup.Spec.Size
	}

	phase, message := o.GetBackupResult(latestSysBackup)

	rs.NextBackupTimestamp = o.GetNextBackupTime(*bc.Spec.BackupPolicy)
	rs.Phase = phase
	rs.FailedMessage = message

	if latestSysBackup != nil {
		rs.SnapshotName = latestSysBackup.Name
		rs.CreationTimestamp = latestSysBackup.ObjectMeta.CreationTimestamp.Unix()
	}

	return &rs, nil
}

func (o *BackupPlan) Get(ctx context.Context, name string) (*ResponseDescribeBackup, error) {
	bc, err := o.manager.GetBackupConfig(ctx, name)
	if err != nil {
		return nil, err
	}

	if err = o.manager.FormatSysBackupTimeofDay(bc); err != nil {
		log.Errorf("convert time to timestamp error: %v", err)
	}

	r := ResponseDescribeBackup{
		Name:           name,
		BackupPolicies: bc.Spec.BackupPolicy,
	}

	l, err := o.manager.ListSysBackups(ctx, name)
	if err != nil {
		return nil, err
	}

	// get the latest succeed backup size
	if l != nil && len(l.Items) > 0 {
		for _, i := range l.Items {
			phase, middlewarePhase := i.Spec.Phase, i.Spec.MiddleWarePhase
			if phase == nil || middlewarePhase == nil {
				continue
			}

			if *phase == velero.VeleroBackupCompleted && util.ListContains([]string{velero.Succeed, velero.Success}, *middlewarePhase) {
				if i.Spec.Size != nil {
					r.Size = i.Spec.Size
				}
			}
		}
	}

	return &r, nil
}

func (o *BackupPlan) Del(ctx context.Context, name string) error {
	return errors.New("to be implement")
}

func (o *BackupPlan) GetBackupResult(sysBackup *sysv1.Backup) (string, string) {
	if sysBackup == nil {
		return "", ""
	}
	resticPhase := util.ListContains([]string{velero.Succeed, velero.Success, velero.VeleroBackupCompleted}, *sysBackup.Spec.ResticPhase)
	middlewarePhase := util.ListContains([]string{velero.Succeed, velero.Success, velero.VeleroBackupCompleted}, *sysBackup.Spec.MiddleWarePhase)

	if resticPhase && middlewarePhase && *sysBackup.Spec.Phase == velero.VeleroBackupCompleted {
		return velero.VeleroBackupCompleted, ""
	} else if !resticPhase {
		return velero.Failed, *sysBackup.Spec.ResticFailedMessage
	} else {
		return velero.Failed, *sysBackup.Spec.FailedMessage
	}
}

func (o *BackupPlan) GetNextBackupTime(bp sysv1.BackupPolicy) *int64 {
	var res int64
	var n = time.Now().Local()
	var prefix int64 = util.ParseToInt64(bp.TimesOfDay) / 1000
	var incr = util.ParseToNextUnixTime(bp.SnapshotFrequency, bp.TimesOfDay, bp.DayOfWeek)

	switch bp.SnapshotFrequency {
	case "@weekly":
		var midweek = util.GetFirstDayOfWeek(n).AddDate(0, 0, bp.DayOfWeek)
		res = midweek.Unix() + incr + prefix
	default:
		var midnight = time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
		res = midnight.Unix() + incr + prefix
	}
	return &res
}
