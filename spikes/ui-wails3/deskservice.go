package main

// AppConfig 由 Go 侧通过绑定提供给前端。
type AppConfig struct {
	AutoStream bool   `json:"autoStream"`
	APIBase    string `json:"apiBase"`
	FramesBase string `json:"framesBase"`
	Mode       string `json:"mode"`
}

// DeskService 用于验证 Wails 绑定可用。
type DeskService struct {
	cfg AppConfig
}

func (d *DeskService) Config() AppConfig {
	return d.cfg
}

func (d *DeskService) Ping(name string) string {
	return "Go 绑定正常，你好 " + name
}