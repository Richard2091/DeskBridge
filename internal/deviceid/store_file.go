package deviceid

import (
	"errors"
	"os"
)

// FileStore 把设备私钥存进文件（base64 文本，权限 0600），是系统密钥库
// 不可用时的备用方案。
type FileStore struct {
	Path string
}

// Get 读取文件中的私钥。文件不存在时返回 ErrNotFound。
func (fs *FileStore) Get() ([]byte, error) {
	data, err := os.ReadFile(fs.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return decodeKey(data)
}

// Set 把私钥写入文件，权限 0600。
func (fs *FileStore) Set(data []byte) error {
	return os.WriteFile(fs.Path, encodeKey(data), 0o600)
}
