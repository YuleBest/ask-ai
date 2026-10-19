package conversation

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"syscall"
	"time"

	"github.com/YuleBest/ask-ai/internal/fileutil"
)

const (
	sessionVersion  = 1
	currentVersion  = 1
	sessionIDBytes  = 16
	sessionIDLength = sessionIDBytes * 2
)

var (
	ErrNotFound      = errors.New("没有可追问的上一次对话")
	sessionIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Session struct {
	Version   int       `json:"version"`
	Revision  int64     `json:"revision,omitempty"`
	ID        string    `json:"id"`
	Provider  string    `json:"provider,omitempty"`
	Model     string    `json:"model,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

type Store struct {
	Root string
}

type currentSession struct {
	Version   int    `json:"version"`
	SessionID string `json:"session_id"`
}

func DefaultStore() (Store, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return Store{}, fmt.Errorf("无法确定用户缓存目录: %w", err)
	}
	return Store{Root: filepath.Join(dir, "ask-ai")}, nil
}

func NewSession() (Session, error) {
	var rawID [sessionIDBytes]byte
	if _, err := rand.Read(rawID[:]); err != nil {
		return Session{}, fmt.Errorf("生成会话 ID 失败: %w", err)
	}
	now := time.Now().UTC()
	return Session{
		Version:   sessionVersion,
		ID:        hex.EncodeToString(rawID[:]),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s Store) LoadLast() (Session, error) {
	current, err := s.loadCurrent()
	if err != nil {
		return Session{}, err
	}
	session, err := s.loadSession(current.SessionID)
	if err != nil {
		return Session{}, err
	}
	if session.ID != current.SessionID {
		return Session{}, errors.New("当前会话 ID 与会话文件不一致")
	}
	return session, nil
}

func (s Store) Save(session Session) error {
	return s.save(session, true)
}

func (s Store) Update(session Session) error {
	return s.save(session, false)
}

func (s Store) save(session Session, setCurrent bool) error {
	if s.Root == "" {
		return errors.New("会话缓存目录不能为空")
	}
	return withStoreLock(s.Root, func() error {
		return s.saveLocked(session, setCurrent)
	})
}

func (s Store) saveLocked(session Session, setCurrent bool) error {
	if s.Root == "" {
		return errors.New("会话缓存目录不能为空")
	}
	if session.Version == 0 {
		session.Version = sessionVersion
	}
	if err := validateSession(session); err != nil {
		return err
	}

	if existing, err := s.loadSession(session.ID); err == nil {
		session.Messages = mergeMessages(existing.Messages, session.Messages)
		session.CreatedAt = existing.CreatedAt
		session.Revision = existing.Revision + 1
	} else if !errors.Is(err, ErrNotFound) {
		return err
	} else {
		session.Revision++
	}
	now := time.Now().UTC()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	session.UpdatedAt = now

	sessionData, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("生成会话数据失败: %w", err)
	}
	sessionData = append(sessionData, '\n')
	if err := writeAtomic(s.sessionPath(session.ID), sessionData); err != nil {
		return fmt.Errorf("保存会话失败: %w", err)
	}

	if setCurrent {
		if err := s.writeCurrent(session.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) loadCurrent() (currentSession, error) {
	if s.Root == "" {
		return currentSession{}, errors.New("会话缓存目录不能为空")
	}
	data, err := os.ReadFile(s.lastPath())
	if errors.Is(err, os.ErrNotExist) {
		return currentSession{}, ErrNotFound
	}
	if err != nil {
		return currentSession{}, fmt.Errorf("读取当前会话失败: %w", err)
	}

	var current currentSession
	if err := json.Unmarshal(data, &current); err != nil {
		return currentSession{}, fmt.Errorf("解析当前会话失败: %w", err)
	}
	if current.Version != currentVersion {
		return currentSession{}, fmt.Errorf("不支持的当前会话版本: %d", current.Version)
	}
	if !sessionIDPattern.MatchString(current.SessionID) {
		return currentSession{}, errors.New("当前会话 ID 无效")
	}
	return current, nil
}

func (s Store) loadSession(sessionID string) (Session, error) {
	if s.Root == "" {
		return Session{}, errors.New("会话缓存目录不能为空")
	}
	data, err := os.ReadFile(s.sessionPath(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("读取会话失败: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, fmt.Errorf("解析会话失败: %w", err)
	}
	if err := validateSession(session); err != nil {
		return Session{}, err
	}
	if session.ID != sessionID {
		return Session{}, fmt.Errorf("会话文件 ID 与内容不一致: %s != %s", sessionID, session.ID)
	}
	return session, nil
}

func mergeMessages(current, incoming []Message) []Message {
	common := 0
	for common < len(current) && common < len(incoming) && current[common] == incoming[common] {
		common++
	}
	if common == len(incoming) {
		return append([]Message(nil), current...)
	}
	if common == len(current) {
		return append(append([]Message(nil), current...), incoming[common:]...)
	}
	return append(append([]Message(nil), current...), incoming[common:]...)
}

func (s Store) writeCurrent(sessionID string) error {
	currentData, err := json.MarshalIndent(currentSession{
		Version:   currentVersion,
		SessionID: sessionID,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("生成当前会话数据失败: %w", err)
	}
	currentData = append(currentData, '\n')
	if err := writeAtomic(s.lastPath(), currentData); err != nil {
		return fmt.Errorf("保存当前会话失败: %w", err)
	}
	return nil
}

func validateSession(session Session) error {
	if session.Version != sessionVersion {
		return fmt.Errorf("不支持的会话版本: %d", session.Version)
	}
	if !sessionIDPattern.MatchString(session.ID) {
		return errors.New("会话 ID 无效")
	}
	if (session.Provider == "") != (session.Model == "") {
		return errors.New("会话的 provider 和 model 必须同时设置")
	}
	if len(session.Messages) == 0 {
		return errors.New("会话消息不能为空")
	}
	for _, message := range session.Messages {
		if message.Role != "user" && message.Role != "assistant" {
			return fmt.Errorf("会话包含不支持的 role: %q", message.Role)
		}
		if message.Content == "" {
			return errors.New("会话包含空消息")
		}
	}
	return nil
}

func (s Store) sessionPath(sessionID string) string {
	return filepath.Join(s.sessionsDir(), sessionID+".json")
}

func (s Store) sessionsDir() string {
	return filepath.Join(s.Root, "sessions")
}

func (s Store) lastPath() string {
	return filepath.Join(s.Root, "last.json")
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建缓存目录失败: %w", err)
	}
	file, err := os.CreateTemp(dir, ".session-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("设置文件权限失败: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("同步临时文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	if err := fileutil.Replace(tempPath, path); err != nil {
		return fmt.Errorf("替换目标文件失败: %w", err)
	}
	if runtime.GOOS != "windows" {
		dirFile, err := os.Open(dir)
		if err != nil {
			return fmt.Errorf("打开缓存目录失败: %w", err)
		}
		syncErr := dirFile.Sync()
		closeErr := dirFile.Close()
		if errors.Is(syncErr, syscall.EINVAL) || errors.Is(syncErr, syscall.ENOTSUP) {
			syncErr = nil
		}
		if syncErr != nil {
			return fmt.Errorf("同步缓存目录失败: %w", syncErr)
		}
		if closeErr != nil {
			return fmt.Errorf("关闭缓存目录失败: %w", closeErr)
		}
	}
	return nil
}
