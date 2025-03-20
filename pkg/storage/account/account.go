package account

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/storage/models"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/emicklei/go-restful/v3"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Account struct {
	factory  client.Factory
	location string
	ip       string
	nounce   string
	owner    string
}

func (a *Account) GetStorage() {
	var url = fmt.Sprintf("http://%s/legacy/v1alpha1/service.settings/v1/api/account/retrieve", a.ip)

	var parmf = "integration-account:%s:%s" // awss3:xxx space:xxx tencent:xxx
	var parm string

	switch a.location {
	case constant.BackupLocationSpace.String():
		parm = fmt.Sprintf(parmf, "space", "")
	case constant.BackupLocationAws.String():
		parm = fmt.Sprintf(parmf, "awss3", "")
	case constant.BackupLocationTencentCloud.String():
		parm = fmt.Sprintf(parmf, "tencent", "")
	}

	if parm == "" {
		// todo error
		return
	}

	client := resty.New().SetTimeout(10 * time.Second)
	var data = make(map[string]string)
	data["name"] = parm
	log.Infof("fetch account from settings: %s", url)
	resp, err := client.R().SetDebug(true).
		SetHeader(restful.HEADER_ContentType, restful.MIME_JSON).
		SetHeader("Terminus-Nonce", a.nounce).
		SetBody(data).
		SetResult(&models.AccountResponse{}).
		Post(url)

	if err != nil {
		return
	}

	if resp.StatusCode() != http.StatusOK {
		err = errors.WithStack(fmt.Errorf("request account response not ok, status: %d", resp.StatusCode()))
		return
	}

	accountResp := resp.Result().(*models.AccountResponse)

	if accountResp.Code == 1 && accountResp.Message == "" {
		err = errors.WithStack(fmt.Errorf("account invalid"))
		return
	} else if accountResp.Code != 0 {
		err = errors.WithStack(fmt.Errorf("request account api response error, status: %d, message: %s", accountResp.Code, accountResp.Message))
		return
	}

	if accountResp.Data == nil || accountResp.Data.RawData == nil {
		err = errors.WithStack(fmt.Errorf("request account api response data is nil, status: %d, message: %s", accountResp.Code, accountResp.Message))
		return
	}

	if accountResp.Data.RawData.UserId == "" || accountResp.Data.RawData.AccessToken == "" {
		err = errors.WithStack(fmt.Errorf("access token invalid"))
		return
	}

	// olaresDid = accountResp.Data.RawData.UserId
	// olaresAccessToken = accountResp.Data.RawData.AccessToken
	// expired = accountResp.Data.RawData.ExpiresAt

}

func (a *Account) getPodIp() (string, error) {
	kubeClient, err := a.factory.KubeClient()
	if err != nil {
		return "", errors.WithStack(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pods, err := kubeClient.CoreV1().Pods(fmt.Sprintf("user-system-%s", a.owner)).List(ctx, metav1.ListOptions{
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

func (a *Account) getAppKey() (string, error) {
	kubeClient, err := a.factory.KubeClient()
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
