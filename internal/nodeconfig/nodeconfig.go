// Package nodeconfig 读写节点设置文件（数据目录下的 node.json）。
// 所有字段都可缺省；预授权密钥不放在这里。
package nodeconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Config 是节点设置。零值表示“未设置”，由调用方套用默认值。
type Config struct {
	Name       string `json:"name"`
	Hostname   string `json:"hostname"`
	ControlURL string `json:"control_url"`
	Port       int    `json:"port"`
}

// Load 读取设置文件。文件不存在返回零值；内容损坏返回错误。
func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("读取节点设置 %s 失败: %w", path, err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("节点设置 %s 不是有效的 JSON: %w", path, err)
	}
	return c, nil
}

// Save 先写临时文件再改名，避免写到一半崩溃留下损坏的文件。
// 调用方需先保证数据目录已创建并限制了权限。
func Save(path string, c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("写入节点设置 %s 失败: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("保存节点设置 %s 失败: %w", path, err)
	}
	return nil
}
