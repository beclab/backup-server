package client

import (
	"fmt"
	"sync"

	"bytetrade.io/web3os/backup-server/pkg/converter"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var once sync.Once

var clientFactory Factory

func ClientFactory() Factory {
	return clientFactory
}

func Init(logLevel string) (err error) {
	log.Debug("new dynamic client")

	f, err := NewFactory()
	if err != nil {
		return err
	}
	once.Do(func() {
		clientFactory = f
	})

	return getNamespaces(logLevel, f)
}

func getNamespaces(logLevel string, f Factory) error {
	gvk := schema.FromAPIVersionAndKind(corev1.SchemeGroupVersion.String(), "Namespace")
	apiResource := metav1.APIResource{
		Name:       "namespaces",
		Namespaced: false,
	}

	nsClient, err := f.ClientForGroupVersion(gvk.GroupVersion(), apiResource, "")
	if err != nil {
		return errors.WithMessage(err, "new namespace client")
	}
	unstructuredNamespaceList, err := nsClient.List(metav1.ListOptions{})
	if err != nil {
		return errors.WithMessage(err, "list namespaces")
	}

	namespaceList := &corev1.NamespaceList{}
	err = converter.UnstructuredListTo(unstructuredNamespaceList, namespaceList)
	if err != nil {
		return errors.WithMessage(err, "decode unstructured namespace")
	}

	log.Debug("get namespaces for testing dynamic client:")

	if logLevel == "debug" {
		for _, ns := range namespaceList.Items {
			fmt.Printf("%s\n", ns.ObjectMeta.Name)
		}
	}
	return nil
}
