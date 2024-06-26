

// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	"context"
	"time"

	v1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	scheme "bytetrade.io/web3os/backup-server/pkg/generated/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// BackupConfigsGetter has a method to return a BackupConfigInterface.
// A group's client should implement this interface.
type BackupConfigsGetter interface {
	BackupConfigs(namespace string) BackupConfigInterface
}

// BackupConfigInterface has methods to work with BackupConfig resources.
type BackupConfigInterface interface {
	Create(ctx context.Context, backupConfig *v1.BackupConfig, opts metav1.CreateOptions) (*v1.BackupConfig, error)
	Update(ctx context.Context, backupConfig *v1.BackupConfig, opts metav1.UpdateOptions) (*v1.BackupConfig, error)
	UpdateStatus(ctx context.Context, backupConfig *v1.BackupConfig, opts metav1.UpdateOptions) (*v1.BackupConfig, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.BackupConfig, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.BackupConfigList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.BackupConfig, err error)
	BackupConfigExpansion
}

// backupConfigs implements BackupConfigInterface
type backupConfigs struct {
	client rest.Interface
	ns     string
}

// newBackupConfigs returns a BackupConfigs
func newBackupConfigs(c *SysV1Client, namespace string) *backupConfigs {
	return &backupConfigs{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the backupConfig, and returns the corresponding backupConfig object, and an error if there is any.
func (c *backupConfigs) Get(ctx context.Context, name string, options metav1.GetOptions) (result *v1.BackupConfig, err error) {
	result = &v1.BackupConfig{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("backupconfigs").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of BackupConfigs that match those selectors.
func (c *backupConfigs) List(ctx context.Context, opts metav1.ListOptions) (result *v1.BackupConfigList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1.BackupConfigList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("backupconfigs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested backupConfigs.
func (c *backupConfigs) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("backupconfigs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a backupConfig and creates it.  Returns the server's representation of the backupConfig, and an error, if there is any.
func (c *backupConfigs) Create(ctx context.Context, backupConfig *v1.BackupConfig, opts metav1.CreateOptions) (result *v1.BackupConfig, err error) {
	result = &v1.BackupConfig{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("backupconfigs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(backupConfig).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a backupConfig and updates it. Returns the server's representation of the backupConfig, and an error, if there is any.
func (c *backupConfigs) Update(ctx context.Context, backupConfig *v1.BackupConfig, opts metav1.UpdateOptions) (result *v1.BackupConfig, err error) {
	result = &v1.BackupConfig{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("backupconfigs").
		Name(backupConfig.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(backupConfig).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *backupConfigs) UpdateStatus(ctx context.Context, backupConfig *v1.BackupConfig, opts metav1.UpdateOptions) (result *v1.BackupConfig, err error) {
	result = &v1.BackupConfig{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("backupconfigs").
		Name(backupConfig.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(backupConfig).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the backupConfig and deletes it. Returns an error if one occurs.
func (c *backupConfigs) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("backupconfigs").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *backupConfigs) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("backupconfigs").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched backupConfig.
func (c *backupConfigs) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.BackupConfig, err error) {
	result = &v1.BackupConfig{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("backupconfigs").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
