// Package mail 实现 iCloud 邮件 IMAP 读取客户端。
//
// 通过 Apple 应用专用密码连接 imap.mail.me.com:993,
// 拉取隐私邮箱别名收到的邮件。对应原 Python 项目 icloud_mail.py。
package mail

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/charset"
	gomail "github.com/emersion/go-message/mail"
)

const (
	IMAPServer = "imap.mail.me.com"
	IMAPPort   = 993

	inboxName = "INBOX"

	// maxHTMLBytes 是单封邮件回传 HTML 的上限。收件箱一次给 20 封,营销邮件
	// 动辄几百 KB,不设限一个请求就能拖出几兆 JSON。超限的丢掉 HTML 只留纯文本 ——
	// 截断会把标签切断,渲染出来是半张残页,还不如不给。
	maxHTMLBytes = 512 << 10
)

// junkNames 是服务器不给 SPECIAL-USE 属性时,垃圾箱名字的兜底猜测。
var junkNames = []string{"Junk", "Spam", "Bulk Mail", "垃圾邮件"}

// Message 是一封邮件的摘要信息。
type Message struct {
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Date    string `json:"date"`
	Preview string `json:"preview"`
	// HTML 是邮件的 text/html 正文原文,供前端渲染。纯文本邮件、
	// 正文过大被丢弃、或走 Web API 那条路时为空。
	HTML string `json:"html,omitempty"`
	// Folder 是这封邮件所在的邮箱名(INBOX / Junk)。
	// 验证码常被判进垃圾箱,不标出来会让人以为收件箱里凭空多了封信。
	Folder string `json:"folder,omitempty"`
}

// FullMessage 是一封邮件的完整内容(含正文)。
type FullMessage struct {
	Message
	Body        string `json:"body"`
	ContentType string `json:"content_type"`
}

// Page 是一页邮件,外加它在完整结果集里的位置。
type Page struct {
	Messages []Message
	// Total 是整个结果集的条数,不是本页条数。Exact 为 false 时它只是
	// "至少这么多" —— Web API 那条路给不出总数,只能报已经看到的。
	Total int
	Exact bool
}

// candidate 是分页第一阶段的产物:定位和排序要用的最少信息,不含正文。
//
// 分页得先知道总数和全局顺序才能切页,而正文是整个流程里最贵的一步
// (营销邮件动辄几百 KB)。所以先只取信封排序,切出当前页,再去拉那一页
// 的正文 —— 否则翻第 1 页也要把几百封信的正文全下载一遍。
type candidate struct {
	box  string
	uid  uint32
	date time.Time
	// to 供服务端搜不动时在本地按收件人过滤
	to string
}

// maxCandidates 是单个邮箱参与分页的邮件数上限。
//
// 不限日期时一个邮箱可能有上万封,把信封全拉回来排序不值当。取最近这些
// 已经够翻很多页了,代价是 Page.Exact 会变成 false。
const maxCandidates = 2000

