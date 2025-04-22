package watchers

import (
	"context"

	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var UserSchemeGroupVersionResource = schema.GroupVersionResource{Group: "iam.kubesphere.io", Version: "v1alpha2", Resource: "users"}

type EventPayload struct {
	Type string      `json:"eventType"`
	Data interface{} `json:"eventData,omitempty"`
}

type Notification struct {
	Factory client.Factory
}

func (n *Notification) Send(ctx context.Context, eventType, user, msg string, data interface{}) error {
	appKey, appSecret, err := n.getUserAppKey(ctx, user)
	if err != nil {
		return err
	}

	client := NewEventClient(appKey, appSecret, "system-server.user-system-"+user)

	return client.CreateEvent(eventType, msg, data)
}

func (n *Notification) getUserAppKey(ctx context.Context, user string) (appKey, appSecret string, err error) {
	dynamicClient, err := n.Factory.DynamicClient()
	if err != nil {
		return
	}
	namespace := "user-system-" + user
	data, err := dynamicClient.Resource(AppPermGVR).Namespace(namespace).Get(ctx, "bfl", metav1.GetOptions{})
	if err != nil {
		log.Errorf("get bfl application permission error: %v", err)
		return
	}

	var appPerm ApplicationPermission

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(data.Object, &appPerm)
	if err != nil {
		log.Errorf("convert bfl application permission error: %v ", err)
		return
	}

	appKey = appPerm.Spec.Key
	appSecret = appPerm.Spec.Secret

	return
}
