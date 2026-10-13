// Package deviceid 管理 DeskBridge 设备 ID：一个 ed25519 密钥对，
// 私钥保存在系统密钥库（备用：文件），设备 ID 由公钥派生。
package deviceid

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound 表示存储中还没有设备私钥。
var ErrNotFound = errors.New("deviceid: 没有已保存的设备私钥")

// 服务名和用户名，用于系统密钥库。
const (
	keyringService = "DeskBridge"
	keyringUser    = "device-key"
)

// Device 是本设备的身份：ed25519 密钥对及派生的设备 ID。
type Device struct {
	priv ed25519.PrivateKey
}

// ID 返回设备 ID：公钥 SHA-256 的前 20 字节，base32 小写、无填充
// （32 个字符）。它不随登录账号变化，用于在 DeskBridge 协议中标识设备。
func (d *Device) ID() string {
	sum := sha256.Sum256(d.priv.Public().(ed25519.PublicKey))
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:20]))
}

// PublicKey 返回设备公钥。
func (d *Device) PublicKey() ed25519.PublicKey {
	return d.priv.Public().(ed25519.PublicKey)
}

// Sign 用设备私钥签名数据。
func (d *Device) Sign(data []byte) []byte {
	return ed25519.Sign(d.priv, data)
}

// Store 是设备私钥的存取接口。数据是原始私钥字节（ed25519 私钥 64 字节），
// 编码和加密由具体实现负责；未保存过时 Get 返回 ErrNotFound。
type Store interface {
	Get() ([]byte, error)
	Set([]byte) error
}

// Load 读取已保存的设备私钥；没有就生成新的并保存。
func Load(store Store) (*Device, error) {
	data, err := store.Get()
	if errors.Is(err, ErrNotFound) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("生成设备密钥对失败: %w", err)
		}
		if err := store.Set(priv); err != nil {
			return nil, fmt.Errorf("保存设备私钥失败: %w", err)
		}
		return &Device{priv: priv}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取设备私钥失败: %w", err)
	}
	if len(data) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("设备私钥长度 %d 字节，期望 %d", len(data), ed25519.PrivateKeySize)
	}
	return &Device{priv: ed25519.PrivateKey(data)}, nil
}

// encodeKey 把私钥编码成 base64 文本，密钥库和文件都用这种格式。
func encodeKey(priv ed25519.PrivateKey) []byte {
	return []byte(base64.StdEncoding.EncodeToString(priv))
}

// decodeKey 解码 base64 文本形式的私钥并检查长度。
func decodeKey(data []byte) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("设备私钥不是合法的 base64: %w", err)
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("设备私钥长度 %d 字节，期望 %d", len(raw), ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(raw), nil
}