// slicePage 从一份已经排好序的完整列表里切出一页。
func slicePage(msgs []Message, limit, offset int, exact bool) Page {
	total := len(msgs)
	if offset >= total {
		return Page{Messages: []Message{}, Total: total, Exact: exact}
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return Page{Messages: msgs[offset:end], Total: total, Exact: exact}
}

// Client 是 iCloud 邮件 IMAP 客户端。
type Client struct {
	appleID     string
	appPassword string
	cli         *client.Client

	junk        string // 垃圾箱邮箱名,空表示这个账号没有
	junkChecked bool   // 查过一次就不再重复 LIST
}

// NewClient 创建 IMAP 客户端。需在调用其它方法前先 Connect。
func NewClient(appleID, appPassword string) *Client {
	return &Client{appleID: appleID, appPassword: appPassword}
}

// Connect 连接并登录 IMAP 服务器。
func (c *Client) Connect() error {
	addr := fmt.Sprintf("%s:%d", IMAPServer, IMAPPort)
	cli, err := client.DialTLS(addr, nil)
	if err != nil {
		return fmt.Errorf("IMAP 连接失败: %w", err)
	}
	if err := cli.Login(c.appleID, c.appPassword); err != nil {
		return fmt.Errorf("IMAP 登录失败 — 请检查: 1) 应用专用密码是否正确 2) Apple ID: %s — %w", c.appleID, err)
	}
	c.cli = cli
	return nil
}

// Disconnect 登出并关闭连接。
func (c *Client) Disconnect() {
	if c.cli != nil {
		_ = c.cli.Logout()
		c.cli = nil
	}
}

// InboxCount 返回收件箱邮件总数。
func (c *Client) InboxCount() (int, error) {
	if c.cli == nil {
		return 0, fmt.Errorf("未连接")
	}
	mbox, err := c.cli.Select("INBOX", false)
	if err != nil {
		return 0, err
	}
	return int(mbox.Messages), nil
}

// ListInbox 按页拉取收件箱和垃圾箱的邮件摘要。
//
// days 用于过滤只看近 N 天的邮件(0 表示不限制)。
// 返回按时间倒序排列。
func (c *Client) ListInbox(limit, offset, days int) (Page, error) {
	return c.page(limit, offset, days, "")
}

// FindByRecipient 按页查找发给指定隐私邮箱别名的邮件,收件箱和垃圾箱都找。
func (c *Client) FindByRecipient(recipient string, limit, offset, days int) (Page, error) {
	return c.page(limit, offset, days, recipient)
}

// page 是两个列表接口共用的分页实现。recipient 非空时只要发给它的邮件。
//
// 先把各邮箱的信封收齐排好序,再只拉当前页的正文。总数来自排序后的完整
// 候选集,所以分页器上的页数是真的 —— 不像"拉 20 封再切两页"那样,翻到
// 底了还有更早的邮件没被取过。
func (c *Client) page(limit, offset, days int, recipient string) (Page, error) {
	if c.cli == nil {
		return Page{}, fmt.Errorf("未连接")
	}
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var all []candidate
	exact := true
	for _, box := range c.mailboxes() {
		cands, truncated, err := c.candidates(box, recipient, days)
		if err != nil {
			// 垃圾箱读不到不该让整个请求失败,收件箱的信还是要给出去
			if box == inboxName {
				return Page{}, err
			}
			continue
		}
		if truncated {
			exact = false
		}
		all = append(all, cands...)
	}

	sort.SliceStable(all, func(i, j int) bool { return all[i].date.After(all[j].date) })

	total := len(all)
	if offset >= total {
		return Page{Messages: []Message{}, Total: total, Exact: exact}, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	msgs, err := c.fetchPage(all[offset:end])
	if err != nil {
		return Page{}, err
	}
	return Page{Messages: msgs, Total: total, Exact: exact}, nil
}

// mailboxes 返回要读的邮箱:收件箱 + 垃圾箱(如果有)。
//
// 别名收到的注册验证码经常被 Apple 直接判成垃圾邮件,只读 INBOX 的话
// 界面上就是"什么都没收到"。
func (c *Client) mailboxes() []string {
	boxes := []string{inboxName}
	if junk := c.findJunk(); junk != "" {
		boxes = append(boxes, junk)
	}
	return boxes
}

// findJunk 找出垃圾箱的邮箱名。
//
// 优先认 SPECIAL-USE 的 \Junk 属性,服务器不给属性时再按常见名字兜底 ——
// 名字是本地化的,不能写死。
func (c *Client) findJunk() string {
	if c.junkChecked {
		return c.junk
	}
	c.junkChecked = true

	infos := make(chan *imap.MailboxInfo, 32)
	done := make(chan error, 1)
	go func() { done <- c.cli.List("", "*", infos) }()

	byName := make(map[string]string)
	for info := range infos {
		byName[strings.ToLower(info.Name)] = info.Name
		for _, attr := range info.Attributes {
			if strings.EqualFold(attr, imap.JunkAttr) {
				c.junk = info.Name
			}
		}
	}
	if err := <-done; err != nil {
		return c.junk
	}
	if c.junk == "" {
		for _, guess := range junkNames {
			if real, ok := byName[strings.ToLower(guess)]; ok {
				c.junk = real
				break
			}
		}
	}
	return c.junk
}

// candidates 收集单个邮箱里符合条件的邮件,只取信封不取正文。
// 第二个返回值表示候选集是否因为超过 maxCandidates 被截断过。
func (c *Client) candidates(box, recipient string, days int) ([]candidate, bool, error) {
	if _, err := c.cli.Select(box, true); err != nil {
		return nil, false, err
	}

	uids, serverFiltered, err := c.searchUIDs(recipient, days)
	if err != nil {
		return nil, false, err
	}
	if len(uids) == 0 {
		return nil, false, nil
	}
	truncated := false
	if len(uids) > maxCandidates {
		uids = uids[len(uids)-maxCandidates:] // UID 递增,留最近的
		truncated = true
	}

	cands, err := c.envelopes(box, uids)
	if err != nil {
		return nil, false, err
	}
	if recipient == "" || serverFiltered {
		return cands, truncated, nil
	}

	// 服务端 TO 搜索没结果时退回本地过滤:别名可能出现在 To 或 Cc 里,
	// 服务器对 HME 地址的索引也未必可靠,宁可自己再筛一遍。
	want := strings.ToLower(recipient)
	kept := cands[:0]
	for _, cd := range cands {
		if strings.Contains(strings.ToLower(cd.to), want) {
			kept = append(kept, cd)
		}
	}
	return kept, truncated, nil
}

// searchUIDs 找出候选邮件的 UID。第二个返回值表示服务端是否已经按
// recipient 过滤过 —— 没有的话调用方得在本地再筛一遍。
func (c *Client) searchUIDs(recipient string, days int) ([]uint32, bool, error) {
	base := imap.NewSearchCriteria()
	if days > 0 {
		base.Since = time.Now().AddDate(0, 0, -days)
	}

	if recipient != "" {
		withTo := imap.NewSearchCriteria()
		withTo.Since = base.Since
		withTo.Header.Add("To", recipient)
		if uids, err := c.cli.UidSearch(withTo); err == nil && len(uids) > 0 {
			return uids, true, nil
		}
	}

	uids, err := c.cli.UidSearch(base)
	if err != nil {
		return nil, false, err
	}
	return uids, recipient == "", nil
}

// envelopes 批量取信封。这一步刻意不要正文 —— 它只服务于排序和计数。
func (c *Client) envelopes(box string, uids []uint32) ([]candidate, error) {
	seqset := new(imap.SeqSet)
	for _, uid := range uids {
		seqset.AddNum(uid)
	}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate}
	messages := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)
	go func() {
		done <- c.cli.UidFetch(seqset, items, messages)
	}()

	out := make([]candidate, 0, len(uids))
	for msg := range messages {
		m := toMessage(msg)
		cd := candidate{box: box, uid: msg.Uid, to: m.To, date: msg.InternalDate}
		// 优先用信头日期,跟列表里显示的时间保持一致;它缺失或坏掉时
		// 退回服务器收信时间,总比把这封信排到最后强。
		if t, ok := parseDate(m.Date); ok {
			cd.date = t
		}
		out = append(out, cd)
	}
	if err := <-done; err != nil {
		return nil, err
	}
	return out, nil
}

