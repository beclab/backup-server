package velero

import (
	"context"
	"time"

	v1crds "bytetrade.io/web3os/backup-server/config/crds/velero/v1/crds"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/pkg/errors"
	veleroutil "github.com/vmware-tanzu/velero/pkg/util/kube"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	kbclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func allCRDs() (*unstructured.UnstructuredList, error) {
	resources := newUnstructuredList()

	for _, crd := range v1crds.CRDs {
		crd.SetLabels(Labels())
		if err := appendUnstructured(resources, crd); err != nil {
			return nil, errors.Errorf("error appending v1 CRD %s: %s\n", crd.GetName(), err.Error())
		}
	}

	// only for v2alpha1crds

	//for _, crd := range v2alpha1crds.CRDs {
	//	crd.SetLabels(Labels())
	//	if err := appendUnstructured(resources, crd); err != nil {
	//		return nil, errors.Errorf("error appending v2alpha1 CRD %s: %s\n", crd.GetName(), err.Error())
	//	}
	//}
	return resources, nil
}

func crdsAreReadyFunc(factory client.Factory, unstructureList *unstructured.UnstructuredList) (fn wait.ConditionFunc, err error) {
	crds := make([]*unstructured.Unstructured, 0)
	for _, crd := range unstructureList.Items {
		crds = append(crds, &crd)
	}

	if len(crds) == 0 {
		// no CRDs to check so return
		return nil, errors.Errorf("no crds")
	}

	kbClient, err := factory.KubeBuilderClient()
	if err != nil {
		return nil, errors.WithMessage(err, "crds check ready")
	}

	// We assume that all Velero CRDs have the same GVK so we can use the GVK of the
	// first CRD to determine whether to use the v1beta1 or v1 API during polling.
	gvk := crds[0].GroupVersionKind()

	if gvk.Version == "v1beta1" {
		fn = crdV1Beta1ReadinessFn(kbClient, crds)
	} else if gvk.Version == "v1" {
		fn = crdV1ReadinessFn(kbClient, crds)
	} else {
		return nil, errors.Errorf("unsupported CRD version %q", gvk.Version)
	}

	return fn, nil
}

func waitUntilCRDsAreReady(factory client.Factory, unstructureList *unstructured.UnstructuredList) (bool, error) {
	fn, err := crdsAreReadyFunc(factory, unstructureList)
	if err != nil {
		return false, errors.WithMessage(err, "wait until crds ready")
	}

	err = wait.PollImmediate(time.Second, time.Minute, fn)
	if err != nil {
		return false, errors.WithMessage(err, "error polling for CRDs")
	}
	return true, nil
}

// crdV1Beta1ReadinessFn returns a function that can be used for polling to check
// if the provided unstructured v1beta1 CRDs are ready for use in the cluster.
func crdV1Beta1ReadinessFn(kbClient kbclient.Client, unstructuredCrds []*unstructured.Unstructured) func() (bool, error) {
	// Track all the CRDs that have been found and in ready state.
	// len should be equal to len(unstructuredCrds) in the happy path.
	return func() (bool, error) {
		foundCRDs := make([]*apiextv1beta1.CustomResourceDefinition, 0)
		for _, unstructuredCrd := range unstructuredCrds {
			crd := &apiextv1beta1.CustomResourceDefinition{}
			key := kbclient.ObjectKey{Name: unstructuredCrd.GetName()}
			err := kbClient.Get(context.Background(), key, crd)
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, errors.Errorf("error waiting for %q to be ready: %v", crd.GetName(), err)
			}
			foundCRDs = append(foundCRDs, crd)
		}

		if len(foundCRDs) != len(unstructuredCrds) {
			return false, nil
		}

		for _, crd := range foundCRDs {
			ready := veleroutil.IsV1Beta1CRDReady(crd)
			if !ready {
				return false, nil
			}
		}
		return true, nil
	}
}

// crdV1ReadinessFn returns a function that can be used for polling to check
// if the provided unstructured v1 CRDs are ready for use in the cluster.
func crdV1ReadinessFn(kbClient kbclient.Client, unstructuredCrds []*unstructured.Unstructured) func() (bool, error) {
	return func() (bool, error) {
		foundCRDs := make([]*apiextv1.CustomResourceDefinition, 0)
		for _, unstructuredCrd := range unstructuredCrds {
			crd := &apiextv1.CustomResourceDefinition{}
			key := kbclient.ObjectKey{Name: unstructuredCrd.GetName()}
			err := kbClient.Get(context.Background(), key, crd)
			if apierrors.IsNotFound(err) {
				log.Warn(err)
				return false, nil
			} else if err != nil {
				return false, errors.Errorf("error waiting for %q to be ready: %v", crd.GetName(), err)
			}
			foundCRDs = append(foundCRDs, crd)
		}

		if len(foundCRDs) != len(unstructuredCrds) {
			return false, nil
		}

		for _, crd := range foundCRDs {
			ready := veleroutil.IsV1CRDReady(crd)
			if !ready {
				return false, nil
			}
		}
		return true, nil
	}
}
