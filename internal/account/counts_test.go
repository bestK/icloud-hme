package account

import (
	"testing"

	"icloud-hme/internal/hme"
)

func TestCountAliases(t *testing.T) {
	total, active := countAliases([]hme.Alias{
		{Active: true}, {Active: false}, {Active: true},
	})
	if total != 3 || active != 2 {
		t.Fatalf("got total=%d active=%d, want 3/2", total, active)
	}
	if total, active = countAliases(nil); total != 0 || active != 0 {
		t.Fatalf("空列表应为 0/0,got %d/%d", total, active)
	}
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

// 未核对过的账号,基数是"不知道",增量必须被忽略 ——
// 否则会凭空造出一个看着像真的数字。
func TestApplyAliasDeltaSkipsUncountedAccount(t *testing.T) {
	m := newTestManager(t)
	acc, err := m.AddAccount("t", "", "icloud.com", "")
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	if acc.AliasCountedAt != "" {
		t.Fatalf("无 Cookie 建号不该有 counted_at,got %q", acc.AliasCountedAt)
	}

	m.ApplyAliasDelta(acc.ID, AliasCreated)

	got, _ := m.GetAccount(acc.ID)
	if got.AliasTotal != 0 || got.AliasActive != 0 || got.AliasCountedAt != "" {
		t.Fatalf("未核对账号被改动了: total=%d active=%d counted_at=%q",
			got.AliasTotal, got.AliasActive, got.AliasCountedAt)
	}
}

func TestApplyAliasDeltaAfterCounted(t *testing.T) {
	m := newTestManager(t)
	acc, err := m.AddAccount("t", "", "icloud.com", "")
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}

	// 用一份"已拉到手"的列表建立基数,绕开网络
	m.SetAliasCountsFrom(acc.ID, []hme.Alias{
		{Active: true}, {Active: true}, {Active: false},
	})
	if got, _ := m.GetAccount(acc.ID); got.AliasCountedAt == "" {
		t.Fatal("SetAliasCountsFrom 应写入 counted_at")
	}

	cases := []struct {
		name                 string
		delta                AliasDelta
		wantTotal, wantAcive int
	}{
		{"新建", AliasCreated, 4, 3},
		{"停用", AliasDeactivated, 4, 2},
		{"启用", AliasReactivated, 4, 3},
	}
	for _, c := range cases {
		m.ApplyAliasDelta(acc.ID, c.delta)
		got, _ := m.GetAccount(acc.ID)
		if got.AliasTotal != c.wantTotal || got.AliasActive != c.wantAcive {
			t.Fatalf("%s 后 total=%d active=%d, want %d/%d",
				c.name, got.AliasTotal, got.AliasActive, c.wantTotal, c.wantAcive)
		}
	}
}

// 计数不该跌到负数,也不该出现 active > total。
func TestApplyAliasDeltaClamps(t *testing.T) {
	m := newTestManager(t)
	acc, _ := m.AddAccount("t", "", "icloud.com", "")
	m.SetAliasCountsFrom(acc.ID, []hme.Alias{{Active: false}})

	// 对一个本就没激活的账号连点两次"停用"
	m.ApplyAliasDelta(acc.ID, AliasDeactivated)
	m.ApplyAliasDelta(acc.ID, AliasDeactivated)
	if got, _ := m.GetAccount(acc.ID); got.AliasActive != 0 {
		t.Fatalf("active 被压到负数: %d", got.AliasActive)
	}

	// 反过来:激活数不该超过总数
	for i := 0; i < 5; i++ {
		m.ApplyAliasDelta(acc.ID, AliasReactivated)
	}
	got, _ := m.GetAccount(acc.ID)
	if got.AliasActive > got.AliasTotal {
		t.Fatalf("active=%d 超过 total=%d", got.AliasActive, got.AliasTotal)
	}
}

func TestSetAliasCountsFromUnknownAccount(t *testing.T) {
	m := newTestManager(t)
	// 不存在的 id 应静默返回,不 panic
	m.SetAliasCountsFrom("acc_nope", []hme.Alias{{Active: true}})
	m.ApplyAliasDelta("acc_nope", AliasCreated)
}
