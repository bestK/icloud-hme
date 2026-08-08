// Package token 提供 API Token 管理与数据隔离。
//
// 一个 admin token(从环境变量 ADMIN_TOKEN 引导)可以增删普通 token;
// 普通 token 只能看到/操作自己创建的 HME 别名,并记录创建统计。
package token

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Role 是 token 的角色。
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// Token 描述一个 API 访问凭据。
type Token struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Secret     string     `json:"secret"`
	Role       Role       `json:"role"`
	Aliases    []AliasRef `json:"aliases,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt time.Time  `json:"last_used_at,omitempty"`
}

// AliasRef 记录一个 HME 别名的归属信息。
type AliasRef struct {
	AnonymousID string    `json:"anonymous_id"`
	Email       string    `json:"email"`
	Label       string    `json:"label"`
	AccountID   string    `json:"account_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// Store 是线程安全的 token 存储。
type Store struct {
	mu       sync.Mutex
	tokens   map[string]*Token
	bySecret map[string]string
	dataFile string
}

// NewStore 打开或创建 tokens.json。
func NewStore(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	s := &Store{
		tokens:   make(map[string]*Token),
		bySecret: make(map[string]string),
		dataFile: filepath.Join(dataDir, "tokens.json"),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	raw, err := os.ReadFile(s.dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var wrapper struct {
		Tokens map[string]*Token `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return err
	}
	if wrapper.Tokens == nil {
		wrapper.Tokens = make(map[string]*Token)
	}
	s.tokens = wrapper.Tokens
	s.rebuildIndex()
	return nil
}

func (s *Store) rebuildIndex() {
	s.bySecret = make(map[string]string, len(s.tokens))
	for id, t := range s.tokens {
		if t.Secret != "" {
			s.bySecret[t.Secret] = id
		}
	}
}

func (s *Store) save() error {
	wrapper := struct {
		Tokens    map[string]*Token `json:"tokens"`
		UpdatedAt string            `json:"updated_at"`
	}{
		Tokens:    s.tokens,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	raw, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.dataFile + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.dataFile)
}

// EnsureAdmin 保证存在唯一一个 role=admin 的 token,其 secret 等于给定值。
// 若已有其他 admin token 会被强制更新。返回该 admin token。
func (s *Store) EnsureAdmin(secret string) (*Token, error) {
	if secret == "" {
		return nil, fmt.Errorf("admin token secret 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var admin *Token
	for _, t := range s.tokens {
		if t.Role == RoleAdmin {
			if admin == nil {
				admin = t
			} else {
				// 多个 admin,清理多余的
				delete(s.tokens, t.ID)
			}
		}
	}
	if admin == nil {
		admin = &Token{
			ID:        "tk_" + uuid.New().String()[:8],
			Name:      "admin",
			Role:      RoleAdmin,
			CreatedAt: time.Now(),
		}
		s.tokens[admin.ID] = admin
	}
	admin.Secret = secret
	s.rebuildIndex()
	if err := s.save(); err != nil {
		return nil, err
	}
	cp := *admin
	return &cp, nil
}

// Add 创建一个新的 user token。返回带 secret 的完整对象(仅此一次可见明文)。
func (s *Store) Add(name string, role Role) (*Token, error) {
	if name == "" {
		return nil, fmt.Errorf("name 不能为空")
	}
	if role == "" {
		role = RoleUser
	}
	if role != RoleUser && role != RoleAdmin {
		return nil, fmt.Errorf("非法 role: %s", role)
	}
	secret, err := randomSecret()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &Token{
		ID:        "tk_" + uuid.New().String()[:8],
		Name:      name,
		Secret:    secret,
		Role:      role,
		CreatedAt: time.Now(),
	}
	s.tokens[t.ID] = t
	s.bySecret[secret] = t.ID
	if err := s.save(); err != nil {
		delete(s.tokens, t.ID)
		delete(s.bySecret, secret)
		return nil, err
	}
	cp := *t
	return &cp, nil
}

// Delete 删除一个 token。不允许删除 admin。
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	if !ok {
		return fmt.Errorf("token 不存在: %s", id)
	}
	if t.Role == RoleAdmin {
		return fmt.Errorf("不允许删除 admin token")
	}
	delete(s.bySecret, t.Secret)
	delete(s.tokens, id)
	return s.save()
}

// List 返回所有 token(secret 已脱敏为空字符串)。
func (s *Store) List() []*Token {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Token, 0, len(s.tokens))
	for _, t := range s.tokens {
		cp := *t
		cp.Secret = ""
		out = append(out, &cp)
	}
	return out
}

// FindBySecret 用 secret 查 token,不存在返回 nil。
func (s *Store) FindBySecret(secret string) *Token {
	if secret == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.bySecret[secret]
	if !ok {
		return nil
	}
	t := s.tokens[id]
	if t == nil {
		return nil
	}
	cp := *t
	return &cp
}

// Touch 更新 last_used_at,不同步写盘(累计更新减少 IO)。
func (s *Store) Touch(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tokens[id]; ok {
		t.LastUsedAt = time.Now()
	}
}

// Flush 强制写盘(如果需要保证 LastUsedAt 落盘)。
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save()
}

// BindAlias 记录一个 alias 归属于指定 token。
func (s *Store) BindAlias(tokenID string, ref AliasRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[tokenID]
	if !ok {
		return fmt.Errorf("token 不存在: %s", tokenID)
	}
	for _, a := range t.Aliases {
		if a.AnonymousID != "" && a.AnonymousID == ref.AnonymousID {
			return nil
		}
	}
	t.Aliases = append(t.Aliases, ref)
	return s.save()
}

// UnbindAlias 移除 alias 归属。
func (s *Store) UnbindAlias(tokenID, anonymousID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[tokenID]
	if !ok {
		return fmt.Errorf("token 不存在: %s", tokenID)
	}
	filtered := t.Aliases[:0]
	changed := false
	for _, a := range t.Aliases {
		if a.AnonymousID == anonymousID {
			changed = true
			continue
		}
		filtered = append(filtered, a)
	}
	t.Aliases = filtered
	if !changed {
		return nil
	}
	return s.save()
}

// HasAlias 校验 alias 是否归属于该 token。
func (s *Store) HasAlias(tokenID, anonymousID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[tokenID]
	if !ok {
		return false
	}
	for _, a := range t.Aliases {
		if a.AnonymousID == anonymousID {
			return true
		}
	}
	return false
}

// AliasesOf 返回该 token 名下所有 alias 归属记录(副本)。
func (s *Store) AliasesOf(tokenID string) []AliasRef {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[tokenID]
	if !ok {
		return nil
	}
	out := make([]AliasRef, len(t.Aliases))
	copy(out, t.Aliases)
	return out
}

// HasAliasEmail 用 email 判断归属(anonymousId 未知时的兜底)。
func (s *Store) HasAliasEmail(tokenID, email string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[tokenID]
	if !ok {
		return false
	}
	for _, a := range t.Aliases {
		if a.Email == email {
			return true
		}
	}
	return false
}

func randomSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

