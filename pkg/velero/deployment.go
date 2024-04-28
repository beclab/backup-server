package velero

import (
	"context"
	"strings"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	applyappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func DeploymentApplyConfiguration(namespace string, bcSpec *sysv1.BackupConfigSpec) *applyappsv1.DeploymentApplyConfiguration {
	deployment := applyappsv1.Deployment(DefaultVeleroDeploymentName, namespace).
		WithLabels(Labels())

	// volumes
	volumes := []*applycorev1.VolumeApplyConfiguration{
		applycorev1.Volume().WithName("plugins").
			WithEmptyDir(applycorev1.EmptyDirVolumeSource()),
		applycorev1.Volume().WithName("scratch").
			WithEmptyDir(applycorev1.EmptyDirVolumeSource()),
	}

	// pod volumeMounts
	volumeMounts := []*applycorev1.VolumeMountApplyConfiguration{
		applycorev1.VolumeMount().WithName("plugins").WithMountPath("/plugins"),
		applycorev1.VolumeMount().WithName("scratch").WithMountPath("/scratch"),
	}

	podEnvNamespaceSource := applycorev1.EnvVarSource().
		WithFieldRef(applycorev1.ObjectFieldSelector().
			WithFieldPath("metadata.namespace"))

	envVars := []*applycorev1.EnvVarApplyConfiguration{
		applycorev1.EnvVar().WithName("VELERO_SCRATCH_DIR").WithValue("/scratch"),
		applycorev1.EnvVar().WithName("VELERO_NAMESPACE").WithValueFrom(podEnvNamespaceSource),
		applycorev1.EnvVar().WithName("LD_LIBRARY_PATH").WithValue("/plugins"),
	}

	// if !bcSpec.NoSecret {
	secretVolume := applycorev1.Volume().
		WithName(DefaultVeleroSecretName).
		WithSecret(applycorev1.SecretVolumeSource().WithSecretName(DefaultVeleroSecretName))

	secretVolumeMount := applycorev1.VolumeMount().
		WithName(DefaultVeleroSecretName).
		WithMountPath("/credentials")

	volumes = append(volumes, secretVolume)
	volumeMounts = append(volumeMounts, secretVolumeMount)

	envVars = append(envVars,
		applycorev1.EnvVar().WithName("GOOGLE_APPLICATION_CREDENTIALS").WithValue("/credentials/cloud"),
		applycorev1.EnvVar().WithName("AWS_SHARED_CREDENTIALS_FILE").WithValue("/credentials/cloud"),
		applycorev1.EnvVar().WithName("AZURE_CREDENTIALS_FILE").WithValue("/credentials/cloud"),
		applycorev1.EnvVar().WithName("ALIBABA_CLOUD_CREDENTIALS_FILE").WithValue("/credentials/cloud"))
	// }

	// plugins initContainer
	initContainers := []*applycorev1.ContainerApplyConfiguration(nil)

	if len(bcSpec.Plugins) > 0 {
		for _, image := range bcSpec.Plugins {
			initContainers = append(initContainers, buildApplyPluginContainer(image))
		}
	}

	resourceRequirements := applycorev1.ResourceRequirements()
	resourceRequirements.WithRequests(map[corev1.ResourceName]resource.Quantity{
		"cpu":    resource.MustParse("50m"),
		"memory": resource.MustParse("100Mi"),
	})
	resourceRequirements.WithLimits(map[corev1.ResourceName]resource.Quantity{
		"cpu":    resource.MustParse("1500m"),
		"memory": resource.MustParse("2Gi"),
	})

	// pod containers
	containers := []*applycorev1.ContainerApplyConfiguration{
		applycorev1.Container().
			WithName(Velero).
			WithImage(DefaultVeleroImage).
			WithImagePullPolicy(corev1.PullIfNotPresent).
			WithPorts(applycorev1.ContainerPort().
				WithName("metrics").
				WithContainerPort(8085)).
			WithCommand("/velero").
			WithArgs("server", "--features=", "--uploader-type=restic").
			WithEnv(envVars...).
			WithResources(resourceRequirements).
			WithVolumeMounts(volumeMounts...),
	}

	podSpec := applycorev1.PodSpec().
		WithRestartPolicy(corev1.RestartPolicyAlways).
		WithServiceAccountName(DefaultVeleroServiceAccountName).
		WithVolumes(volumes...).
		WithInitContainers(initContainers...).
		WithContainers(containers...)

	templateSpec := applycorev1.PodTemplateSpec().
		WithLabels(podLabels(nil, map[string]string{"deploy": Velero})).
		WithAnnotations(podAnnotations(nil)).
		WithSpec(podSpec)

	deployment.Spec = applyappsv1.DeploymentSpec().
		WithReplicas(1).
		WithSelector(applymetav1.LabelSelector().
			WithMatchLabels(map[string]string{"deploy": Velero})).
		WithTemplate(templateSpec)

	return deployment
}

func buildApplyPluginContainer(image string) *applycorev1.ContainerApplyConfiguration {
	name := getName(image)

	volumeMount := applycorev1.VolumeMount().
		WithName("plugins").
		WithMountPath("/target")

	return applycorev1.Container().
		WithName(name).
		WithImage(image).
		WithImagePullPolicy(corev1.PullIfNotPresent).
		WithVolumeMounts(volumeMount)
}

func getName(image string) string {
	slashIndex := strings.Index(image, "/")
	slashCount := 0
	if slashIndex >= 0 {
		slashCount = strings.Count(image[slashIndex:], "/")
	}

	start := 0
	if slashCount > 1 || slashIndex == 0 {
		start = slashIndex + 1
	}
	end := len(image)
	atIndex := strings.LastIndex(image, "@")
	if atIndex > 0 {
		end = atIndex
	} else {
		colonIndex := strings.LastIndex(image, ":")
		if colonIndex > 0 {
			end = colonIndex
		}
	}

	re := strings.NewReplacer("/", "-",
		"_", "-",
		".", "-")
	return re.Replace(image[start:end])
}

func isAvailable(c appsv1.DeploymentCondition) bool {
	if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
		if !c.LastTransitionTime.IsZero() && c.LastTransitionTime.Add(10*time.Second).Before(time.Now()) {
			return true
		}
	}
	return false
}

func deploymentReadyFunc(ctx context.Context, kubeclient kubernetes.Interface, namespace string) (fn wait.ConditionFunc) {
	name := DefaultVeleroDeploymentName

	return func() (done bool, err error) {
		deploy, err := kubeclient.AppsV1().Deployments(namespace).
			Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Errorf("describe %q deployment: %v", name, err)
		}

		for _, c := range deploy.Status.Conditions {
			if !isAvailable(c) {
				return false, errors.Errorf("%q deployment conditions is unavailable", name)
			}
		}
		return true, nil
	}
}

func waitUntilDeploymentReady(ctx context.Context, kubeclient kubernetes.Interface, namespace string) (bool, error) {
	var isReady bool
	var readyObservations int

	err := wait.PollImmediate(time.Second, 5*time.Minute, func() (done bool, err error) {
		if done, err := deploymentReadyFunc(ctx, kubeclient, namespace)(); done && err == nil {
			readyObservations++
		}
		if readyObservations > 4 {
			isReady = true
			return true, nil
		}
		return false, nil
	})

	return isReady, errors.WithStack(err)
}
