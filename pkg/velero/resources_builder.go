package velero

import (
	"context"
	"fmt"
	"strconv"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/converter"
	"bytetrade.io/web3os/backup-server/pkg/util/pointer"
	"github.com/pkg/errors"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
)

func Labels() map[string]string {
	return map[string]string{
		"component":                     Velero,
		"velero.io/exclude-from-backup": "true",
	}
}

func podLabels(userLabels ...map[string]string) map[string]string {
	base := Labels()
	for _, labels := range userLabels {
		for k, v := range labels {
			base[k] = v
		}
	}
	return base
}

func podAnnotations(userAnnotations map[string]string) map[string]string {
	base := map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/port":   "8085",
		"prometheus.io/path":   "/metrics",
	}
	for k, v := range userAnnotations {
		base[k] = v
	}
	return base
}

func newUnstructuredResourceWithGVK(gvk schema.GroupVersionKind, obj any) (*unstructured.Unstructured, error) {
	o, err := converter.ToUnstructured(obj)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	r := unstructured.Unstructured{Object: o}
	r.SetGroupVersionKind(gvk)
	return &r, nil
}

func newUnstructuredList() *unstructured.UnstructuredList {
	resources := new(unstructured.UnstructuredList)
	resources.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "List"})
	return resources
}

func appendUnstructured(list *unstructured.UnstructuredList, obj runtime.Object) error {
	u, err := converter.ToUnstructured(&obj)
	delete(u, "status")
	if err != nil {
		return errors.Errorf("append unstructured: %v", err)
	}
	list.Items = append(list.Items, unstructured.Unstructured{Object: u})
	return nil
}

func objectMeta(ns, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: ns,
		Labels:    Labels(),
	}
}

func BuildServiceAccount(ns string, annotations map[string]string) *corev1.ServiceAccount {
	objMeta := objectMeta(ns, DefaultVeleroServiceAccountName)
	objMeta.Annotations = annotations
	return &corev1.ServiceAccount{
		ObjectMeta: objMeta,
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
	}
}

func BuildClusterRoleBinding(ns string) *rbacv1.ClusterRoleBinding {
	crbName := Velero
	if ns != DefaultVeleroNamespace {
		crbName = fmt.Sprintf("%s-%s", Velero, ns)
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: objectMeta("", crbName),
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: rbacv1.SchemeGroupVersion.String(),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Namespace: ns,
				Name:      Velero,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	return crb
}

func BuildNamespace(namespace string) *corev1.Namespace {
	ns := &corev1.Namespace{
		ObjectMeta: objectMeta("", namespace),
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
	}

	ns.Labels["pod-security.kubernetes.io/enforce"] = "privileged"
	ns.Labels["pod-security.kubernetes.io/enforce-version"] = "latest"

	return ns
}

func BuildBackupStorageLocation(ns string, bc *sysv1.BackupConfigSpec) *velerov1api.BackupStorageLocation {
	provider := bc.Provider
	// s3ForcePathStyle := strconv.FormatBool(bc.S3ForcePathStyle)

	configMap := make(map[string]string)

	if bc.Provider == Minio {
		provider = AWS
		configMap = map[string]string{
			"region": bc.Region,
			//"s3ForcePathStyle": s3ForcePathStyle,
			"s3Url": bc.S3Url,
		}
	}

	return &velerov1api.BackupStorageLocation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bc.StorageLocation,
			Namespace: ns,
			Labels: map[string]string{
				"component": Velero,
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "BackupStorageLocation",
			APIVersion: velerov1api.SchemeGroupVersion.String(),
		},
		Spec: velerov1api.BackupStorageLocationSpec{
			Provider: provider,
			StorageType: velerov1api.StorageType{
				ObjectStorage: &velerov1api.ObjectStorageLocation{
					Bucket: bc.Bucket,
					Prefix: bc.Prefix,
					//CACert: []byte(bc.CaCert),
				},
			},
			Config:  configMap,
			Default: true,
		},
	}
}

func BuildSecretApplyConfiguration(ns, name string, data []byte) *applycorev1.SecretApplyConfiguration {
	return applycorev1.Secret(name, ns).
		WithLabels(Labels()).
		WithData(map[string][]byte{"cloud": data})
}

// func (v *velero) DefaultBackupConfigSpec() (bc *sysv1.BackupConfigSpec, err error) {
// 	sc, err := v.factory.Sysv1Client()
// 	if err != nil {
// 		return nil, err
// 	}

// 	l, err := sc.SysV1().BackupConfigs(v.namespace).
// 		List(context.Background(), metav1.ListOptions{})
// 	if err != nil {
// 		return nil, errors.WithStack(err)
// 	}
// 	if len(l.Items) > 0 {
// 		return &l.Items[0].Spec, nil
// 	}

// 	return nil, errors.WithStack(fmt.Errorf("no default backup config can be found"))
// }

