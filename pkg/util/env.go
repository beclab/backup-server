package util

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	// "k8s.io/klog"
	// "k8s.io/utils/ptr"
)

func EnvOrDefault(name, def string) string {
	v := os.Getenv(name)

	if v == "" && def != "" {
		return def
	}
	v = strings.TrimRight(v, "/")
	return v
}

func GetUserServiceAccountToken(ctx context.Context, client kubernetes.Interface, user string) (string, error) {
	namespace := fmt.Sprintf("user-system-%s", user)
	var exp int64 = 86400
	tr := &authv1.TokenRequest{
		Spec: authv1.TokenRequestSpec{
			Audiences:         []string{"https://kubernetes.default.svc.cluster.local"},
			ExpirationSeconds: &exp, //ptr.To[int64](86400), // one day
		},
	}
	token, err := client.CoreV1().ServiceAccounts(namespace).
		CreateToken(ctx, "user-backend", tr, metav1.CreateOptions{})
	if err != nil {
		klog.Errorf("Failed to create token for user %s in namespace %s: %v", user, namespace, err)
		return "", err
	}
	return token.Status.Token, nil
}

func GetBackendToken(randomKey string) (string, error) {
	if randomKey == "" {
		randomKey = os.Getenv("APP_RANDOM_KEY")
	}
	timestamp := getTimestamp()
	cipherText, err := AesEncrypt([]byte(timestamp), []byte(randomKey))
	if err != nil {
		return "", err
	}
	b64CipherText := base64.StdEncoding.EncodeToString(cipherText)
	backendTokenNonce := "appservice:" + b64CipherText
	return backendTokenNonce, nil
}

func getTimestamp() string {
	t := time.Now().Unix()
	return strconv.Itoa(int(t))
}

func AesEncrypt(origin, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	origin = PKCS7Padding(origin, blockSize)
	blockMode := cipher.NewCBCEncrypter(block, key[:blockSize])
	crypted := make([]byte, len(origin))
	blockMode.CryptBlocks(crypted, origin)
	return crypted, nil
}

func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}
