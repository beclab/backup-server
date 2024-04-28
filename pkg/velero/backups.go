package velero

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/util/pointer"
	"github.com/pkg/errors"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

func (v *velero) ListBackups(ctx context.Context) (*velerov1api.BackupList, error) {
	vc, err := v.factory.Client()
	if err != nil {
		return nil, err
	}

	backups, err := vc.VeleroV1().Backups(DefaultVeleroNamespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	sort.Slice(backups.Items, func(i, j int) bool {
		return !backups.Items[i].ObjectMeta.CreationTimestamp.Before(&backups.Items[j].ObjectMeta.CreationTimestamp)
	})

	return backups, nil
}

func (v *velero) BackupStatus(ctx context.Context, name string) (bool, string, error) {
	// sysv1 backup status
	sb, err := v.GetSysBackup(ctx, name)
	if err != nil {
		return false, "", err
	}

	return v.backupStatus(sb)
}

func (v *velero) backupStatus(sb *sysv1.Backup) (bool, string, error) {
	var (
		phase *string
		err   error
	)

	phase = sb.Spec.ResticPhase
	if phase == nil {
		switch {
		case sb.Spec.Phase != nil:
			switch *sb.Spec.Phase {
			case Pending, Started, InProgress:
				return false, *sb.Spec.Phase, nil
			case FinalizingPartiallyFailed,
				PartiallyFailed,
				Failed,
				FailedValidation:
				return false, Failed, errors.New(*sb.Spec.FailedMessage)
			case VeleroBackupCompleted:
				if sb.Spec.MiddleWarePhase == nil {
					return false, Running, nil
				}

				switch *sb.Spec.MiddleWarePhase {
				case Running:
					return false, Running, nil
				case Success:
					return true, *phase, nil
				case Failed:
					err = errors.New("middleware backup failed")
					if sb.Spec.MiddleWareFailedMessage != nil {
						err = errors.New(*sb.Spec.MiddleWareFailedMessage)
					}
					return false, Failed, err
				}

			}
		}
	}

	switch *phase {
	case Succeed:
		return true, *phase, nil
	case Running:
		return false, *phase, nil
	case Failed:
		return false, *phase, errors.New(*sb.Spec.ResticFailedMessage)
	}

	return false, "", errors.New("unknown backup status")
}

func (v *velero) GetVeleroBackup(ctx context.Context, name string) (*velerov1api.Backup, error) {
	vc, err := v.factory.Client()
	if err != nil {
		return nil, err
	}

	backup, err := vc.VeleroV1().Backups(DefaultVeleroNamespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.WithStack(err)
	}
	return backup, nil
}

func (v *velero) ListSysBackups(ctx context.Context, config string) (*sysv1.BackupList, error) {
	c, err := v.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	opts := metav1.ListOptions{}
	if config != "" {
		opts.LabelSelector = fmt.Sprintf("%s=%s", LabelBackupConfig, config)
	}

	res, err := c.SysV1().Backups(v.namespace).List(ctx, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	sort.Slice(res.Items, func(i, j int) bool {
		return !res.Items[i].ObjectMeta.CreationTimestamp.Before(&res.Items[j].ObjectMeta.CreationTimestamp)
	})
	return res, nil
}

func (v *velero) GetSysBackup(ctx context.Context, name string) (*sysv1.Backup, error) {
	c, err := v.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	b, err := c.SysV1().Backups(v.namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return b, nil
}

func (v *velero) ExistRunningBackup(ctx context.Context, config string) (bool, error) {
	// only one backup in progress
	backups, err := v.ListSysBackups(ctx, config)
	if err != nil {
		return true, err
	}
	for _, b := range backups.Items {
		_, phase, err := v.backupStatus(&b)
		if err != nil {
			klog.Error("get running backup err,", err)
		}

		if !util.ListContains([]string{Succeed, Failed}, phase) {
			return true, nil
		}

	}

	return false, nil
}

func (v *velero) CreateBackup(ctx context.Context, config, name, owner string, retainDays int64) (*sysv1.Backup, error) {
	_, err := v.ExistRunningBackup(ctx, config)
	if err != nil {
		return nil, err
	}

	bc, err := v.GetBackupConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	exists, err := v.ListSysBackups(ctx, config)
	if err != nil {
		log.Error("list exsits backup tasks error, ", err, ", ", config)
		return nil, err
	}

	sysv1Client, err := v.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	// first backup task for the config is full backup
	backupType := FullyBackup
	if len(exists.Items) > 0 {
		backupType = IncrementalBackup
	}
	backup, err := v.BuildSysBackup(ctx, config, name, owner, bc.Spec.StorageLocation, backupType, retainDays)
	if err != nil {
		return nil, err
	}
	createdBackup, err := sysv1Client.SysV1().Backups(v.namespace).
		Create(ctx, backup, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	log.Debugf("created sys backup %q: %v", name, util.PrettyJSON(createdBackup))
	return createdBackup, nil
}

func (v *velero) DownloadBackup(ctx context.Context, name, downloadToFilePath string) (int64, error) {
	return 0, nil
}

func (v *velero) AsyncOsDataBackup(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	var (
		err    error
		exitCh = make(chan error)
	)

	go v.backupOsData(ctx, name, exitCh)

	select {
	case e, ok := <-exitCh:
		log.Debugf("got exit channel, ok: %v, err: %v", ok, e)
		if ok && e != nil {
			err = e
		}
	case <-ctx.Done():
		err = errors.Errorf("backup %q osdata timed out in 2 hour", name)
	}

	if err != nil {
		log.Errorf("backup %q occurred err: %+v", name, err)
		v.updateBackupOrPanic(context.TODO(), name, sysv1.BackupSpec{
			ResticPhase:         pointer.String(Failed),
			ResticFailedMessage: pointer.String(err.Error())})
		return
	}
	log.Debugf("async to backup %q osdata finished", name)
}

func (v *velero) backupOsData(ctx context.Context, name string, exitCh chan<- error) {
	var (
		err      error
		metaData *MetaData

		sb *sysv1.Backup
	)

	defer func() {
		if e := recover(); e != nil {
			exitCh <- errors.Errorf("recovered osdata with backup %q: %v", name, e)
			return
		}
		exitCh <- nil
	}()

	sc, err := v.factory.Sysv1Client()
	if err != nil {
		exitCh <- err
		return
	}

	sb, err = sc.SysV1().Backups(v.namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		exitCh <- errors.WithStack(err)
		return
	}

	// backup type
	backupType, ok := sb.Spec.Extra[ExtraBackupType]
	if !ok {
		exitCh <- fmt.Errorf("invalid backup %q, no backup type is specified", sb.Name)
		return
	}

	v.updateBackupOrPanic(ctx, name, sysv1.BackupSpec{
		ResticPhase: pointer.String(Running),
	})

	// metadata
	log.Debugf("collecting metadata with backup %q", sb.Name)
	metaData, err = v.collectMetaData(ctx)
	if err != nil {
		exitCh <- err
		return
	} else {
		var data []byte
		data, err = json.Marshal(metaData)
		if err == nil && data != nil && len(data) > 0 {
			err = os.WriteFile(TerminusMetaDataFilePath, data, 0644)
		}
		if err != nil {
			exitCh <- err
			return
		}
	}

	bcName, ok := sb.Labels[LabelBackupConfig]
	if !ok {
		exitCh <- errors.New("no bc found")
		return
	}

	var bc *sysv1.BackupConfig
	bc, err = v.GetBackupConfig(ctx, bcName)
	if err != nil {
		exitCh <- err
		return
	}

	// restic snapshot backup
	log.Debugf("creating %q userdata restic backup", sb.Name)
	start := time.Now()

	isFullyBackup := backupType == FullyBackup

	repo, summary, err := v.createResticSnapshot(ctx, name, isFullyBackup, bc, sb)
	if err != nil {
		exitCh <- err
		return
	}
	log.Debugf("restic snapshot backup output summary: %s", util.PrettyJSON(summary))

	updatedBackup := v.updateBackupOrPanic(ctx, name, sysv1.BackupSpec{
		ResticPhase: pointer.String(Succeed),
		Size:        summary.TotalBytesProcessed,
		Extra: map[string]string{
			ExtraSnapshotId:   *summary.SnapshotID,
			ExtraS3Repository: repo.S3Repository,
			ExtraS3RepoPrefix: repo.Prefix,
		},
	})

	log.Debugf("create %q osdata backup total took (%s), updated backup: %s",
		sb.Name, time.Since(start), util.PrettyJSON(updatedBackup))
}

func (v *velero) createResticSnapshot(ctx context.Context, name string, isFullyBackup bool, bc *sysv1.BackupConfig,
	sb *sysv1.Backup) (*Repo, *ResticMessage, error) {
	log.Debugf("restic snapshot with backup %q", name)

	dynamicClient, _ := v.factory.DynamicClient()
	err := SetResticEnv(ctx, dynamicClient, &bc.Spec)
	if err != nil {
		return nil, nil, err
	}

	repo := GetRepository(name, string(sb.UID), bc)

RETRY:
	if isFullyBackup {
		password, err := getPasswordFromSettings(bc)
		if err != nil {
			return nil, nil, err
		}

		repositoryPassword := ""
		if password != "" {
			repositoryPassword = util.EncodeStringToBase64(password)
		} else if bc.Spec.RepositoryPassword != "" {
			repositoryPassword = bc.Spec.RepositoryPassword
		}

		if repositoryPassword == "" {
			klog.Error("restic password is not provided")
			return nil, nil, errors.New("backup password is empty")
		}

		os.Setenv("RESTIC_PASSWORD", util.DecodeBase64ToString(repositoryPassword))

		// init repository directly
		initArgs := []string{
			"-r",
			repo.S3Repository,
			"init",
			// DefaultOSDataPath,
		}
		log.Debugf("restic init repository, 'restic %s'", strings.Join(initArgs, " "))

		out, err := exec.Command("/usr/bin/restic", initArgs...).CombinedOutput()
		if err != nil {
			return nil, nil, errors.Errorf("%s, %v", string(out), err)
		}
		log.Debugf("restic init repository output:")
		fmt.Println(string(out))

		v.updateBackupOrPanic(ctx, name, sysv1.BackupSpec{
			Extra: map[string]string{
				ExtraRepositoryPassword:     repositoryPassword,
				ExtraRepositoryPasswordHash: util.BytesToSha256Hash([]byte(repositoryPassword)),
			},
		})
	} else {
		backups, err := v.ListSysBackups(ctx, bc.Name)
		if err != nil {
			return nil, nil, err
		}

		var latestFullBackup *sysv1.Backup

		for _, elem := range backups.Items {
			if elem.Spec.Extra != nil {
				if v, ok := elem.Spec.Extra[ExtraBackupType]; ok && v == FullyBackup &&
					elem.Spec.ResticPhase != nil &&
					*elem.Spec.ResticPhase == Succeed {
					latestFullBackup = &elem
					break
				}
			}
		}
		if latestFullBackup == nil {
			klog.Error(errors.Errorf("incremental backup %q, do not have fully backup yet", name))

			klog.Info("full backup not found, swtich increment to full backup")
			isFullyBackup = true
			goto RETRY
		}

		password, ok := latestFullBackup.Spec.Extra[ExtraRepositoryPassword]
		if !ok || password == "" {
			return nil, nil, errors.Errorf("incremental backup %q, has no repository password", name)
		}

		passwordHash, ok := latestFullBackup.Spec.Extra[ExtraRepositoryPasswordHash]
		if !ok || passwordHash == "" {
			return nil, nil, errors.Errorf("incremental backup %q, has no repository password hash", name)
		}

		log.Debugf("got backup %q ref full backup base64 encoded password: %q", name, password)
		os.Setenv("RESTIC_PASSWORD", util.DecodeBase64ToString(password))

		v.updateBackupOrPanic(ctx, name, sysv1.BackupSpec{
			Extra: map[string]string{
				ExtraRepositoryPasswordHash: passwordHash,
				ExtraRefFullyBackupUid:      string(latestFullBackup.UID),
				ExtraRefFullyBackupName:     latestFullBackup.Name,
			},
		})

		repo = GetRepository(latestFullBackup.Name, string(latestFullBackup.UID), bc)

		s3client, err := NewS3Client(&bc.Spec)
		if err != nil {
			return nil, nil, err
		}

		if exists, err := s3client.ObjectExists(ctx, repo.Key); err != nil {
			return nil, nil, err
		} else if !exists {
			return nil, nil, errors.Errorf("fully backup %q, not init repository %q",
				latestFullBackup.Name, repo.Name)
		}
	}

	// create snapshot backup
	backupArgs := []string{"-r", repo.S3Repository}

	for _, elem := range ExcludeOsDataDirOrFiles {
		backupArgs = append(backupArgs,
			"--exclude", filepath.Join(DefaultOSDataPath, elem))
	}
	backupArgs = append(backupArgs, "backup", DefaultOSDataPath, "--json")

	log.Debugf("restic creating snapshot, 'restic %s'", strings.Join(backupArgs, " "))

	out, err := exec.Command("/usr/bin/restic", backupArgs...).CombinedOutput()
	if err != nil {
		return nil, nil, errors.Errorf("%s, %v", string(out), err)
	}
	log.Debugf("created restic snapshot backup output:")
	fmt.Println(string(out))

	var (
		summary []byte
		message ResticMessage
	)
	for _, row := range strings.Split(string(out), "\n") {
		if strings.Contains(row, "{") && strings.Contains(row, "summary") {
			summary = []byte(row)
			break
		}
	}
	if summary == nil {
		return nil, nil, errors.New("failed to create restic snapshot backup")
	}
	if err = json.Unmarshal(summary, &message); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	return repo, &message, nil
}

func (v *velero) GetTerminusVersion(ctx context.Context, dc dynamic.Interface) (*string, error) {
	var err error

	if dc == nil {
		dc, err = v.factory.DynamicClient()
		if err != nil {
			return nil, err
		}
	}

	unstructuredTerminus, err := dc.Resource(TerminusGVR).
		Get(ctx, "terminus", metav1.GetOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	terminusObj := unstructuredTerminus.UnstructuredContent()
	terminusVersion, _, err := unstructured.NestedString(terminusObj, "spec", "version")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &terminusVersion, nil
}

func (v *velero) collectMetaData(ctx context.Context) (*MetaData, error) {
	metaData := MetaData{}

	kb, err := v.factory.KubeClient()
	if err != nil {
		return nil, err
	}

	nodes, err := kb.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	metaData.Nodes = len(nodes.Items)
	nodes = nil

	pods, err := kb.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	metaData.Pods = len(pods.Items)
	pods = nil

	// pv
	metaData.PersistentVolumes = []PersistentVolume(nil)

	appServiceSts, err := kb.AppsV1().StatefulSets("os-system").
		Get(ctx, "app-service", metav1.GetOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	pvValues := func(p string, annotations map[string]string) map[string]string {
		res := make(map[string]string)
		for _, k := range []string{"pv", "sc", "storage", "hostpath"} {
			value := v.getAnnotationValue(annotations, p+"_"+k)
			res[k] = value
		}
		return res
	}

	for _, p := range []string{"charts", "usertmpl"} {
		values := pvValues(p, appServiceSts.Annotations)
		metaData.PersistentVolumes = append(metaData.PersistentVolumes, PersistentVolume{
			Name:         values["pv"],
			Storage:      values["storage"],
			StorageClass: values["sc"],
			HostPath:     values["hostpath"],
		})
	}

	dc, err := v.factory.DynamicClient()
	if err != nil {
		return nil, err
	}

	// terminus version
	terminusVersion, err := v.GetTerminusVersion(ctx, dc)
	if err != nil {
		return nil, err
	}
	metaData.Version.Terminus = *terminusVersion

	// users
	unstructuredUsers, err := dc.Resource(UsersGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	metaData.Users = []User(nil)

	for _, unstructuredUser := range unstructuredUsers.Items {
		userName := unstructuredUser.GetName()
		ns := "user-space-" + userName
		annotations := unstructuredUser.GetAnnotations()

		obj := unstructuredUser.UnstructuredContent()
		email, _, err := unstructured.NestedString(obj, "spec", "email")
		if err != nil {
			return nil, errors.WithStack(err)
		}

		user := User{
			Name:        userName,
			Email:       email,
			Did:         v.getAnnotationValue(annotations, "bytetrade.io/did"),
			Zone:        v.getAnnotationValue(annotations, "bytetrade.io/zone"),
			OwnerRole:   v.getAnnotationValue(annotations, "bytetrade.io/owner-role"),
			Annotations: annotations,
		}
		user.PersistentVolumes = []PersistentVolume(nil)

		bfl, err := kb.AppsV1().StatefulSets(ns).Get(ctx, "bfl", metav1.GetOptions{})
		if err != nil {
			return nil, errors.WithStack(err)
		}
		for _, p := range []string{"userspace", "appdata", "dbdata"} {
			values := pvValues(p, bfl.Annotations)
			user.PersistentVolumes = append(user.PersistentVolumes, PersistentVolume{
				Name:         values["pv"],
				Storage:      values["storage"],
				StorageClass: values["sc"],
				HostPath:     values["hostpath"],
			})
		}
		log.Debugf("collected user %q metadata: %s", userName, util.PrettyJSON(user))
		metaData.Users = append(metaData.Users, user)
	}

	return &metaData, nil
}

func (v *velero) getAnnotationValue(annotations map[string]string, k string) string {
	for key, value := range annotations {
		if k == key {
			return value
		}
	}
	return ""
}

func (v *velero) updateBackupOrPanic(ctx context.Context, name string, o sysv1.BackupSpec) *sysv1.Backup {
	var retry int

	c, err := v.factory.Sysv1Client()
	if err != nil {
		panic(err)
	}

	log.Debugf("update backup %q or panic, %s", name, util.PrettyJSON(o))

RETRY:
	if retry >= 3 {
		return nil
	}
	sb, err := c.SysV1().Backups(v.namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		panic(err)
	}

	if o.ResticPhase != nil {
		sb.Spec.ResticPhase = o.ResticPhase
	}
	if o.Size != nil {
		sb.Spec.Size = o.Size
	}
	if o.ResticFailedMessage != nil {
		sb.Spec.ResticFailedMessage = o.ResticFailedMessage
	}

	if o.Extra != nil {
		if sb.Spec.Extra == nil {
			sb.Spec.Extra = make(map[string]string)
		}
		for k, v := range o.Extra {
			sb.Spec.Extra[k] = v
		}
	}

	updatedBackup, err := c.SysV1().Backups(v.namespace).
		Update(ctx, sb, metav1.UpdateOptions{})
	if err != nil && apierrors.IsConflict(err) {
		log.Warnf("update backup %q, apiserver has conflict, retry it", name)
		retry++
		goto RETRY
	} else if err != nil {
		panic(err)
	}
	return updatedBackup
}

type backupJob struct {
	name string
	f    func()
}

func (b backupJob) Run() { b.f() }

func (v *velero) ApplyBackupSchedule(ctx context.Context, name, owner, schedule string, paused bool) (*velerov1api.Schedule, error) {
	klog.Info("create backup schedule, ", schedule)

	entries := v.cron.Entries()
	for _, e := range entries {
		if e.Job.(backupJob).name == name {
			klog.Info("remove prev cron job to apply new one")
			v.cron.Remove(e.ID)
		}
	}

	_, err := v.cron.AddJob(schedule, backupJob{
		name: name,
		f: func() {
			klog.Info("start to create backup task")

			backupName := fmt.Sprintf("%s-%v", name, time.Now().Format("20060102150405"))
			_, err := v.CreateBackup(ctx, name, backupName, owner, DefaultBackupTTL)
			if err != nil {
				klog.Error("create backup task error, ", err)
			}
		},
	})

	if err != nil {
		klog.Error("add backup schedule error, ", err)
	}

	return nil, err
}

func (v *velero) DeleteBackup(ctx context.Context, name string, sb *sysv1.Backup) error {
	config := sb.Labels[LabelBackupConfig]
	if config == "" {
		return errors.New("backup config not found for backup")
	}

	bc, err := v.GetBackupConfig(ctx, config)
	if err != nil {
		return err
	}

	dynamicClient, _ := v.factory.DynamicClient()
	err = SetResticEnv(ctx, dynamicClient, &bc.Spec)
	if err != nil {
		return err
	}

	if sb != nil {
		if sb.Spec.Extra != nil {
			var (
				backupType string
				s3Repo     string
			)
			extra := sb.Spec.Extra
			if v, ok := extra[ExtraBackupType]; ok && v != "" {
				backupType = v
			}
			if v, ok := extra[ExtraS3Repository]; ok && v != "" {
				s3Repo = v
			}
			if backupType == IncrementalBackup && s3Repo != "" { // delete restic snapshot only
				if snapshotId, ok := sb.Spec.Extra[ExtraSnapshotId]; ok && snapshotId != "" {
					log.Debugf("deleting backup %q, restic snapshot id: %q", name, snapshotId)
					err = v.deleteResticSnapshot(ctx, &bc.Spec, sb, snapshotId, s3Repo)
					if err != nil {
						log.Errorf("delete backup %q restic snapshot: %v", name, err)
					}
				}
			} else if backupType == FullyBackup && s3Repo != "" {
				c, err := NewS3Client(&bc.Spec)
				if err != nil {
					return err
				}
				if err = c.DeleteObject(ctx, s3Repo); err != nil {
					log.Warnf("delete s3 %q, %v", s3Repo, err)
				}
			}
		}
	}

	return nil
}

func (v *velero) getRefFullyBackup(ctx context.Context, sb *sysv1.Backup) (*string, error) {
	name := sb.Name

	if sb.Spec.Extra == nil {
		return nil, errors.Errorf("backup %q no extra", name)
	}
	backupType, ok := sb.Spec.Extra[ExtraBackupType]
	if !ok {
		return nil, errors.Errorf("backup %q no backup specific", name)
	}
	if backupType != IncrementalBackup {
		return nil, errors.Errorf("backup %q not incremental backup", name)
	}

	var (
		refUID  string
		refName string
	)
	if refUID, ok = sb.Spec.Extra[ExtraRefFullyBackupUid]; !ok {
		return nil, errors.Errorf("backup %q no ref backup uid", name)
	}
	if refName, ok = sb.Spec.Extra[ExtraRefFullyBackupName]; !ok {
		return nil, errors.Errorf("backup %q no ref backup name", name)
	}

	configName := sb.Labels[LabelBackupConfig]
	l, err := v.ListSysBackups(ctx, configName)
	if err != nil {
		return nil, err
	}

	for _, i := range l.Items {
		if i.Name == refName && string(i.UID) == refUID {
			if password, ok := i.Spec.Extra[ExtraRepositoryPassword]; ok && password != "" {
				return pointer.String(password), nil
			}
		}
	}
	return nil, errors.Errorf("incremental backup %q has no ref backup specified", name)
}

func (v *velero) deleteResticSnapshot(ctx context.Context, bc *sysv1.BackupConfigSpec, sb *sysv1.Backup, id, repo string) error {
	passwordHash, err := v.getRefFullyBackup(ctx, sb)
	if err != nil {
		return err
	}
	password := util.DecodeBase64ToString(*passwordHash)

	log.Debugf("delete restic snapshot %q with password: %q", id, password)
	args := []string{
		"-v",
		"-r",
		repo,
		"forget",
		id,
	}
	log.Debugf("restic forget snapshot %q, command: 'restic %s'", id, strings.Join(args, " "))

	os.Setenv("RESTIC_PASSWORD", password)
	out, err := exec.Command("/usr/bin/restic", args...).CombinedOutput()
	if err != nil {
		return errors.Errorf("%s, %v", string(out), err)
	}
	log.Debugf("restic forget snapshot %q output:", id)
	fmt.Println(string(out))
	return nil
}

func (v *velero) DeleteVeleroBackup(ctx context.Context, namespace, name string) error {
	vc, err := v.factory.Client()
	if err != nil {
		return err
	}

	if _, err = vc.VeleroV1().DeleteBackupRequests(namespace).Create(ctx, &velerov1api.DeleteBackupRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: velerov1api.DeleteBackupRequestSpec{
			BackupName: name,
		},
	}, metav1.CreateOptions{}); err != nil {
		return err
	}
	return nil
}

func (v *velero) GetBackupSchedule(ctx context.Context, name string) (*velerov1api.Schedule, error) {
	vc, err := v.factory.Client()
	if err != nil {
		return nil, err
	}
	schedule, err := vc.VeleroV1().Schedules(v.namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return schedule, nil
}
