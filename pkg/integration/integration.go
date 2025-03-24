package integration

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/emicklei/go-restful/v3"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ IntegrationInterface = &Integration{}

type Integration struct {
	Factory  client.Factory
	Owner    string
	Location string
	Name     string
}

func (i *Integration) GetIntegrationSpaceToken() (IntegrationToken, error) {
	data, err := i.query()
	if err != nil {
		return nil, nil
	}

	return i.withSpaceToken(data), nil
}

func (i *Integration) GetIntegrationCloudToken() (IntegrationToken, error) {
	data, err := i.query()
	if err != nil {
		return nil, nil
	}

	return i.withSpaceToken(data), nil
}

func (i *Integration) GetIntegrationToken() (IntegrationToken, error) {
	data, err := i.query()
	if err != nil {
		return nil, nil
	}

	switch i.Location {
	case constant.BackupLocationSpace.String():
		return i.withSpaceToken(data), nil
	}
	return i.withCloudToken(data), nil // TODO filesystem
}

func (i *Integration) withSpaceToken(data *accountResponseData) IntegrationToken {
	return &IntegrationSpace{
		Name:        data.Name,
		Type:        data.Type,
		OlaresDid:   data.RawData.UserId,
		AccessToken: data.RawData.AccessToken,
		ExpiresAt:   data.RawData.ExpiresAt,
		Available:   data.RawData.Available,
		Location:    i.Location,
	}
}

func (i *Integration) withCloudToken(data *accountResponseData) IntegrationToken {
	return &IntegrationCloud{
		Name:      data.Name,
		Type:      data.Type,
		AccessKey: data.Name,
		SecretKey: data.RawData.AccessToken,
		Endpoint:  data.RawData.Endpoint,
		Bucket:    data.RawData.Bucket,
		Available: data.RawData.Available,
		Location:  i.Location,
	}
}

func (i *Integration) query() (*accountResponseData, error) {
	ip, err := i.getSettingsIP()
	if err != nil {
		return nil, errors.WithStack(fmt.Errorf("get settings service ip error: %v", err))
	}

	headerNonce, err := i.getAppKey()
	if err != nil {
		return nil, errors.WithStack(fmt.Errorf("get header nonce error: %v", err))
	}

	var settingsUrl = fmt.Sprintf("http://%s/legacy/v1alpha1/service.settings/v1/api/account/retrieve", ip)

	client := resty.New().SetTimeout(10 * time.Second)
	var data = make(map[string]string)
	data["name"] = i.formatUrl(i.Location, i.Name)
	log.Infof("fetch integration from settings: %s", settingsUrl)
	resp, err := client.R().SetDebug(true).
		SetHeader(restful.HEADER_ContentType, restful.MIME_JSON).
		SetHeader("Terminus-Nonce", headerNonce).
		SetBody(data).
		SetResult(&accountResponse{}).
		Post(settingsUrl)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		err = errors.WithStack(fmt.Errorf("request account api response not ok, status: %d", resp.StatusCode()))
		return nil, err
	}

	accountResp := resp.Result().(*accountResponse)

	if accountResp.Code == 1 && accountResp.Message == "" {
		err = errors.WithStack(fmt.Errorf("olres space is not enabled"))
		return nil, err
	} else if accountResp.Code != 0 {
		err = errors.WithStack(fmt.Errorf("request account api response error, status: %d, message: %s", accountResp.Code, accountResp.Message))
		return nil, err
	}

	if accountResp.Data == nil || accountResp.Data.RawData == nil {
		err = errors.WithStack(fmt.Errorf("request account api response data is nil, status: %d, message: %s", accountResp.Code, accountResp.Message))
		return nil, err
	}

	return accountResp.Data, nil
}

func (i *Integration) getSettingsIP() (ip string, err error) {
	kubeClient, err := i.Factory.KubeClient()
	if err != nil {
		return "", errors.WithStack(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pods, err := kubeClient.CoreV1().Pods(fmt.Sprintf("user-system-%s", i.Owner)).List(ctx, metav1.ListOptions{
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

func (i *Integration) getAppKey() (string, error) {
	kubeClient, err := i.Factory.KubeClient()
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

func (i *Integration) formatUrl(location, name string) string {
	var l string
	switch location {
	case "space":
		l = "space"
	case "aws":
		l = "awss3"
	case "tencentcloud":
		l = "tencentcloud" // TODO debug
	}
	return fmt.Sprintf("integration-account:%s:%s", l, name)
}
