package util

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"os"
	"strconv"
	"strings"
	"time"
)

func EnvOrDefault(name, def string) string {
	v := os.Getenv(name)

	if v == "" && def != "" {
		return def
	}
	v = strings.TrimRight(v, "/")
	return v
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
