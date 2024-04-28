

// Code generated by lister-gen. DO NOT EDIT.

package v1

import (
	v1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// BackupLister helps list Backups.
// All objects returned here must be treated as read-only.
type BackupLister interface {
	// List lists all Backups in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.Backup, err error)
	// Backups returns an object that can list and get Backups.
	Backups(namespace string) BackupNamespaceLister
	BackupListerExpansion
}

// backupLister implements the BackupLister interface.
type backupLister struct {
	indexer cache.Indexer
}

// NewBackupLister returns a new BackupLister.
func NewBackupLister(indexer cache.Indexer) BackupLister {
	return &backupLister{indexer: indexer}
}

// List lists all Backups in the indexer.
func (s *backupLister) List(selector labels.Selector) (ret []*v1.Backup, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.Backup))
	})
	return ret, err
}

// Backups returns an object that can list and get Backups.
func (s *backupLister) Backups(namespace string) BackupNamespaceLister {
	return backupNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// BackupNamespaceLister helps list and get Backups.
// All objects returned here must be treated as read-only.
type BackupNamespaceLister interface {
	// List lists all Backups in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.Backup, err error)
	// Get retrieves the Backup from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1.Backup, error)
	BackupNamespaceListerExpansion
}

// backupNamespaceLister implements the BackupNamespaceLister
// interface.
type backupNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Backups in the indexer for a given namespace.
func (s backupNamespaceLister) List(selector labels.Selector) (ret []*v1.Backup, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.Backup))
	})
	return ret, err
}

// Get retrieves the Backup from the indexer for a given namespace and name.
func (s backupNamespaceLister) Get(name string) (*v1.Backup, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("backup"), name)
	}
	return obj.(*v1.Backup), nil
}
