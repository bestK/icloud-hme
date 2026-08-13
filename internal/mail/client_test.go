package mail

import (
	"strings"
	"testing"
)

// 绝大多数真实邮件是 multipart/alternative。之前只看顶层 Content-Type,
// 这类信会把 MIME 分隔符和编码后的字节当正文读出来。
func TestReadBodySplitsMultipartAlternative(t *testing.T) {
	raw := crlf(`From: no-reply@example.com
To: alias@icloud.com
Subject: Your code
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="b1"

--b1
Content-Type: text/plain; charset=utf-8

Your code is 123456
--b1
Content-Type: text/html; charset=utf-8
Content-Transfer-Encoding: quoted-printable

<html><body><b>Your code is 123456</b> =E2=9C=93</body></html>
--b1--
`)

	b := readBody([]byte(raw))

	if !strings.Contains(b.text, "Your code is 123456") {
		t.Errorf("纯文本分段没取到,got %q", b.text)
	}
	if strings.Contains(b.text, "--b1") {
		t.Errorf("纯文本里混进了 MIME 分隔符,got %q", b.text)
	}
	if !strings.Contains(b.html, "<b>Your code is 123456</b>") {
		t.Errorf("HTML 分段没取到,got %q", b.html)
	}
	// quoted-printable 应该已经解开,前端拿到的不该是 =E2=9C=93
	if !strings.Contains(b.html, "✓") {
		t.Errorf("quoted-printable 没解码,got %q", b.html)
	}
}

// 只有 HTML 的信也要给出 HTML,并且能降级出纯文本供 OTP 提取。
func TestReadBodyHTMLOnly(t *testing.T) {
	raw := crlf(`From: no-reply@example.com
Subject: hi
MIME-Version: 1.0
Content-Type: text/html; charset=utf-8

<html><body><p>code 998877</p></body></html>
`)

	b := readBody([]byte(raw))

	if b.text != "" {
		t.Errorf("这封信没有 text/plain 分段,不该凭空造一份,got %q", b.text)
	}
	if !strings.Contains(b.html, "<p>code 998877</p>") {
		t.Errorf("HTML 没取到,got %q", b.html)
	}
	if got := stripHTML(b.html); !strings.Contains(got, "code 998877") {
		t.Errorf("HTML 剥不出纯文本,got %q", got)
	}
}

// 纯文本信不该带 HTML —— 前端据此决定要不要显示渲染视图。
func TestReadBodyPlainOnly(t *testing.T) {
	raw := crlf(`From: no-reply@example.com
Subject: hi

just text
`)

	b := readBody([]byte(raw))

	if !strings.Contains(b.text, "just text") {
		t.Errorf("正文没取到,got %q", b.text)
	}
	if b.html != "" {
		t.Errorf("纯文本信不该有 HTML,got %q", b.html)
	}
}

// 头坏掉时宁可把整封原文当正文交出去,也不能返回空 —— 验证码还在里面。
func TestReadBodyFallsBackOnGarbage(t *testing.T) {
	b := readBody([]byte("code 424242"))

	if !strings.Contains(b.text, "424242") {
		t.Errorf("解析失败时该退回原文,got %q", b.text)
	}
}

// 邮件头按 RFC 5322 用 CRLF 分行,LF 换行的原文喂不进 MIME 解析器。
func crlf(s string) string {
	return strings.ReplaceAll(s, "\n", "\r\n")
}

func msgs(n int) []Message {
	out := make([]Message, n)
	for i := range out {
		out[i] = Message{ID: string(rune('a' + i))}
	}
	return out
}

// Total 报的必须是结果集总数而不是本页条数 —— 前端拿它算页数,
// 报成本页条数就又回到"永远只有一页"的假分页。
func TestSlicePageReportsWholeSetTotal(t *testing.T) {
	p := slicePage(msgs(57), 10, 0, true)

	if p.Total != 57 {
		t.Errorf("Total 应是结果集总数 57,got %d", p.Total)
	}
	if len(p.Messages) != 10 {
		t.Errorf("首页应有 10 条,got %d", len(p.Messages))
	}
	if p.Messages[0].ID != "a" {
		t.Errorf("首页该从头开始,got %q", p.Messages[0].ID)
	}
}

// 最后一页通常不满,不能因为凑不够 limit 就越界。
func TestSlicePageLastPagePartial(t *testing.T) {
	p := slicePage(msgs(25), 10, 20, true)

	if len(p.Messages) != 5 {
		t.Errorf("最后一页应剩 5 条,got %d", len(p.Messages))
	}
	if p.Total != 25 {
		t.Errorf("Total 不该受切页影响,got %d", p.Total)
	}
}

// 邮件是会变的:翻到第 3 页时前面的信可能已经被删了。
// offset 落在总数之外要给空页,不能 panic。
func TestSlicePageOffsetBeyondTotal(t *testing.T) {
	p := slicePage(msgs(5), 10, 50, true)

	if len(p.Messages) != 0 {
		t.Errorf("越界的 offset 应返回空页,got %d 条", len(p.Messages))
	}
	if p.Messages == nil {
		t.Error("空页也要是空切片,nil 会被序列化成 null")
	}
	if p.Total != 5 {
		t.Errorf("Total 仍应是 5,got %d", p.Total)
	}
}

// Web API 那条路给不出真实总数,Exact 必须如实传下去。
func TestSlicePageKeepsExactFlag(t *testing.T) {
	if p := slicePage(msgs(3), 10, 0, false); p.Exact {
		t.Error("Exact=false 不该被改写成 true")
	}
	if p := slicePage(msgs(3), 10, 0, true); !p.Exact {
		t.Error("Exact=true 不该被改写成 false")
	}
}
