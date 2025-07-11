/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
// Code generated by informer-gen. DO NOT EDIT.

package v1

import (
	"context"
	time "time"

	sysbytetradeiov1 "olares.com/backup-server/pkg/apis/sys.bytetrade.io/v1"
	versioned "olares.com/backup-server/pkg/generated/clientset/versioned"
	internalinterfaces "olares.com/backup-server/pkg/generated/informers/externalversions/internalinterfaces"
	v1 "olares.com/backup-server/pkg/generated/listers/sys.bytetrade.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// RestoreInformer provides access to a shared informer and lister for
// Restores.
type RestoreInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1.RestoreLister
}

type restoreInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewRestoreInformer constructs a new informer for Restore type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewRestoreInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredRestoreInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredRestoreInformer constructs a new informer for Restore type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredRestoreInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SysV1().Restores(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SysV1().Restores(namespace).Watch(context.TODO(), options)
			},
		},
		&sysbytetradeiov1.Restore{},
		resyncPeriod,
		indexers,
	)
}

func (f *restoreInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredRestoreInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *restoreInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&sysbytetradeiov1.Restore{}, f.defaultInformer)
}

func (f *restoreInformer) Lister() v1.RestoreLister {
	return v1.NewRestoreLister(f.Informer().GetIndexer())
}