// fetchPage 把当前页这几封邮件连正文一起取回来。
//
// 按邮箱分组是因为 UID 只在单个邮箱内唯一,收件箱和垃圾箱的 UID 混进
// 同一个 FETCH 会取错信。
func (c *Client) fetchPage(cands []candidate) ([]Message, error) {
	byBox := make(map[string][]uint32)
	order := make([]string, 0, 2)
	for _, cd := range cands {
		if _, seen := byBox[cd.box]; !seen {
			order = append(order, cd.box)
		}
		byBox[cd.box] = append(byBox[cd.box], cd.uid)
	}

	got := make(map[string]Message, len(cands))
	for _, box := range order {
		msgs, err := c.fetchByUIDs(box, byBox[box])
		if err != nil {
			return nil, err
		}
		for _, m := range msgs {
			got[box+"/"+m.ID] = m
		}
	}

	// FETCH 不保证返回顺序,按排好的候选顺序还原
	out := make([]Message, 0, len(cands))
	for _, cd := range cands {
		if m, ok := got[fmt.Sprintf("%s/%d", cd.box, cd.uid)]; ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// fetchByUIDs 取指定 UID 的邮件,含完整正文。
func (c *Client) fetchByUIDs(box string, uids []uint32) ([]Message, error) {
	if len(uids) == 0 {
		return []Message{}, nil
	}
	if _, err := c.cli.Select(box, true); err != nil {
		return nil, err
	}

	seqset := new(imap.SeqSet)
	for _, uid := range uids {
		seqset.AddNum(uid)
	}

	// 拉取完整正文,以便填充 Preview(OTP 验证码在正文中)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate, section.FetchItem()}
	messages := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)
	go func() {
		done <- c.cli.UidFetch(seqset, items, messages)
	}()

	var out []Message
	for msg := range messages {
		m := toMessageWithBody(msg)
		m.Folder = box
		out = append(out, m)
	}
	if err := <-done; err != nil {
		return nil, err
	}
	return out, nil
}

// GetFull 获取单封邮件的完整内容(含正文)。
func (c *Client) GetFull(uid uint32) (*FullMessage, error) {
	if c.cli == nil {
		return nil, fmt.Errorf("未连接")
	}
	if _, err := c.cli.Select("INBOX", true); err != nil {
		return nil, err
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate, imap.FetchRFC822}
	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- c.cli.UidFetch(seqset, items, messages)
	}()

	msg := <-messages
	if err := <-done; err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, fmt.Errorf("邮件不存在 (uid=%d)", uid)
	}

	full := &FullMessage{Message: toMessageWithBody(msg)}
	if full.HTML != "" {
		full.Body, full.ContentType = full.HTML, "text/html"
	} else {
		full.Body, full.ContentType = full.Preview, "text/plain"
	}
	return full, nil
}

