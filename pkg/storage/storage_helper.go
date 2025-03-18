package storage

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/apiserver/response"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/emicklei/go-restful/v3"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AccountResponse struct {
	response.Header
	Data *AccountResponseData `json:"data,omitempty"`
}

type AccountResponseRawData struct {
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	ExpiresAt    int64  `json:"expires_at"`
	UserId       string `json:"userid"`
	Available    bool   `json:"available"`
	CreateAt     int64  `json:"create_at"`
}

type AccountResponseData struct {
	Name     string                  `json:"name"`
	Type     string                  `json:"type"`
	RawData  *AccountResponseRawData `json:"raw_data"`
	CloudUrl string                  `json:"cloudUrl"`
}

func (s *storage) GetUserToken() (olaresDid, olaresAccessToken string, expired int64, err error) {
	podIp, err := s.getPodIp()
	if err != nil {
		return
	}

	appKey, err := s.getAppKey()
	if err != nil {
		return
	}

	terminusNonce, err := util.GenTerminusNonce(appKey)
	if err != nil {
		log.Errorf("generate nonce error: %v", err)
		return
	}

	var settingsUrl = fmt.Sprintf("http://%s/legacy/v1alpha1/service.settings/v1/api/account/retrieve", podIp)

	client := resty.New().SetTimeout(10 * time.Second)
	var data = make(map[string]string)
	data["name"] = fmt.Sprintf("integration-account:space:%s", s.olaresId)
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

	olaresDid = accountResp.Data.RawData.UserId
	olaresAccessToken = accountResp.Data.RawData.AccessToken
	expired = accountResp.Data.RawData.ExpiresAt

	return
}

func (s *storage) getPodIp() (string, error) {
	kubeClient, err := s.factory.KubeClient()
	if err != nil {
		return "", errors.WithStack(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pods, err := kubeClient.CoreV1().Pods(fmt.Sprintf("user-system-%s", s.owner)).List(ctx, metav1.ListOptions{
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

func (s *storage) getAppKey() (string, error) {
	kubeClient, err := s.factory.KubeClient()
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
