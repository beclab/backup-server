package http

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/util"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
)

func RequestJSON(method, url string, headers map[string]string, body, to any) (statusCode int, err error) {
	var (
		reqBytes []byte
		br       io.Reader
	)

	if body != nil {
		reqBytes, err = json.Marshal(body)
		if err != nil {
			return 0, errors.WithStack(err)
		}
		br = bytes.NewReader(reqBytes)
	}

	var req *http.Request
	req, err = http.NewRequest(method, url, br)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	req.Header.Set("Accept", "application/json")
	if util.ListContains([]string{"POST", "PUT", "PATCH"}, method) && body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if headers != nil {
		for k, v := range headers {
			req.Header.Add(k, v)
		}
	}

	var resp *http.Response
	resp, err = (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	var respBytes []byte
	respBytes, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		err = errors.WithStack(err)
		return
	}
	defer resp.Body.Close()

	if to != nil {
		if err = json.Unmarshal(respBytes, to); err != nil {
			return 0, errors.WithStack(err)
		}
	}

	return resp.StatusCode, nil
}

func Post[T any](ctx context.Context, url string, headers map[string]string, data interface{}, debug bool) (*T, error) {
	var result T
	client := resty.New().SetTimeout(10 * time.Second).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}).R().SetDebug(debug)

	if headers != nil {
		client.SetHeaders(headers)
	}

	resp, err := client.SetContext(ctx).
		SetBody(data).
		SetResult(&result).
		Post(url)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("request failed, status code: %d", resp.StatusCode())
	}

	return &result, nil
}
