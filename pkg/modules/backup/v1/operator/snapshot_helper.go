package operator

import (
	"context"
	"fmt"
	"net/http"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/response"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/emicklei/go-restful/v3"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

type ProxyRequest struct {
	Op       string      `json:"op"`
	DataType string      `json:"datatype"`
	Version  string      `json:"version"`
	Group    string      `json:"group"`
	Param    interface{} `json:"param,omitempty"`
	Data     string      `json:"data,omitempty"`
	Token    string
}

type AccountResponse struct {
	response.Header
	Data *AccountResponseData `json:"data,omitempty"`
}

type AccountResponseRawData struct {
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	// ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt int64  `json:"expires_at"`
	UserId    string `json:"userid"`
	Available bool   `json:"available"`
	CreateAt  int64  `json:"create_at"`
}

type AccountResponseData struct {
	Name     string                  `json:"name"`
	Type     string                  `json:"type"`
	RawData  *AccountResponseRawData `json:"raw_data"`
	CloudUrl string                  `json:"cloudUrl"`
}

type AccountValue struct {
	Email   string `json:"email"`
	Userid  string `json:"userid"`
	Token   string `json:"token"`
	Expired any    `json:"expired"`
}

type PasswordResponse struct {
	response.Header
	Data *PasswordResponseData `json:"data,omitempty"`
}

type PasswordResponseData struct {
	Env   string `json:"env"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (o *SnapshotOperator) getPassword(backup *sysv1.Backup) (string, error) {
	settingsUrl := fmt.Sprintf("http://settings-service.user-space-%s/api/backup/password", backup.Spec.Owner)
	client := resty.New().SetTimeout(2 * time.Second).SetDebug(true)

	req := &ProxyRequest{
		Op:       "getAccount",
		DataType: "backupPassword",
		Version:  "v1",
		Group:    "service.settings",
		Data:     backup.Name,
	}

	terminusNonce, err := util.GenTerminusNonce("")
	if err != nil {
		log.Error("generate nonce error, ", err)
		return "", err
	}

	log.Info("fetch password from settings, ", settingsUrl)
	resp, err := client.R().
		SetHeader(restful.HEADER_ContentType, restful.MIME_JSON).
		SetHeader("Terminus-Nonce", terminusNonce).
		SetBody(req).
		SetResult(&PasswordResponse{}).
		Post(settingsUrl)

	if err != nil {
		log.Error("request settings password api error, ", err)
		return "", err
	}

	if resp.StatusCode() != http.StatusOK {
		log.Error("request settings password api response not ok, ", resp.StatusCode())
		err = errors.New(string(resp.Body()))
		return "", err
	}

	pwdResp := resp.Result().(*PasswordResponse)
	log.Infof("settings password api response, %+v", pwdResp)
	if pwdResp.Code != 0 {
		log.Error("request settings password api response error, ", pwdResp.Code, ", ", pwdResp.Message)
		err = errors.New(pwdResp.Message)
		return "", err
	}

	if pwdResp.Data == nil {
		log.Error("request settings password api response data is nil, ", pwdResp.Code, ", ", pwdResp.Message)
		err = errors.New("request settings password api response data is nil")
		return "", err
	}

	return pwdResp.Data.Value, nil
}

func (o *SnapshotOperator) getClusterId() (string, error) {
	dynamicClient, err := o.factory.DynamicClient()
	if err != nil {
		return "", errors.WithStack(err)
	}

	var backoff = wait.Backoff{
		Duration: 2 * time.Second,
		Factor:   2,
		Jitter:   0.1,
		Steps:    5,
	}

	var clusterId string
	if err := retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		unstructuredUser, err := dynamicClient.Resource(constant.TerminusGVR).Get(ctx, "terminus", metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		obj := unstructuredUser.UnstructuredContent()
		clusterId, _, err = unstructured.NestedString(obj, "metadata", "labels", "bytetrade.io/cluster-id")
		if err != nil {
			return errors.WithStack(err)
		}
		if clusterId == "" {
			return errors.WithStack(fmt.Errorf("cluster id not found"))
		}
		return nil
	}); err != nil {
		return "", errors.WithStack(err)
	}

	return clusterId, nil
}

func (o *SnapshotOperator) getOlaresId(owner string) (string, error) {
	dynamicClient, err := o.factory.DynamicClient()
	if err != nil {
		return "", errors.WithStack(err)
	}

	var backoff = wait.Backoff{
		Duration: 2 * time.Second,
		Factor:   2,
		Jitter:   0.1,
		Steps:    5,
	}

	var olaresName string
	if err := retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		unstructuredUser, err := dynamicClient.Resource(constant.UsersGVR).Get(ctx, owner, metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		obj := unstructuredUser.UnstructuredContent()
		olaresName, _, err = unstructured.NestedString(obj, "spec", "email")
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}); err != nil {
		return "", errors.WithStack(err)
	}

	return olaresName, nil
}

func (o *SnapshotOperator) getPodIp(olaresId string) (string, error) {
	kubeClient, err := o.factory.KubeClient()
	if err != nil {
		return "", errors.WithStack(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pods, err := kubeClient.CoreV1().Pods(fmt.Sprintf("user-system-%s", olaresId)).List(ctx, metav1.ListOptions{
		LabelSelector: "app=systemserver",
	})
	if err != nil {
		return "", errors.WithStack(err)
	}

	if pods == nil || pods.Items == nil || len(pods.Items) == 0 {
		return "", fmt.Errorf("system server pod not found")
	}

	pod := pods.Items[0]
	podIp := pod.Status.PodIP
	if podIp == "" {
		return "", fmt.Errorf("system server pod ip invalid")
	}

	return podIp, nil
}

func (o *SnapshotOperator) getAppKey() (string, error) {
	kubeClient, err := o.factory.KubeClient()
	if err != nil {
		return "", errors.WithStack(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	secret, err := kubeClient.CoreV1().Secrets("os-system").Get(ctx, "app-key", metav1.GetOptions{})
	if err != nil {
		return "", errors.WithStack(err)
	}
	if secret == nil || secret.Data == nil || len(secret.Data) == 0 {
		return "", fmt.Errorf("secret not found")
	}

	key, ok := secret.Data["random-key"]
	if !ok {
		return "", fmt.Errorf("app key not found")
	}

	return string(key), nil
}

func (o *SnapshotOperator) getUserToken(owner string, olaresId string) (userid, token string, err error) {
	podIp, err := o.getPodIp(owner)
	if err != nil {
		return "", "", err
	}

	appKey, err := o.getAppKey()
	if err != nil {
		return "", "", err
	}

	terminusNonce, err := util.GenTerminusNonce(appKey)
	if err != nil {
		log.Errorf("generate nonce error: %v", err)
		return
	}

	var settingsUrl = fmt.Sprintf("http://%s/legacy/v1alpha1/service.settings/v1/api/account/retrieve", podIp)

	client := resty.New().SetTimeout(10 * time.Second)
	var data = make(map[string]string)
	data["name"] = fmt.Sprintf("integration-account:space:%s", olaresId)
	log.Infof("fetch account from settings: %s", settingsUrl)
	resp, err := client.R().SetDebug(true).
		SetHeader(restful.HEADER_ContentType, restful.MIME_JSON).
		SetHeader("Terminus-Nonce", terminusNonce).
		SetBody(data).
		SetResult(&AccountResponse{}).
		Post(settingsUrl)

	if err != nil {
		return
	}

	if resp.StatusCode() != http.StatusOK {
		err = errors.WithStack(fmt.Errorf("request settings account api response not ok, status: %d", resp.StatusCode()))
		return
	}

	accountResp := resp.Result().(*AccountResponse)

	if accountResp.Code == 1 && accountResp.Message == "" {
		err = errors.WithStack(fmt.Errorf("olres space is not enabled"))
		return
	} else if accountResp.Code != 0 {
		err = errors.WithStack(fmt.Errorf("request settings account api response error, status: %d, message: %s", accountResp.Code, accountResp.Message))
		return
	}

	if accountResp.Data == nil || accountResp.Data.RawData == nil {
		err = errors.WithStack(fmt.Errorf("request settings account api response data is nil, status: %d, message: %s", accountResp.Code, accountResp.Message))
		return
	}

	if accountResp.Data.RawData.UserId == "" || accountResp.Data.RawData.AccessToken == "" {
		err = errors.WithStack(fmt.Errorf("access token invalid"))
		return
	}

	userid = accountResp.Data.RawData.UserId
	token = accountResp.Data.RawData.AccessToken

	return

	// if accountResp.Code == 1 && accountResp.Message == "" {
	// 	err = errors.WithStack(fmt.Errorf("\nOlares Space is not enabled. Please go to the Settings - Integration page in the LarePass App to add Space\n"))
	// 	return
	// } else if accountResp.Code != 0 {
	// 	err = errors.WithStack(fmt.Errorf("request settings account api response error, status: %d, message: %s", accountResp.Code, accountResp.Message))
	// 	return
	// }

	// if accountResp.Data == nil || accountResp.Data.RawData == nil {
	// 	err = errors.WithStack(fmt.Errorf("request settings account api response data is nil, status: %d, message: %s", accountResp.Code, accountResp.Message))
	// 	return
	// }

	// if accountResp.Data.RawData.UserId == "" || accountResp.Data.RawData.AccessToken == "" {
	// 	err = errors.WithStack(fmt.Errorf("access token invalid"))
	// 	return
	// }

	// userid = accountResp.Data.RawData.UserId
	// token = accountResp.Data.RawData.AccessToken

	// return
}