// ---- 时间与排序 ----

// parseDate 解析 Message.Date。IMAP 和 Web 两条路都按 RFC3339 输出。
func parseDate(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// sortNewest 把多个邮箱合并来的邮件按时间倒序排好。
func sortNewest(msgs []Message) []Message {
	if msgs == nil {
		return []Message{}
	}
	sort.SliceStable(msgs, func(i, j int) bool {
		ti, oki := parseDate(msgs[i].Date)
		tj, okj := parseDate(msgs[j].Date)
		if oki != okj {
			return oki // 有日期的排前面
		}
		return ti.After(tj)
	})
	return msgs
}

// ---- 解析工具 ----

func toMessage(msg *imap.Message) Message {
	m := Message{}
	if msg.Uid > 0 {
		m.ID = fmt.Sprintf("%d", msg.Uid)
	}
	if msg.Envelope != nil {
		if len(msg.Envelope.From) > 0 {
			m.From = msg.Envelope.From[0].Address()
		}
		if len(msg.Envelope.To) > 0 {
			addrs := make([]string, 0, len(msg.Envelope.To))
			for _, a := range msg.Envelope.To {
				addrs = append(addrs, a.Address())
			}
			m.To = strings.Join(addrs, ", ")
		}
		m.Subject = decodeHeader(msg.Envelope.Subject)
		if !msg.Envelope.Date.IsZero() {
			m.Date = msg.Envelope.Date.Format(time.RFC3339)
		}
	}
	if m.From != "" {
		m.From = decodeHeader(m.From)
	}
	if m.To != "" {
		m.To = decodeHeader(m.To)
	}
	return m
}

// toMessageWithBody 在 toMessage 基础上解析正文,填充 Preview(供 OTP 提取)和 HTML。
func toMessageWithBody(msg *imap.Message) Message {
	m := toMessage(msg)
	r := msg.GetBody(&imap.BodySectionName{})
	if r == nil {
		return m
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return m
	}

	b := readBody(raw)
	m.Preview = strings.TrimSpace(b.text)
	if m.Preview == "" {
		m.Preview = stripHTML(b.html)
	}
	if len(b.html) <= maxHTMLBytes {
		m.HTML = b.html
	}
	return m
}

// decodeHeader 解码 RFC 2047 编码的邮件头(如 =?UTF-8?B?xxx?=)。
func decodeHeader(s string) string {
	if s == "" {
		return ""
	}
	dec := mime.WordDecoder{CharsetReader: charset.Reader}
	out, err := dec.DecodeHeader(s)
	if err != nil {
		return s
	}
	return out
}

var htmlTag = regexp.MustCompile(`<[^>]+>`)

// body 是一封邮件的两份正文,视邮件构成可能只有一份。
type body struct {
	text string
	html string
}

// readBody 解析整封邮件(含头)的正文,纯文本和 HTML 各取第一份。
//
// 真实邮件几乎都是 multipart/alternative,只看顶层 Content-Type 会把 MIME
// 分隔符和 base64 原样当成正文读出来。这里走一遍分段树,顺带做掉
// Content-Transfer-Encoding 解码和字符集转换。
func readBody(raw []byte) body {
	if b, err := readMIME(raw); err == nil && (b.text != "" || b.html != "") {
		return b
	}
	// 头都解析不出来时退回整封原文 —— 难看,但验证码还在里面
	return body{text: strings.TrimSpace(string(raw))}
}

func readMIME(raw []byte) (body, error) {
	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return body{}, err
	}
	defer mr.Close()

	var out body
	for {
		p, err := mr.NextPart()
		if err != nil {
			// io.EOF 是正常读完;某个分段坏掉时也就此打住,
			// 前面已经读到的正文不该跟着一起丢
			break
		}
		h, ok := p.Header.(*gomail.InlineHeader)
		if !ok {
			continue // 附件
		}
		ct, _, err := h.ContentType()
		if err != nil {
			continue
		}
		// 各取第一份:multipart/related 里同一封信可能有多个 text/html 片段
		if (ct == "text/plain" && out.text != "") || (ct == "text/html" && out.html != "") {
			continue
		}
		data, err := io.ReadAll(p.Body)
		if err != nil {
			continue
		}
		switch ct {
		case "text/plain":
			out.text = string(data)
		case "text/html":
			out.html = string(data)
		}
	}
	return out, nil
}

// stripHTML 粗略剥离 HTML 标签,保留可读文本。
func stripHTML(html string) string {
	// 换行标签转换行
	html = strings.ReplaceAll(html, "<br>", "\n")
	html = strings.ReplaceAll(html, "<br/>", "\n")
	html = strings.ReplaceAll(html, "<br />", "\n")
	html = strings.ReplaceAll(html, "</p>", "\n")
	html = strings.ReplaceAll(html, "</div>", "\n")
	html = strings.ReplaceAll(html, "</tr>", "\n")
	html = strings.ReplaceAll(html, "<li>", "\n- ")
	// 去掉所有标签
	html = htmlTag.ReplaceAllString(html, "")
	// 反转义常见实体
	html = strings.ReplaceAll(html, "&nbsp;", " ")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	// 压缩多余空白
	lines := strings.Split(html, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
