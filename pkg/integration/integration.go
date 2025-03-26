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

var IntegrationService *Integration

type Integration struct {
	Factory      client.Factory
	Owner        string
	Location     string
	Name         string
	OlaresTokens map[string]*SpaceToken
}

func NewIntegrationManager(factory client.Factory) {
	IntegrationService = &Integration{
		Factory:      factory,
		OlaresTokens: make(map[string]*SpaceToken),
	}
}

func IntegrationManager() *Integration {
	return IntegrationService
}

func (i *Integration) GetIntegrationSpaceToken(ctx context.Context, olaresId string) (*SpaceToken, error) {
	token := i.OlaresTokens[olaresId]
	if token != nil {
		if !token.Expired() {
			return token, nil
		}
	}

	data, err := i.query(ctx, constant.BackupLocationSpace.String(), olaresId)
	if err != nil {
		return nil, err
	}

	token = i.withSpaceToken(data)
	i.OlaresTokens[olaresId] = token

	if token.Expired() {
		return nil, fmt.Errorf("olares space token expired")
	}

	return token, nil
}

func (i *Integration) GetIntegrationCloudToken(ctx context.Context, location, integrationName string) (*IntegrationToken, error) {
	data, err := i.query(ctx, location, integrationName)
	if err != nil {
		return nil, err
	}

	return i.withCloudToken(data), nil
}

func (i *Integration) withSpaceToken(data *accountResponseData) *SpaceToken {
	return &SpaceToken{
		Name:        data.Name,
		Type:        data.Type,
		OlaresDid:   "did:key:z6MkiwBrVUoVizE94HcMxxqXE47s4SswMyQkJzdMtUBJ4PfJ", // data.RawData.UserId,
		AccessToken: "d1b40d78955348458f94213a9a8900f1",                         //data.RawData.AccessToken,
		ExpiresAt:   data.RawData.ExpiresAt,
		Available:   data.RawData.Available,
	}
}

func (i *Integration) withCloudToken(data *accountResponseData) *IntegrationToken {
	return &IntegrationToken{
		Name:      data.Name,
		Type:      data.Type,
		AccessKey: data.Name,
		SecretKey: data.RawData.AccessToken,
		Endpoint:  data.RawData.Endpoint,
		Bucket:    data.RawData.Bucket,
		Available: data.RawData.Available,
	}
}

func (i *Integration) query(ctx context.Context, integrationLocation, integrationAccountName string) (*accountResponseData, error) {
	ip, err := i.getSettingsIP(ctx)
	if err != nil {
		return nil, errors.WithStack(fmt.Errorf("get settings service ip error: %v", err))
	}

	headerNonce, err := i.getAppKey(ctx)
	if err != nil {
		return nil, errors.WithStack(fmt.Errorf("get header nonce error: %v", err))
	}

	var settingsUrl = fmt.Sprintf("http://%s/legacy/v1alpha1/service.settings/v1/api/account/retrieve", ip)

	client := resty.New().SetTimeout(10 * time.Second)
	var data = make(map[string]string)
	data["name"] = i.formatUrl(integrationLocation, integrationAccountName)
	log.Infof("fetch integration from settings: %s", settingsUrl)
	resp, err := client.R().SetDebug(true).SetContext(ctx).
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

	accountResp.Data.RawData.ExpiresAt = 1742978678000 // TODO debug
	return accountResp.Data, nil
}

func (i *Integration) getSettingsIP(ctx context.Context) (ip string, err error) {
	kubeClient, err := i.Factory.KubeClient()
	if err != nil {
		return "", errors.WithStack(err)
	}

	getCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	pods, err := kubeClient.CoreV1().Pods(fmt.Sprintf("user-system-%s", i.Owner)).List(getCtx, metav1.ListOptions{
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

func (i *Integration) getAppKey(ctx context.Context) (string, error) {
	kubeClient, err := i.Factory.KubeClient()
	if err != nil {
		return "", errors.WithStack(err)
	}

	getCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	secret, err := kubeClient.CoreV1().Secrets("os-system").Get(getCtx, "app-key", metav1.GetOptions{})
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
