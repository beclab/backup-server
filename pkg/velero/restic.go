package velero

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/response"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"github.com/emicklei/go-restful/v3"
	"github.com/go-resty/resty/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

type Repo struct {
	Name         string
	Prefix       string
	Repository   string
	S3Repository string
	Key          string
}

func SetResticEnv(ctx context.Context, client dynamic.Interface, bc *sysv1.BackupConfigSpec) error {

	// get cloud user id and token from settings
	userid, token, err := getAccountFromSettings(bc)
	if err != nil {
		klog.Error("fetch userid and token from settings error, ", err)
		return err
	}

	klog.Info("get user and token, ", userid, ", ", token)

	// get aws sts session token from cloud
	awsAccount, err := GetAwsAccountFromCloud(ctx, client, userid, token, DefaultBackupBucket, DefaultBackupKeyPrefix)
	if err != nil {
		return err
	}

	if awsAccount.Cloud != "AWS" {
		return fmt.Errorf("unsupported cloud platform: %s", awsAccount.Cloud)
	}

	os.Setenv("AWS_ACCESS_KEY_ID", awsAccount.Key)
	os.Setenv("AWS_SECRET_ACCESS_KEY", awsAccount.Secret)
	os.Setenv("AWS_SESSION_TOKEN", awsAccount.Token)

	bc.Bucket = awsAccount.Bucket
	bc.Prefix = awsAccount.Prefix
	bc.Region = awsAccount.Region
	bc.Provider = awsAccount.Cloud

	return nil
}

// FIXME: backup uid will be changed if cluster has been restored
func GetRepository(name, uid string, bc *sysv1.BackupConfig) *Repo {
	repoName := fmt.Sprintf("%s_%s", name, uid)
	repoPrefix := filepath.Join(bc.Spec.Prefix, "restic", repoName)

	var s3Domain string

	if bc.Spec.Provider == AWS {
		s3Domain = fmt.Sprintf("s3.%s.amazonaws.com", bc.Spec.Region)
	}

	repository := filepath.Join(s3Domain, bc.Spec.Bucket, repoPrefix)

	return &Repo{
		Name:         repoName,
		Prefix:       repoPrefix,
		Repository:   repository,
		S3Repository: fmt.Sprintf("s3:%s", repository),
		Key:          filepath.Join(repoPrefix, "config"),
	}
}

func getAccountFromSettings(bc *sysv1.BackupConfigSpec) (userid, token string, err error) {
	settingsUrl := fmt.Sprintf("http://settings-service.user-space-%s/api/account", bc.Owner)
	client := resty.New().SetTimeout(2 * time.Second)

	req := &ProxyRequest{
		Op:       "getAccount",
		DataType: "account",
		Version:  "v1",
		Group:    "service.settings",
		Data:     "settings-account-space",
	}

	terminusNonce, err := util.GenTerminusNonce("")
	if err != nil {
		klog.Error("generate nonce error, ", err)
		return
	}

	klog.Info("fetch account from settings, ", settingsUrl)
	resp, err := client.R().SetDebug(true).
		SetHeader(restful.HEADER_ContentType, restful.MIME_JSON).
		SetHeader("Terminus-Nonce", terminusNonce).
		SetBody(req).
		SetResult(&AccountResponse{}).
		Post(settingsUrl)

	if err != nil {
		klog.Error("request settings account api error, ", err)
		return
	}

	if resp.StatusCode() != http.StatusOK {
		klog.Error("request settings account api response not ok, ", resp.StatusCode())
		err = fmt.Errorf("settings account response, %d, %s", resp.StatusCode(), string(resp.Body()))
		return
	}

	accountResp := resp.Result().(*AccountResponse)
	klog.Infof("settings account api response, %+v", accountResp)
	if accountResp.Code != 0 {
		klog.Error("request settings account api response error, ", accountResp.Code, ", ", accountResp.Message)
		err = errors.New(accountResp.Message)
		return
	}

	if accountResp.Data == nil {
		klog.Error("request settings account api response data is nil, ", accountResp.Code, ", ", accountResp.Message)
		err = errors.New("request settings account api response data is nil")
		return
	}

	var value AccountValue
	err = json.Unmarshal([]byte(accountResp.Data.Value), &value)
	if err != nil {
		klog.Error("parse value error, ", err, ", ", value)
		return
	}
	userid = value.Userid
	token = value.Token

	return
}

