package v1

import (
	"context"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/modules/backup/v1/operator"
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
	owner            string
	c                *BackupCreate
	factory          client.Factory
	manager          velero.Manager
	backupOperator   *operator.BackupOperator
	snapshotOperator *operator.SnapshotOperator
}

func NewBackupPlan(owner string, factory client.Factory, manager velero.Manager, backupOperator *operator.BackupOperator) *BackupPlan {
	return &BackupPlan{
		owner:          owner,
		factory:        factory,
		manager:        manager,
		backupOperator: backupOperator,
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
	if o.c.Name == "" {
		return errors.New("name is required")
	}
	if o.owner == "" {
		return errors.New("owner is required")
	}

	if err := o.validLocation(); err != nil {
		return err
	}

	if err := o.validBackupPolicy(); err != nil {
		return err
	}

	if err := o.validPassword(); err != nil {
		return err
	}

	return nil
}

func (o *BackupPlan) mergeConfig(name string) *sysv1.BackupSpec {
	var locationName = o.c.Location
	var location = make(map[string]string)
	location[locationName] = util.ToJSON(o.c.LocationConfig)

	bc := &sysv1.BackupSpec{
		Name:     o.c.Name,
		Owner:    o.owner,
		Location: location,
	}
	if o.c.BackupPolicies != nil {
		bc.BackupPolicy = o.c.BackupPolicies
	}
	return bc
}

func (o *BackupPlan) apply(ctx context.Context, name string) error {
	var (
		backupSpec *sysv1.BackupSpec
	)

	backupSpec = o.mergeConfig(name)
	if o.c != nil {
		// todo update
		// if o.c.Location != "" && configSpec.Location != o.c.Location {
		// 	return errors.New("change location is not allowed")
		// }
	}

	log.Debugf("merged backup spec: %s", util.PrettyJSON(backupSpec))

	backup, err := o.backupOperator.CreateBackup(ctx, backupSpec)
	if err != nil {
		return err
	}

	log.Infof("create backup %s, uid %s", backup.Name, backup.UID)

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

func (o *BackupPlan) GetLatest(ctx context.Context, name string) (*ResponseDescribeBackup, error) {
	bc, err := o.manager.GetBackupConfig(ctx, name)
	if err != nil {
		return nil, err
	}

	if err = o.manager.FormatSysBackupTimeofDay(bc); err != nil {
		log.Errorf("convert time to timestamp error: %v", err)
	}

	rs := ResponseDescribeBackup{
		Name:           name,
		BackupPolicies: bc.Spec.BackupPolicy,
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

func (o *BackupPlan) validLocation() error {
	log.Infof("new backup %s location %s", o.c.Name, util.ToJSON(o.c.LocationConfig))

	location := o.c.Location
	locationConfig := o.c.LocationConfig

	if ok := util.ListContains([]string{constant.BackupLocationSpace.String(), constant.BackupLocationS3.String()}, location); !ok {
		return errors.Errorf("backup %s location %s not support", o.c.Name, location)
	}

	if location == constant.BackupLocationSpace.String() {
		if locationConfig.CloudName == "" || locationConfig.RegionId == "" {
			return errors.Errorf("backup %s location space invalid, cloudName: %s, regionId: %s", o.c.Name, locationConfig.CloudName, locationConfig.RegionId)
		}
	} else {
		if locationConfig.Endpoint == "" || locationConfig.AccessKey == "" || locationConfig.SecretKey == "" {
			return errors.Errorf("backup %s location s3 invalid, please check endpoint, accessKey, secretKey", o.c.Name)
		}
	}
	return nil
}

func (o *BackupPlan) validBackupPolicy() error {
	log.Infof("new backup %s location %s", o.c.Name, util.ToJSON(o.c.BackupPolicies))

	policy := o.c.BackupPolicies

	if ok := util.ListContains([]string{
		constant.BackupSnapshotFrequencyHourly.String(),
		constant.BackupSnapshotFrequencyDaily.String(),
		constant.BackupSnapshotFrequencyWeekly.String(),
		constant.BackupSnapshotFrequencyMonthly.String(),
	}, policy.SnapshotFrequency); !ok {
		return errors.Errorf("backup %s snapshot frequency %s not support", o.c.Name, policy.SnapshotFrequency)
	}

	_, err := util.ParseTimestampToLocal(o.c.BackupPolicies.TimesOfDay)
	if err != nil {
		return errors.Errorf("backup %s snapshot times of day invalid, eg: '48600000'", o.c.Name)
	}

	return nil

}

func (o *BackupPlan) validPassword() error {
	if !(o.c.Password == o.c.ConfirmPassword && o.c.Password != "") {
		return errors.Errorf("password not match")
	}
	return nil
}
