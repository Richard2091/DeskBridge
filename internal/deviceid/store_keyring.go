package deviceid

import (
	"encoding/base64"
	"errors"

	"github.com/zalando/go-keyring"
)

// KeyringStore 把设备私钥存进系统密钥库（macOS 钥匙串、Windows 凭据管理器、
// Linux Secret Service）。私钥以 base64 文本保存。
type KeyringStore struct{}

// Get 读取密钥库中的私钥。没有保存过时返回 ErrNotFound。
func (KeyringStore) Get() ([]byte, error) {
	s, err := keyring.Get(keyringService, keyringUser)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// Set 把私钥写入密钥库。
func (KeyringStore) Set(data []byte) error {
	return keyring.Set(keyringService, keyringUser, base64.StdEncoding.EncodeToString(data))
}
