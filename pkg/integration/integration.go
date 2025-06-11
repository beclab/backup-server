package integration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/emicklei/go-restful/v3"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"olares.com/backup-server/pkg/client"
	"olares.com/backup-server/pkg/constant"
	"olares.com/backup-server/pkg/util"
	"olares.com/backup-server/pkg/util/log"
	"olares.com/backup-server/pkg/util/repo"
)

var IntegrationService *Integration

type Integration struct {
	Factory      client.Factory
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

func (i *Integration) GetIntegrationSpaceToken(ctx context.Context, owner string, integrationName string) (*SpaceToken, error) {
	token := i.OlaresTokens[integrationName]
	if token != nil {
		if !token.Expired() {
			return token, nil
		}
	}

	data, err := i.query(ctx, owner, constant.BackupLocationSpace.String(), integrationName)
	if err != nil {
		return nil, err
	}

	token = i.withSpaceToken(data)
	i.OlaresTokens[integrationName] = token

	if token.Expired() {
		log.Errorf("olares space token expired, expiresAt: %d, format: %s", token.ExpiresAt, util.ParseUnixMilliToDate(token.ExpiresAt))
		return nil, errors.New("Access token expired. Please re-connect to your Olares Space in LarePass.")
	}

	return token, nil
}

func (i *Integration) GetIntegrationCloudToken(ctx context.Context, owner, location, integrationName string) (*IntegrationToken, error) {
	data, err := i.query(ctx, owner, location, integrationName)
	if err != nil {
		return nil, err
	}
	return i.withCloudToken(data), nil
}

// todo
func (i *Integration) GetIntegrationCloudAccount(ctx context.Context, owner, location, endpoint string) (*IntegrationToken, error) {
	accounts, err := i.queryIntegrationAccounts(ctx, owner)
	if err != nil {
		return nil, err
	}

	var token *IntegrationToken
	for _, account := range accounts {
		if account.Type == "space" {
			continue
		}
		if strings.Contains(location, account.Type) {
			token, err = i.GetIntegrationCloudToken(ctx, owner, location, account.Name)
			if err != nil {
				return nil, err
			}
			if token != nil {
				if location == constant.BackupLocationAwsS3.String() {
					tokenRepoInfo, err := repo.FormatS3(token.Endpoint)
					if err != nil {
						return nil, err
					}
					urlRepoInfo, err := repo.FormatS3(endpoint)
					if err != nil {
						return nil, err
					}
					if tokenRepoInfo.Endpoint == urlRepoInfo.Endpoint {
						break
					}
				} else if location == constant.BackupLocationTencentCloud.String() {
					tokenRepoInfo, err := repo.FormatCosByRawUrl(token.Endpoint)
					if err != nil {
						return nil, err
					}
					urlRepoInfo, err := repo.FormatCosByRawUrl(endpoint)
					if err != nil {
						return nil, err
					}
					if tokenRepoInfo.Endpoint == urlRepoInfo.Endpoint {
						break
					}
				}
			}
		}
	}

	if token == nil {
		return nil, fmt.Errorf("integration token not found")
	}

	return token, nil
}

func (i *Integration) GetIntegrationAccountsByLocation(ctx context.Context, owner, location string) ([]string, error) {

	accounts, err := i.queryIntegrationAccounts(ctx, owner) // GetIntegrationAccountsByLocation
	if err != nil {
		return nil, err
	}

	var result []string
	for _, account := range accounts {
		if account.Type == "space" {
			continue
		}
		if strings.Contains(location, account.Type) {
			result = append(result, account.Name)
		}
	}

	return result, nil
}

func (i *Integration) ValidIntegrationNameByLocationName(ctx context.Context, owner string, location string, locationConfigName string) (string, error) {
	accounts, err := i.queryIntegrationAccounts(ctx, owner)
	if err != nil {
		return "", err
	}

	var name string

	for _, account := range accounts {
		if util.ListContains([]string{constant.BackupLocationSpace.String(), constant.BackupLocationFileSystem.String()}, location) {
			if account.Type == "space" && (account.Name == locationConfigName || strings.Contains(account.Name, locationConfigName)) {
				name = account.Name
				break
			}
		}
		if location == constant.BackupLocationAwsS3.String() {
			if account.Type == "awss3" && account.Name == locationConfigName {
				name = account.Name
				break
			}
		} else if location == constant.BackupLocationTencentCloud.String() {
			if account.Type == "tencent" && account.Name == locationConfigName {
				name = account.Name
				break
			}
		}

	}

	if name == "" {
		return "", fmt.Errorf("integration account not found, owner: %s, location: %s", owner, location)
	}

	return name, nil
}

func (i *Integration) GetIntegrationNameByLocation(ctx context.Context, owner, location, bucket, region, prefix string) (string, error) {
	if location == constant.BackupLocationFileSystem.String() {
		return "", nil
	}

	accounts, err := i.queryIntegrationAccounts(ctx, owner)
	if err != nil {
		return "", err
	}

	var name string

	if location == constant.BackupLocationFileSystem.String() {
		for _, account := range accounts {
			if account.Type == "space" {
				name = account.Name
				break
			}
		}
		return name, nil
	}

	for _, account := range accounts {
		// account.Type includes: space, awss3, tencent
		// location includes: space, awss3, tencentcloud, filesystem
		if strings.Contains(location, account.Type) {
			if account.Type == "space" {
				name = account.Name
				break
			} else if account.Type == "awss3" {
				token, err := i.GetIntegrationCloudToken(ctx, owner, location, account.Name)
				if err != nil {
					return "", err
				}
				tokenInfo, err := repo.FormatS3(token.Endpoint)
				if err != nil {
					return "", err
				}
				if tokenInfo.Bucket == bucket && tokenInfo.Region == region && tokenInfo.Prefix == prefix {
					name = account.Name
					break
				}
			} else if account.Type == "tencent" {
				token, err := i.GetIntegrationCloudToken(ctx, owner, location, account.Name)
				if err != nil {
					return "", err
				}
				tokenInfo, err := repo.FormatCosByRawUrl(token.Endpoint)
				if err != nil {
					return "", err
				}
				if tokenInfo.Bucket == bucket && tokenInfo.Region == region && tokenInfo.Prefix == strings.TrimRight(prefix, "/") {
					name = account.Name
					break
				}
			}
		}

	}

	if name == "" {
		return "", fmt.Errorf("integration account not found, owner: %s, location: %s", owner, location)
	}

	return name, nil
}

func (i *Integration) withSpaceToken(data *accountResponseData) *SpaceToken {
	return &SpaceToken{
		Name:        data.Name,
		Type:        data.Type,
		OlaresDid:   data.RawData.UserId,
		AccessToken: data.RawData.AccessToken,
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

func (i *Integration) queryIntegrationAccounts(ctx context.Context, owner string) ([]*accountsResponseData, error) {
	ip, err := i.getSettingsIP(ctx, owner)
	if err != nil {
		return nil, errors.WithStack(fmt.Errorf("get settings service ip error: %v", err))
	}

	headerNonce, err := i.getAppKey(ctx)
	if err != nil {
		return nil, errors.WithStack(fmt.Errorf("get header nonce error: %v", err))
	}

	var settingsUrl = fmt.Sprintf("http://%s/legacy/v1alpha1/service.settings/v1/api/account/all", ip)

	client := resty.New().SetTimeout(10 * time.Second)
	log.Infof("fetch integration from settings: %s", settingsUrl)
	resp, err := client.R().SetDebug(true).SetContext(ctx).
		SetHeader(constant.BackendTokenHeader, headerNonce).
		SetResult(&accountsResponse{}).
		Get(settingsUrl)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		err = errors.WithStack(fmt.Errorf("request account api response not ok, status: %d", resp.StatusCode()))
		return nil, err
	}

	accountsResp := resp.Result().(*accountsResponse)

	if accountsResp.Code == 1 && accountsResp.Message == "" {
		err = errors.WithStack(fmt.Errorf("integration accounts not exists"))
		return nil, err
	} else if accountsResp.Code != 0 {
		err = errors.WithStack(fmt.Errorf("get integration accounts error, status: %d, message: %s", accountsResp.Code, accountsResp.Message))
		return nil, err
	}

	if accountsResp.Data == nil || len(accountsResp.Data) == 0 {
		err = errors.WithStack(fmt.Errorf("integration accounts not exists"))
		return nil, err
	}

	return accountsResp.Data, nil
}

func (i *Integration) query(ctx context.Context, owner, integrationLocation, integrationName string) (*accountResponseData, error) {
	ip, err := i.getSettingsIP(ctx, owner)
	if err != nil {
		return nil, errors.WithStack(fmt.Errorf("get settings service ip error: %v, location: %s, name: %s", err, integrationLocation, integrationName))
	}

	headerNonce, err := i.getAppKey(ctx)
	if err != nil {
		return nil, errors.WithStack(fmt.Errorf("get header nonce error: %v", err))
	}

	var settingsUrl = fmt.Sprintf("http://%s/legacy/v1alpha1/service.settings/v1/api/account/retrieve", ip)

	client := resty.New().SetTimeout(10 * time.Second)
	var data = make(map[string]string)
	data["name"] = i.formatUrl(integrationLocation, integrationName)
	log.Infof("fetch integration from settings: %s", settingsUrl)
	resp, err := client.R().SetDebug(true).SetContext(ctx).
		SetHeader(restful.HEADER_ContentType, restful.MIME_JSON).
		SetHeader(constant.BackendTokenHeader, headerNonce).
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
		err = errors.WithStack(fmt.Errorf("olares space is not enabled"))
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

func (i *Integration) getOlaresId(ctx context.Context, owner string) (string, error) {
	dynamicClient, err := i.Factory.DynamicClient()
	if err != nil {
		return "", errors.WithStack(fmt.Errorf("get dynamic client error: %v", err))
	}

	getCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	unstructuredUser, err := dynamicClient.Resource(constant.UsersGVR).Get(getCtx, owner, metav1.GetOptions{})
	if err != nil {
		return "", errors.WithStack(fmt.Errorf("get user error: %v", err))
	}
	obj := unstructuredUser.UnstructuredContent()
	olaresId, _, err := unstructured.NestedString(obj, "spec", "email")
	if err != nil {
		return "", errors.WithStack(fmt.Errorf("get user nested string error: %v", err))
	}
	return olaresId, nil
}

func (i *Integration) getSettingsIP(ctx context.Context, onwer string) (ip string, err error) {
	kubeClient, err := i.Factory.KubeClient()
	if err != nil {
		return "", errors.WithStack(err)
	}

	getCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	pods, err := kubeClient.CoreV1().Pods(fmt.Sprintf("user-system-%s", onwer)).List(getCtx, metav1.ListOptions{
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
	case "awss3":
		l = "awss3"
	case "tencentcloud":
		l = "tencent"
	}
	return fmt.Sprintf("integration-account:%s:%s", l, name)
}
