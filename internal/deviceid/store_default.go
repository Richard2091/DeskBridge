package deviceid

import (
	"errors"
	"log"
	"sync"
)

// fallbackStore 先用系统密钥库；密钥库不可用（读取或保存出错，且不是
// "还没保存过"）时退回文件，并打日志说明。读取顺序：密钥库 → 文件 → 生成新的。
type fallbackStore struct {
	keyring Store
	file    *FileStore
	logf    func(format string, args ...any)

	mu     sync.Mutex
	broken bool // 密钥库一旦确定不可用，后续直接用文件，避免每次都重试
	warned bool
}

// DefaultStore 返回默认的设备私钥存储：先系统密钥库，退回 file 指定的文件。
func DefaultStore(file string) Store {
	return &fallbackStore{
		keyring: KeyringStore{},
		file:    &FileStore{Path: file},
		logf:    log.Printf,
	}
}

// Get 按密钥库 → 文件的顺序读取私钥。
func (s *fallbackStore) Get() ([]byte, error) {
	if s.keyringUsable() {
		data, err := s.keyring.Get()
		if err == nil {
			return data, nil
		}
		if errors.Is(err, ErrNotFound) {
			// 密钥库可用但没保存过；文件里可能有早先退回时保存的私钥。
			return s.file.Get()
		}
		s.markBroken(err)
	}
	return s.file.Get()
}

// Set 保存私钥到密钥库；密钥库不可用时退回文件。
func (s *fallbackStore) Set(data []byte) error {
	if s.keyringUsable() {
		err := s.keyring.Set(data)
		if err == nil {
			return nil
		}
		s.markBroken(err)
	}
	return s.file.Set(data)
}

func (s *fallbackStore) keyringUsable() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.broken
}

// markBroken 记录密钥库不可用并打一次日志。
func (s *fallbackStore) markBroken(cause error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.broken = true
	if !s.warned {
		s.warned = true
		if s.logf != nil {
			s.logf("系统密钥库不可用（%v），改用文件 %s 保存设备私钥", cause, s.file.Path)
		}
	}
}
