package conversation

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"github.com/YuleBest/ask-ai/internal/i18n"
)

const bindingVersion = 1

var (
	ErrNotBound          = i18n.NewDynamic("当前终端未绑定任务")
	terminalIDPattern    = regexp.MustCompile(`^[a-f0-9]{32}$`)
	terminalBindingStore = "terminals"
)

type BindingStore struct {
	Root       string
	TerminalID string
}

type terminalBinding struct {
	Version   int    `json:"version"`
	SessionID string `json:"session_id"`
	ProcessID int    `json:"process_id"`
}

func NewBindingStore(root, terminalID string) BindingStore {
	return BindingStore{
		Root:       root,
		TerminalID: terminalID,
	}
}

func (s BindingStore) Load() (string, error) {
	if err := s.validate(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotBound
	}
	if err != nil {
		return "", i18n.Errorf("读取终端绑定失败: %w", err)
	}

	var binding terminalBinding
	if err := json.Unmarshal(data, &binding); err != nil {
		return "", i18n.Errorf("解析终端绑定失败: %w", err)
	}
	if binding.Version != bindingVersion {
		return "", i18n.Errorf("不支持的终端绑定版本: %d", binding.Version)
	}
	if !processAlive(binding.ProcessID) {
		if err := s.Remove(); err != nil {
			return "", err
		}
		return "", ErrNotBound
	}
	if !sessionIDPattern.MatchString(binding.SessionID) {
		return "", i18n.New("终端绑定的任务 ID 无效")
	}
	return binding.SessionID, nil
}

func (s BindingStore) Save(sessionID string) error {
	if err := s.validate(); err != nil {
		return err
	}
	if !sessionIDPattern.MatchString(sessionID) {
		return i18n.New("任务 ID 无效")
	}
	data, err := json.MarshalIndent(terminalBinding{
		Version:   bindingVersion,
		SessionID: sessionID,
		ProcessID: os.Getpid(),
	}, "", "  ")
	if err != nil {
		return i18n.Errorf("生成终端绑定失败: %w", err)
	}
	data = append(data, '\n')
	if err := writeAtomic(s.path(), data); err != nil {
		return i18n.Errorf("保存终端绑定失败: %w", err)
	}
	return nil
}

func (s BindingStore) Remove() error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := os.Remove(s.path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return i18n.Errorf("清除终端绑定失败: %w", err)
	}
	return nil
}

func (s BindingStore) validate() error {
	if s.Root == "" {
		return i18n.New("会话缓存目录不能为空")
	}
	if !terminalIDPattern.MatchString(s.TerminalID) {
		return i18n.New("终端会话 ID 无效")
	}
	return nil
}

func (s BindingStore) path() string {
	return filepath.Join(s.Root, terminalBindingStore, s.TerminalID+".json")
}
