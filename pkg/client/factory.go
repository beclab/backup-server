package client

import (
	"bytetrade.io/web3os/backup-server/pkg/converter"
	"github.com/pkg/errors"
	veleroclient "github.com/vmware-tanzu/velero/pkg/client"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	k8scheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	kbclient "sigs.k8s.io/controller-runtime/pkg/client"

	// velerov2alpha1api "bytetrade.io/web3os/backup-server/pkg/apis/velero/v2alpha1"
	sysv1 "bytetrade.io/web3os/backup-server/pkg/generated/clientset/versioned"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	clientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
)

type Factory interface {
	ClientConfig() (*rest.Config, error)

	Client() (clientset.Interface, error)

	DynamicClient() (dynamic.Interface, error)

	KubeClient() (kubernetes.Interface, error)

	Sysv1Client() (sysv1.Interface, error)

	KubeBuilderClient() (kbclient.Client, error)

	VeleroDynamicFactory() veleroclient.DynamicFactory

	ClientForGroupVersion(gv schema.GroupVersion, apiResource metav1.APIResource, namespace string) (veleroclient.Dynamic, error)

	ClientForUnstructured(r *unstructured.Unstructured) (veleroclient.Dynamic, error)
}

var _ Factory = &factory{}

type factory struct {
	config *rest.Config

	client dynamic.Interface
}

func NewFactory() (Factory, error) {
	config, err := ctrl.GetConfig()
	if err != nil {
		return nil, errors.Errorf("new rest kubeconfig: %v", err)
	}

	config.Burst = 15
	config.QPS = 50

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, errors.Errorf("new dynamic client: %v", err)
	}

	f := &factory{
		config: config,
		client: client,
	}
	return f, nil
}

func (f *factory) ClientConfig() (*rest.Config, error) {
	return f.config, nil
}

func (f *factory) Client() (clientset.Interface, error) {
	c, err := clientset.NewForConfig(f.config)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return c, nil
}

func (f *factory) DynamicClient() (dynamic.Interface, error) {
	return f.client, nil
}

func (f *factory) KubeClient() (kubernetes.Interface, error) {
	c, err := kubernetes.NewForConfig(f.config)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return c, nil
}

func (f *factory) Sysv1Client() (sysv1.Interface, error) {
	c, err := sysv1.NewForConfig(f.config)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return c, nil
}

func (f *factory) KubeBuilderClient() (kbclient.Client, error) {
	var err error

	scheme := runtime.NewScheme()
	if err = velerov1api.AddToScheme(scheme); err != nil {
		return nil, errors.Errorf("add schema velerov1api: %v", err)
	}
	// if err = velerov2alpha1api.AddToScheme(scheme); err != nil {
	// 	return nil, errors.Errorf("add schema velerov2alpha1api: %v", err)
	// }
	if err = k8scheme.AddToScheme(scheme); err != nil {
		return nil, errors.Errorf("add schema k8scheme: %v", err)
	}
	if err = apiextv1beta1.AddToScheme(scheme); err != nil {
		return nil, errors.Errorf("add schema apiextv1beta1: %v", err)
	}
	if err = apiextv1.AddToScheme(scheme); err != nil {
		return nil, errors.Errorf("add schema apiextv1: %v", err)
	}
	kubebuilderClient, err := kbclient.New(f.config, kbclient.Options{
		Scheme: scheme,
	})

	if err != nil {
		return nil, errors.Errorf("new kubeclient with scheme: %v", err)
	}

	return kubebuilderClient, nil
}

func (f *factory) NewUnstructuredResources() *unstructured.UnstructuredList {
	r := new(unstructured.UnstructuredList)
	r.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "List"})

	return r
}

func (f *factory) VeleroDynamicFactory() veleroclient.DynamicFactory {
	return veleroclient.NewDynamicFactory(f.client)
}

func (f *factory) ClientForGroupVersion(gv schema.GroupVersion, apiResource metav1.APIResource, namespace string) (veleroclient.Dynamic, error) {
	c, err := f.VeleroDynamicFactory().ClientForGroupVersionResource(gv, apiResource, namespace)
	if err != nil {
		return nil, errors.Errorf("new client for GroupVersionResource: %v", err)
	}
	return c, nil
}

func (f *factory) ClientForUnstructured(r *unstructured.Unstructured) (veleroclient.Dynamic, error) {
	gvk := schema.FromAPIVersionAndKind(r.GetAPIVersion(), r.GetKind())

	apiResource := metav1.APIResource{
		Name:       converter.KindResources[r.GetKind()],
		Namespaced: r.GetNamespace() != "",
	}

	c, err := f.ClientForGroupVersion(gvk.GroupVersion(), apiResource, r.GetNamespace())
	if err != nil {
		return nil, err
	}
	return c, nil
}
