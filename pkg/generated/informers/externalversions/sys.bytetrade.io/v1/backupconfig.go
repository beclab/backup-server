

// Code generated by informer-gen. DO NOT EDIT.

package v1

import (
	"context"
	time "time"

	sysbytetradeiov1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	versioned "bytetrade.io/web3os/backup-server/pkg/generated/clientset/versioned"
	internalinterfaces "bytetrade.io/web3os/backup-server/pkg/generated/informers/externalversions/internalinterfaces"
	v1 "bytetrade.io/web3os/backup-server/pkg/generated/listers/sys.bytetrade.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// BackupConfigInformer provides access to a shared informer and lister for
// BackupConfigs.
type BackupConfigInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1.BackupConfigLister
}

type backupConfigInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewBackupConfigInformer constructs a new informer for BackupConfig type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewBackupConfigInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredBackupConfigInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredBackupConfigInformer constructs a new informer for BackupConfig type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredBackupConfigInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SysV1().BackupConfigs(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SysV1().BackupConfigs(namespace).Watch(context.TODO(), options)
			},
		},
		&sysbytetradeiov1.BackupConfig{},
		resyncPeriod,
		indexers,
	)
}

func (f *backupConfigInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredBackupConfigInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *backupConfigInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&sysbytetradeiov1.BackupConfig{}, f.defaultInformer)
}

func (f *backupConfigInformer) Lister() v1.BackupConfigLister {
	return v1.NewBackupConfigLister(f.Informer().GetIndexer())
}