func (v *velero) BuildSysBackup(ctx context.Context, config, name, owner, bsl, backupType string, retainDays int64) (*sysv1.Backup, error) {
	terminusVersion, err := v.GetTerminusVersion(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &sysv1.Backup{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Backup",
			APIVersion: sysv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: v.namespace,
			Labels: podLabels(map[string]string{
				LabelStorageLocation: bsl,
				LabelBackupConfig:    config,
			}),
		},
		Spec: sysv1.BackupSpec{
			Owner:           pointer.String(owner),
			Phase:           pointer.String(Pending),
			TerminusVersion: terminusVersion,
			Extra: map[string]string{
				ExtraBackupType:            backupType,
				ExtraBackupStorageLocation: bsl,
				ExtraRetainDays:            strconv.FormatInt(retainDays, 10),
			},
		},
	}, nil
}

// func (v *velero) BuildVeleroBackup(name, owner, backupType string, retainDays int64) *velerov1api.Backup {
// 	bc, err := v.DefaultBackupConfigSpec()
// 	if err != nil {
// 		return nil
// 	}

// 	return &velerov1api.Backup{
// 		TypeMeta: metav1.TypeMeta{
// 			Kind:       "Backup",
// 			APIVersion: velerov1api.SchemeGroupVersion.String(),
// 		},
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      name,
// 			Namespace: v.namespace,
// 			Labels: podLabels(map[string]string{
// 				LabelStorageLocation: bc.StorageLocation,
// 			}),
// 			Annotations: map[string]string{
// 				AnnotationBackupRetainDays: strconv.FormatInt(retainDays, 10),
// 				AnnotationBackupOwner:      owner,
// 				AnnotationBackupType:       backupType,
// 			},
// 		},
// 		Spec: velerov1api.BackupSpec{
// 			IncludedNamespaces: []string{"*"},
// 			ExcludedResources: []string{
// 				"pods",
// 				"nodes",
// 				"events",
// 				"replicasets",
// 			},
// 			IncludeClusterResources: pointer.Bool(true),
// 			TTL:                     metav1.Duration{Duration: time.Duration(retainDays) * 24 * time.Hour},
// 			StorageLocation:         bc.StorageLocation,
// 			ItemOperationTimeout:    metav1.Duration{Duration: time.Hour},
// 		},
// 	}
// }

// func (v *velero) buildBackupSchedule(name, owner, schedule, backupType string, paused bool, retainDays int64) *velerov1api.Schedule {
// 	vb := v.BuildVeleroBackup(name, owner, backupType, retainDays)

// 	return &velerov1api.Schedule{
// 		TypeMeta: metav1.TypeMeta{
// 			Kind:       "Schedule",
// 			APIVersion: velerov1api.SchemeGroupVersion.String(),
// 		},
// 		ObjectMeta: metav1.ObjectMeta{
// 			Namespace:   v.namespace,
// 			Name:        name,
// 			Labels:      vb.Labels,
// 			Annotations: vb.Annotations,
// 		},
// 		Spec: velerov1api.ScheduleSpec{
// 			Template:                   vb.Spec,
// 			Schedule:                   schedule,
// 			UseOwnerReferencesInBackup: pointer.Bool(false),
// 			Paused:                     paused,
// 		},
// 	}
// }

// func (v *velero) buildIncrementalBackupSchedule(name, owner, schedule string, paused bool, retainDays int64) *velerov1api.Schedule {
// 	return v.buildBackupSchedule(name, owner, schedule, IncrementalBackup, paused, retainDays)
// }

func (v *velero) BuildVeleroBackupX(ctx context.Context, name, jobTime, owner, backupType string, retainDays int64) *velerov1api.Backup {
	bc, err := v.GetBackupConfig(ctx, name)
	if err != nil {
		return nil
	}

	return &velerov1api.Backup{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Backup",
			APIVersion: velerov1api.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: v.namespace,
			Labels: podLabels(map[string]string{
				LabelStorageLocation: bc.Spec.StorageLocation,
			}),
			Annotations: map[string]string{
				AnnotationBackupRetainDays: strconv.FormatInt(retainDays, 10),
				AnnotationBackupOwner:      owner,
				AnnotationBackupType:       backupType,
				AnnotationBackupJobTime:    jobTime,
			},
		},
		Spec: velerov1api.BackupSpec{
			IncludedNamespaces: []string{"*"},
			ExcludedResources: []string{
				"pods",
				"nodes",
				"events",
				"replicasets",
			},
			OrLabelSelectors: []*metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app.kubernetes.io/part-of",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"percona-server-mongodb"},
						},
						{
							Key:      "managed-by",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"redis-cluster-operator", "citus-operator"},
						},
					},
				},
			},
			IncludeClusterResources: pointer.Bool(true),
			SnapshotVolumes:         pointer.Bool(false),
			TTL:                     metav1.Duration{Duration: time.Duration(retainDays) * 24 * time.Hour},
			StorageLocation:         bc.Spec.StorageLocation,
			ItemOperationTimeout:    metav1.Duration{Duration: time.Hour},
		},
	}
}
