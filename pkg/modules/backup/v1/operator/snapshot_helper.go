package operator

import (
	"context"
	"encoding/json"
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

func (o *SnapshotOperator) getOlaresId(owner string) (string, error) {
	dynamicClient, err := o.factory.DynamicClient()
	if err != nil {
		return "", errors.WithStack(fmt.Errorf("get dynamic client error %v", err))
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
			return errors.WithStack(fmt.Errorf("get user error %v", err))
		}
		obj := unstructuredUser.UnstructuredContent()
		olaresName, _, err = unstructured.NestedString(obj, "spec", "email")
		if err != nil {
			return errors.WithStack(fmt.Errorf("get user nested string error %v", err))
		}
		return nil
	}); err != nil {
		return "", errors.WithStack(err)
	}

	return olaresName, nil
}

func (o *SnapshotOperator) getPath(backup *sysv1.Backup) string {
	var p string
	for k, v := range backup.Spec.BackupType {
		if k != "file" {
			continue
		}
		var m = make(map[string]string)
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			log.Errorf("unmarshal backup type error: %v, value: %s", err, v)
			continue
		}
		p = m["path"]
	}

	return p
}