func getPasswordFromSettings(bc *sysv1.BackupConfig) (password string, err error) {
	settingsUrl := fmt.Sprintf("http://settings-service.user-space-%s/api/backup/password", bc.Spec.Owner)
	client := resty.New().SetTimeout(2 * time.Second).SetDebug(true)

	req := &ProxyRequest{
		Op:       "getAccount",
		DataType: "backupPassword",
		Version:  "v1",
		Group:    "service.settings",
		Data:     bc.Name,
	}

	terminusNonce, err := util.GenTerminusNonce("")
	if err != nil {
		klog.Error("generate nonce error, ", err)
		return
	}

	klog.Info("fetch password from settings, ", settingsUrl)
	resp, err := client.R().
		SetHeader(restful.HEADER_ContentType, restful.MIME_JSON).
		SetHeader("Terminus-Nonce", terminusNonce).
		SetBody(req).
		SetResult(&PasswordResponse{}).
		Post(settingsUrl)

	if err != nil {
		klog.Error("request settings password api error, ", err)
		return
	}

	if resp.StatusCode() != http.StatusOK {
		klog.Error("request settings password api response not ok, ", resp.StatusCode())
		err = errors.New(string(resp.Body()))
		return
	}

	pwdResp := resp.Result().(*PasswordResponse)
	klog.Infof("settings password api response, %+v", pwdResp)
	if pwdResp.Code != 0 {
		klog.Error("request settings password api response error, ", pwdResp.Code, ", ", pwdResp.Message)
		err = errors.New(pwdResp.Message)
		return
	}

	if pwdResp.Data == nil {
		klog.Error("request settings password api response data is nil, ", pwdResp.Code, ", ", pwdResp.Message)
		err = errors.New("request settings password api response data is nil")
		return
	}

	password = pwdResp.Data.Value
	return
}

func GetAwsAccountFromCloud(ctx context.Context, client dynamic.Interface, userid, token, bucket, prefix string) (*AWSAccount, error) {
	serverDomain := util.EnvOrDefault(constant.EnvSpaceUrl, constant.DefaultSyncServerURL)
	serverURL := fmt.Sprintf("%s/v1/resource/stsToken", strings.TrimRight(serverDomain, "/"))

	clusterId, err := getClusterId(ctx, client)
	if err != nil {
		return nil, err
	}

	httpClient := resty.New().SetTimeout(15 * time.Second).SetDebug(true)
	duration := 12 * time.Hour
	resp, err := httpClient.R().
		SetFormData(map[string]string{
			"clusterId":       clusterId,
			"userid":          userid,
			"token":           token,
			"bucket":          bucket,
			"bucketPrefix":    prefix,
			"durationSeconds": fmt.Sprintf("%.0f", duration.Seconds()),
		}).
		SetResult(&AWSAccountResponse{}).
		Post(serverURL)

	if err != nil {
		klog.Error("fetch data from cloud error, ", err, ", ", serverURL)
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		klog.Error("fetch data from cloud response error, ", resp.StatusCode(), ", ", resp.Body())
		return nil, errors.New(string(resp.Body()))
	}

	awsResp := resp.Result().(*AWSAccountResponse)
	if awsResp.Code != http.StatusOK {
		klog.Error("get aws account from cloud error, ", awsResp.Code, ", ", awsResp.Message)
		return nil, errors.New(awsResp.Message)
	}
	klog.Infof("get aws account from cloud %v", util.ToJSON(awsResp))

	if awsResp.Data == nil {
		klog.Error("get aws account from cloud data is empty, ", awsResp.Code, ", ", awsResp.Message)
		return nil, errors.New("data is empty")
	}

	return awsResp.Data, nil
}

func getClusterId(ctx context.Context, client dynamic.Interface) (string, error) {
	gvr := schema.GroupVersionResource{
		Group:    "sys.bytetrade.io",
		Version:  "v1alpha1",
		Resource: "terminus",
	}

	data, err := client.Resource(gvr).Get(ctx, "terminus", metav1.GetOptions{})
	if err != nil {
		klog.Error("get terminus define error, ", err)
		return "", err
	}

	labels := data.GetLabels()
	if labels != nil {
		if id, ok := labels["bytetrade.io/cluster-id"]; ok {
			klog.Info("found cluster id, ", id)
			return id, nil
		}
	}

	klog.Error("cluster id not found")
	return "", errors.New("cluster id not found")
}

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

type AccountResponseData struct {
	Name  string `json:"name"`
	Value string `json:"value"`
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

type AWSAccount struct {
	Cloud      string `json:"cloud"`  // "AWS",
	Bucket     string `json:"bucket"` // "terminus-us-west-1",
	Token      string `json:"st"`
	Prefix     string `json:"prefix"` // "fbcf5f573ed242c28758-342957450633",
	Secret     string `json:"sk"`
	Key        string `json:"ak"`
	Expiration string `json:"expiration"` // "1705550635000",
	Region     string `json:"region"`     // "us-west-1"
}

type AWSAccountResponse struct {
	response.Header
	Data *AWSAccount `json:"data"`
}
