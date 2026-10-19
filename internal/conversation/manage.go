package conversation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Summary struct {
	ID           string
	FirstMessage string
	UpdatedAt    time.Time
	Current      bool
	Provider     string
	Model        string
}

func (s Store) List() ([]Summary, error) {
	ids, err := s.sessionIDs()
	if err != nil {
		return nil, err
	}

	currentID := ""
	current, err := s.loadCurrent()
	if err == nil {
		currentID = current.SessionID
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	summaries := make([]Summary, 0, len(ids))
	for _, id := range ids {
		session, err := s.loadSession(id)
		if err != nil {
			return nil, fmt.Errorf("读取任务 %s 失败: %w", id, err)
		}
		summaries = append(summaries, summarizeSession(session, session.ID == currentID))
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].UpdatedAt.Equal(summaries[j].UpdatedAt) {
			return summaries[i].ID < summaries[j].ID
		}
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})
	return summaries, nil
}

func (s Store) Resolve(prefix string) (string, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if len(prefix) < 6 || len(prefix) > sessionIDLength {
		return "", fmt.Errorf("哈希长度必须为 6 到 %d 位", sessionIDLength)
	}
	for _, char := range prefix {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return "", errors.New("哈希只能包含 0-9 和 a-f")
		}
	}

	ids, err := s.sessionIDs()
	if err != nil {
		return "", err
	}
	matches := make([]string, 0, 1)
	for _, id := range ids {
		if strings.HasPrefix(id, prefix) {
			matches = append(matches, id)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("没有匹配的任务: %s", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("哈希前缀 %s 匹配 %d 个任务，请提供更长的哈希", prefix, len(matches))
	}
}

func (s Store) Switch(prefix string) (Summary, error) {
	id, err := s.Resolve(prefix)
	if err != nil {
		return Summary{}, err
	}
	var session Session
	err = withStoreLock(s.Root, func() error {
		session, err = s.loadSession(id)
		if err != nil {
			return err
		}
		return s.writeCurrent(session.ID)
	})
	if err != nil {
		return Summary{}, err
	}
	return summarizeSession(session, true), nil
}

func (s Store) Load(prefix string) (Session, error) {
	id, err := s.Resolve(prefix)
	if err != nil {
		return Session{}, err
	}
	return s.loadSession(id)
}

func (s Store) Remove(prefix string) (Summary, error) {
	summaries, err := s.RemoveMany([]string{prefix})
	if err != nil {
		return Summary{}, err
	}
	if len(summaries) != 1 {
		return Summary{}, errors.New("删除任务失败")
	}
	return summaries[0], nil
}

func (s Store) RemoveMany(prefixes []string) ([]Summary, error) {
	var summaries []Summary
	err := withStoreLock(s.Root, func() error {
		var err error
		summaries, err = s.removeManyLocked(prefixes)
		return err
	})
	return summaries, err
}

func (s Store) removeManyLocked(prefixes []string) ([]Summary, error) {
	if len(prefixes) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(prefixes))
	seen := make(map[string]bool, len(prefixes))
	for _, prefix := range prefixes {
		id, err := s.Resolve(prefix)
		if err != nil {
			return nil, err
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}

	current, currentErr := s.loadCurrent()
	if currentErr != nil && !errors.Is(currentErr, ErrNotFound) {
		return nil, currentErr
	}
	currentID := ""
	if currentErr == nil {
		currentID = current.SessionID
	}

	summaries := make([]Summary, 0, len(ids))
	for _, id := range ids {
		session, err := s.loadSession(id)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summarizeSession(session, session.ID == currentID))
	}

	removedCurrent := false
	for _, summary := range summaries {
		if err := os.Remove(s.sessionPath(summary.ID)); err != nil {
			return nil, fmt.Errorf("删除任务文件失败: %w", err)
		}
		removedCurrent = removedCurrent || summary.Current
	}
	if removedCurrent {
		if err := os.Remove(s.lastPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("清除当前任务失败: %w", err)
		}
	}
	return summaries, nil
}

func summarizeSession(session Session, current bool) Summary {
	return Summary{
		ID:           session.ID,
		FirstMessage: firstUserMessage(session.Messages),
		UpdatedAt:    session.UpdatedAt,
		Current:      current,
		Provider:     session.Provider,
		Model:        session.Model,
	}
}

func (s Store) sessionIDs() ([]string, error) {
	if s.Root == "" {
		return nil, errors.New("会话缓存目录不能为空")
	}
	entries, err := os.ReadDir(s.sessionsDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取任务目录失败: %w", err)
	}

	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if sessionIDPattern.MatchString(id) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func firstUserMessage(messages []Message) string {
	for _, message := range messages {
		if message.Role == "user" {
			return message.Content
		}
	}
	return ""
}
