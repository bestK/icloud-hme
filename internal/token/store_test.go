package token

import (
	"os"
	"sync"
	"testing"
	"time"
)

func newTempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEnsureAdmin_Idempotent(t *testing.T) {
	s := newTempStore(t)
	a1, err := s.EnsureAdmin("secret-1")
	if err != nil {
		t.Fatal(err)
	}
	a2, err := s.EnsureAdmin("secret-1")
	if err != nil {
		t.Fatal(err)
	}
	if a1.ID != a2.ID {
		t.Fatalf("EnsureAdmin 应保持 admin id 稳定,got %s vs %s", a1.ID, a2.ID)
	}
	if a2.Secret != "secret-1" {
		t.Fatalf("secret mismatch: %s", a2.Secret)
	}
}

func TestEnsureAdmin_RotatesSecret(t *testing.T) {
	s := newTempStore(t)
	if _, err := s.EnsureAdmin("old"); err != nil {
		t.Fatal(err)
	}
	if got := s.FindBySecret("old"); got == nil {
		t.Fatal("旧 secret 应先命中")
	}
	if _, err := s.EnsureAdmin("new"); err != nil {
		t.Fatal(err)
	}
	if got := s.FindBySecret("old"); got != nil {
		t.Fatal("旧 secret 换掉后不应再命中")
	}
	if got := s.FindBySecret("new"); got == nil || got.Role != RoleAdmin {
		t.Fatalf("新 secret 应指向 admin,got %+v", got)
	}
}

func TestAddAndDelete_UserToken(t *testing.T) {
	s := newTempStore(t)
	if _, err := s.EnsureAdmin("admin"); err != nil {
		t.Fatal(err)
	}
	tk, err := s.Add("laptop", RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Secret == "" {
		t.Fatal("Add 应返回 secret 明文")
	}
	if s.FindBySecret(tk.Secret) == nil {
		t.Fatal("新 token 应可通过 secret 查到")
	}

	all := s.List()
	if len(all) != 2 {
		t.Fatalf("List 应返回 2 条,got %d", len(all))
	}
	for _, t2 := range all {
		if t2.Secret != "" {
			t.Fatalf("List 结果不应包含 secret,got %s", t2.Secret)
		}
	}

	if err := s.Delete(tk.ID); err != nil {
		t.Fatal(err)
	}
	if s.FindBySecret(tk.Secret) != nil {
		t.Fatal("删除后 secret 不应命中")
	}
}

func TestDelete_AdminForbidden(t *testing.T) {
	s := newTempStore(t)
	a, err := s.EnsureAdmin("admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(a.ID); err == nil {
		t.Fatal("删除 admin 应报错")
	}
}

func TestBindAndUnbindAlias(t *testing.T) {
	s := newTempStore(t)
	tk, _ := s.Add("t", RoleUser)
	ref := AliasRef{AnonymousID: "abc", Email: "x@icloud.com", AccountID: "acc_1", CreatedAt: time.Now()}
	if err := s.BindAlias(tk.ID, ref); err != nil {
		t.Fatal(err)
	}
	if !s.HasAlias(tk.ID, "abc") {
		t.Fatal("绑定后应能查到")
	}
	// duplicate bind is no-op
	if err := s.BindAlias(tk.ID, ref); err != nil {
		t.Fatal(err)
	}
	if got := s.AliasesOf(tk.ID); len(got) != 1 {
		t.Fatalf("重复绑定应去重,got %d", len(got))
	}
	if err := s.UnbindAlias(tk.ID, "abc"); err != nil {
		t.Fatal(err)
	}
	if s.HasAlias(tk.ID, "abc") {
		t.Fatal("解绑后不应命中")
	}
}

func TestPersistence_ReloadKeepsData(t *testing.T) {
	dir := t.TempDir()
	s1, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s1.EnsureAdmin("admin-secret")
	tk, _ := s1.Add("u1", RoleUser)
	_ = s1.BindAlias(tk.ID, AliasRef{AnonymousID: "aid1", Email: "a@icloud.com", CreatedAt: time.Now()})

	s2, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.FindBySecret("admin-secret"); got == nil || got.Role != RoleAdmin {
		t.Fatal("重新加载后 admin 消失")
	}
	if got := s2.FindBySecret(tk.Secret); got == nil {
		t.Fatal("重新加载后 user token 消失")
	}
	if !s2.HasAlias(tk.ID, "aid1") {
		t.Fatal("重新加载后 alias 归属消失")
	}
}

func TestAtomicSave_TmpCleanedUp(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnsureAdmin("x"); err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		if len(f.Name()) > 4 && f.Name()[len(f.Name())-4:] == ".tmp" {
			t.Fatalf("原子写不应残留 tmp: %s", f.Name())
		}
	}
}

func TestConcurrentAddAndBind(t *testing.T) {
	s := newTempStore(t)
	_, _ = s.EnsureAdmin("admin")

	var wg sync.WaitGroup
	tokens := make([]*Token, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tk, err := s.Add("t", RoleUser)
			if err != nil {
				t.Errorf("Add: %v", err)
				return
			}
			tokens[i] = tk
		}(i)
	}
	wg.Wait()

	for i, tk := range tokens {
		if tk == nil {
			t.Fatalf("token[%d] nil", i)
		}
		for j := 0; j < 5; j++ {
			wg.Add(1)
			go func(id, j int) {
				defer wg.Done()
				_ = s.BindAlias(tokens[id].ID, AliasRef{AnonymousID: string(rune('a' + j)), CreatedAt: time.Now()})
			}(i, j)
		}
	}
	wg.Wait()

	for _, tk := range tokens {
		if got := s.AliasesOf(tk.ID); len(got) != 5 {
			t.Fatalf("每个 token 应有 5 个 alias,got %d", len(got))
		}
	}
}
