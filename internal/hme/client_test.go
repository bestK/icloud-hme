package hme

import (
	"fmt"
	"testing"
	"time"
)

// listBody 拼一份 /v2/hme/list 形状的响应。ms 为 0 表示上游没给创建时间。
func listBody(items ...struct {
	email  string
	ms     int64
	active bool
}) string {
	out := ""
	for i, it := range items {
		if i > 0 {
			out += ","
		}
		ts := ""
		if it.ms > 0 {
			ts = fmt.Sprintf(`,"createTimestamp":%d`, it.ms)
		}
		out += fmt.Sprintf(`{"hme":%q,"anonymousId":"id-%d","label":"l","isActive":%t%s}`,
			it.email, i, it.active, ts)
	}
	return `{"success":true,"result":{"hmeEmails":[` + out + `]}}`
}

type item = struct {
	email  string
	ms     int64
	active bool
}

// iCloud 的 createTimestamp 是毫秒时间戳。原样透传出去,接口里就会混着
// 裸数字和 RFC3339 两种时间格式。
func TestParseAliasListConvertsTimestamp(t *testing.T) {
	// 2026-08-08T14:57:07Z
	const ms = 1786201027000

	aliases := parseAliasList(listBody(item{"a@icloud.com", ms, true}))

	if len(aliases) != 1 {
		t.Fatalf("应解析出 1 个别名,got %d", len(aliases))
	}
	got := aliases[0].CreatedAt
	want := time.UnixMilli(ms).Format(time.RFC3339)
	if got != want {
		t.Errorf("createdAt 应转成 RFC3339 %q,got %q", want, got)
	}
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Errorf("createdAt 解析不了: %v", err)
	}
}

// 别名是一个个攒出来的,最常找的是刚建的那个。
func TestParseAliasListSortsNewestFirst(t *testing.T) {
	const base = 1786201027000

	aliases := parseAliasList(listBody(
		item{"old@icloud.com", base, true},
		item{"newest@icloud.com", base + 2*60_000, true},
		item{"mid@icloud.com", base + 60_000, true},
	))

	want := []string{"newest@icloud.com", "mid@icloud.com", "old@icloud.com"}
	for i, w := range want {
		if aliases[i].Email != w {
			t.Fatalf("第 %d 个应是 %s,got %s (完整顺序 %v)", i, w, aliases[i].Email, emails(aliases))
		}
	}
}

// 停用的别名不该因为状态被拎到前面 —— 列表是按时间排的,状态有单独一列。
func TestParseAliasListIgnoresActiveWhenSorting(t *testing.T) {
	const base = 1786201027000

	aliases := parseAliasList(listBody(
		item{"active-old@icloud.com", base, true},
		item{"inactive-new@icloud.com", base + 60_000, false},
	))

	if aliases[0].Email != "inactive-new@icloud.com" {
		t.Errorf("更晚创建的该排前面,即使它已停用,got %v", emails(aliases))
	}
}

// 上游偶尔不给创建时间。这些的位置本来就是猜的,沉到最后,
// 而且它们之间要有稳定顺序,不能每次刷新都变。
func TestParseAliasListPutsUndatedLast(t *testing.T) {
	const base = 1786201027000

	aliases := parseAliasList(listBody(
		item{"zzz-undated@icloud.com", 0, true},
		item{"dated@icloud.com", base, true},
		item{"aaa-undated@icloud.com", 0, true},
	))

	want := []string{"dated@icloud.com", "aaa-undated@icloud.com", "zzz-undated@icloud.com"}
	for i, w := range want {
		if aliases[i].Email != w {
			t.Fatalf("第 %d 个应是 %s,got %s (完整顺序 %v)", i, w, aliases[i].Email, emails(aliases))
		}
	}
}

func emails(aliases []Alias) []string {
	out := make([]string, len(aliases))
	for i, a := range aliases {
		out[i] = a.Email
	}
	return out
}
