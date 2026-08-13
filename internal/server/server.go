// Package server 提供 HTTP API,基于 Gin。
//
// 两个核心接口:
//   POST /api/create  — 在指定账号下创建一个 Hide My Email 别名
//   GET  /api/inbox   — 读取指定账号(或指定别名)收到的邮件
//
// 辅助接口(用于多账号管理):账号增删查、别名列表、设置 App 密码。
package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"icloud-hme/internal/account"
	"icloud-hme/internal/hme"
	"icloud-hme/internal/mail"
	"icloud-hme/internal/pool"
	"icloud-hme/internal/token"
	"icloud-hme/internal/web"
)

// Server 封装 Gin 引擎、账号管理器、token 存储和暖池。
type Server struct {
	mgr    *account.Manager
	tokens *token.Store
	pool   *pool.Store
	filler *pool.Filler
	r      *gin.Engine

	// 等待 2FA 验证码的登录。密码那一步和验证码那一步之间必须复用同一个
	// Apple 会话,所以只能把它挂在内存里(进程重启后作废,重新登录即可)。
	loginsMu sync.Mutex
	logins   map[string]*pendingLogin
}

// New 创建 Server。debug 为 true 时启用 Gin 调试日志。
func New(mgr *account.Manager, tokens *token.Store, poolStore *pool.Store, filler *pool.Filler, debug bool) *Server {
	if !debug {
		gin.SetMode(gin.ReleaseMode)
	}
	s := &Server{
		mgr:    mgr,
		tokens: tokens,
		pool:   poolStore,
		filler: filler,
		logins: make(map[string]*pendingLogin),
	}
	s.r = gin.Default() // 自带 Logger + Recovery 中间件
	s.register()
	web.Attach(s.r) // 挂管理面板静态资源(embed)
	return s
}

// Run 启动 HTTP 服务。
func (s *Server) Run(addr string) error {
	return s.r.Run(addr)
}

// Handler 返回底层 gin 引擎(便于测试)。
func (s *Server) Handler() http.Handler { return s.r }

func (s *Server) register() {
	api := s.r.Group("/api", s.authMiddleware())

	// admin 独占接口
	adm := api.Group("", requireAdmin())
	adm.GET("/accounts", s.listAccounts)
	adm.POST("/accounts", s.addAccount)
	adm.DELETE("/accounts/:id", s.removeAccount)
	adm.POST("/accounts/:id/password", s.setAppPassword)
	adm.PUT("/accounts/:id/cookies", s.updateCookies)
	adm.POST("/accounts/:id/login", s.loginAccount)
	adm.POST("/accounts/:id/login/verify", s.verifyLogin)
	adm.POST("/accounts/:id/revalidate", s.revalidateAccount)
	adm.POST("/reload", s.reloadConfig)
	adm.GET("/tokens", s.listTokens)
	adm.POST("/tokens", s.createToken)
	adm.DELETE("/tokens/:id", s.deleteToken)
	adm.GET("/pool", s.listPool)
	adm.GET("/pool/filler", s.fillerStatus)

	// admin + user 都能调,内部按 role 做数据隔离
	api.POST("/create", s.createAlias)
	api.GET("/inbox", s.listInbox)
	api.GET("/aliases", s.listAliases)
	api.POST("/aliases/:id/deactivate", s.deactivateAlias)
	api.POST("/aliases/:id/reactivate", s.reactivateAlias)
	api.DELETE("/aliases/:id", s.deleteAlias)
}

// ---- 统一响应 ----

type apiResp struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	// Scope 标明是谁的凭证不行:缺省是面板自己的 token,"upstream" 是 iCloud 会话。
	// 两者都会返 401,前端靠它决定该把用户踢回登录页,还是只报个错。
	Scope string `json:"scope,omitempty"`
	// UpstreamStatus 是 iCloud 返回的原始状态码。我们对外的状态码是映射过的
	// (比如 421 → 401),排查时需要看到没被改写的那个。
	UpstreamStatus int `json:"upstream_status,omitempty"`
}

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, apiResp{Success: true, Data: data})
}

func fail(c *gin.Context, code int, msg string) {
	c.JSON(code, apiResp{Success: false, Message: msg})
}

func failWithUpstream(c *gin.Context, code, upstream int, msg string) {
	c.JSON(code, apiResp{Success: false, Message: msg, Scope: "upstream", UpstreamStatus: upstream})
}

// failUpstream 把 iCloud 上游返回的错误统一映射为 HTTP 状态码:
//   - 会话失效(401/403/421) → 401
//   - 限流(上游 429 或 hme.IsRateLimit) → 429
//   - 其他上游错误 → 502
//
// 分档优先看 hme.UpstreamError 里的状态码;拿不到(比如 IMAP / Web 邮件客户端
// 自己拼的错误)才退回关键字匹配。421 不原样透传:它在 HTTP/2 里有连接层语义,
// 浏览器可能拿它去换条连接重试,原始码放在 upstream_status 里。
//
// prefix 用于给用户提供操作上下文,比如"创建邮箱失败"。
func failUpstream(c *gin.Context, prefix string, err error) {
	msg := err.Error()
	upstream := upstreamStatusOf(err)

	switch {
	case hme.SessionExpired(err) || (upstream == 0 && isSessionError(msg)):
		failWithUpstream(c, http.StatusUnauthorized, upstream,
			prefix+": iCloud 会话失效,需要重新登录换 Cookie — "+msg)
	case upstream == http.StatusTooManyRequests || hme.IsRateLimit(msg):
		failWithUpstream(c, http.StatusTooManyRequests, upstream,
			prefix+": iCloud 限流,请稍后重试 — "+msg)
	default:
		failWithUpstream(c, http.StatusBadGateway, upstream, prefix+": "+msg)
	}
}

// upstreamStatusOf 从错误链里取出上游 HTTP 状态码,取不到返回 0。
// iCloud 数据接口的 UpstreamError 和 Apple 认证接口的 AuthError 都实现了它。
func upstreamStatusOf(err error) int {
	var coded interface{ UpstreamStatus() int }
	if errors.As(err, &coded) {
		return coded.UpstreamStatus()
	}
	return 0
}

// failAuth 处理密码登录过程中的失败。
//
// 登录失败和会话失效是两件事。后者的处理办法是「重新登录」,而这里用户正在
// 登录,再让他去「重新登录换 Cookie」就是一句废话 —— 所以认证错误必须先拦下来,
// 不能落进 failUpstream 那套按状态码猜的分档里。
func failAuth(c *gin.Context, prefix string, err error) {
	var ae *hme.AuthError
	if errors.As(err, &ae) {
		failWithUpstream(c, http.StatusUnauthorized, ae.Status, prefix+": "+ae.Error())
		return
	}
	failUpstream(c, prefix, err)
}

// ====================================================================
// 核心接口 1: 创建邮箱
//   POST /api/create
//   body: {"account_id": "acc_xxx", "label": "可选标签"}
//   返回: 新创建的 HME 邮箱地址
// ====================================================================

type createReq struct {
	AccountID string `json:"account_id" binding:"required"`
	Label     string `json:"label"`
}

func (s *Server) createAlias(c *gin.Context) {
	var req createReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: account_id 必填 — "+err.Error())
		return
	}

	// 优先从暖池 pop
	if s.pool != nil {
		if entry, hit := s.pool.Pop(req.AccountID); hit {
			label := req.Label
			if label == "" {
				label = entry.Label
			}
			ref := token.AliasRef{
				AnonymousID: entry.AnonymousID,
				Email:       entry.Email,
				Label:       label,
				AccountID:   req.AccountID,
				CreatedAt:   time.Now(),
			}
			if tok := currentToken(c); tok != nil {
				_ = s.tokens.BindAlias(tok.ID, ref)
			}
			// 这里不动账号别名计数:池里的条目在 filler 预建时就已经
			// 在 iCloud 侧创建并计过数了,Pop 只是把它交出去。
			ok(c, gin.H{
				"email":        entry.Email,
				"anonymous_id": entry.AnonymousID,
				"label":        label,
				"created_at":   time.Now().Format(time.RFC3339),
				"account_id":   req.AccountID,
				"source":       "pool",
			})
			return
		}
	}

	client, err := s.mgr.HMEClient(req.AccountID, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}

	result, err := client.CreateAlias(req.Label, 5)

	// 操作完成后,保存可能已刷新的 Cookie（validate 会轮换 token）
	_ = s.mgr.SaveCookies(req.AccountID, client.Cookies)

	if err != nil {
		failUpstream(c, "创建邮箱失败", err)
		return
	}

	// 记录归属:非 admin 一定要绑;admin 也绑一份,便于统计
	if tok := currentToken(c); tok != nil {
		ref := token.AliasRef{
			AnonymousID: result.AnonymousID,
			Email:       result.Email,
			Label:       result.Label,
			AccountID:   req.AccountID,
			CreatedAt:   time.Now(),
		}
		_ = s.tokens.BindAlias(tok.ID, ref)
	}

	// 实时申请这条路径确实新增了一个上游别名,计入账号统计
	s.mgr.ApplyAliasDelta(req.AccountID, account.AliasCreated)

	// 也记进补池的小时账本。这条路径不受配额拦截(不能让用户的请求失败),
	// 但它跟补池打的是同一个上游,不记的话补池会以为这一小时很闲、继续按满速
	// 补,两边叠加出来的频率恰好在需求高峰时冲最高。记上之后补池会自动让路。
	if s.pool != nil {
		s.pool.RecordUsage(req.AccountID)
	}

	ok(c, gin.H{
		"email":        result.Email,
		"anonymous_id": result.AnonymousID,
		"label":        result.Label,
		"created_at":   result.CreatedAt,
		"account_id":   req.AccountID,
		"source":       "live",
	})
}

// ====================================================================
// 核心接口 2: 读取邮件
//   GET /api/inbox?account_id=acc_xxx[&alias=xxx@icloud.com][&limit=20][&days=7]
//
//   - 不传 alias: 返回该账号收件箱最近邮件
//   - 传 alias:   只返回发给该 HME 别名的邮件
//
//   认证优先级: IMAP (App Password) 优先 > Web API (Cookie) 回退
//   - IMAP: 支持服务端按收件人搜索 (FindByRecipient)
//   - Web API: 不支持收件人搜索,拉取收件箱后本地按别名过滤 (FindByAlias)
// ====================================================================

func (s *Server) listInbox(c *gin.Context) {
	accountID := c.Query("account_id")
	if accountID == "" {
		fail(c, http.StatusBadRequest, "参数缺失: account_id")
		return
	}
	alias := strings.TrimSpace(c.Query("alias"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))

	if !isAdmin(c) {
		if alias == "" {
			fail(c, http.StatusBadRequest, "非 admin 必须指定 alias")
			return
		}
		if !s.tokens.HasAliasEmail(currentToken(c).ID, alias) {
			fail(c, http.StatusNotFound, "别名不存在")
			return
		}
	}

	// 优先使用 IMAP (App Password 认证)
	mc, err := s.mgr.MailClient(accountID)
	if err == nil {
		if connErr := mc.Connect(); connErr == nil {
			defer mc.Disconnect()
			var messages []mail.Message
			if alias != "" {
				messages, err = mc.FindByRecipient(alias, limit, days)
			} else {
				messages, err = mc.ListInbox(limit, days)
			}
			if err == nil {
				ok(c, gin.H{
					"account_id": accountID,
					"alias":      alias,
					"count":      len(messages),
					"messages":   messages,
					"method":     "imap",
				})
				return
			}
			// IMAP 失败，继续尝试 Web API
		}
	}

	// 回退到 Web API (Cookie 认证，无需 App Password)
	wmc, err := s.mgr.WebMailClient(accountID)
	if err != nil {
		fail(c, http.StatusBadRequest, "无可用邮件客户端: 需要 App Password 或 Cookie")
		return
	}

	if alias != "" {
		messages, err := wmc.FindByAlias(alias, limit)
		if err != nil {
			failUpstream(c, "读取邮件失败", err)
			return
		}
		ok(c, gin.H{
			"account_id": accountID,
			"alias":      alias,
			"count":      len(messages),
			"messages":   messages,
			"method":     "web_api",
		})
	} else {
		messages, err := wmc.ListInbox(limit)
		if err != nil {
			failUpstream(c, "读取邮件失败", err)
			return
		}
		ok(c, gin.H{
			"account_id": accountID,
			"count":      len(messages),
			"messages":   messages,
			"method":     "web_api",
		})
	}
}

// ====================================================================
// 辅助接口
// ====================================================================

func (s *Server) listAccounts(c *gin.Context) {
	ok(c, s.mgr.ListAccounts())
}

type addAccountReq struct {
	Name     string `json:"name" binding:"required"`
	Cookies  string `json:"cookies"` // 可选,后续可通过 /login 获取
	Host     string `json:"host"`
	Proxy    string `json:"proxy"` // HTTP/SOCKS5 代理
}

func (s *Server) addAccount(c *gin.Context) {
	var req addAccountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: name 必填 — "+err.Error())
		return
	}
	acc, err := s.mgr.AddAccount(req.Name, req.Cookies, req.Host, req.Proxy)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	// 返回时脱敏
	acc.Cookies = nil
	c.JSON(http.StatusCreated, apiResp{Success: true, Data: acc})
}

func (s *Server) removeAccount(c *gin.Context) {
	id := c.Param("id")
	if !s.mgr.RemoveAccount(id) {
		fail(c, http.StatusNotFound, "账号不存在")
		return
	}
	ok(c, gin.H{"id": id})
}

type setPwdReq struct {
	ICloudEmail string `json:"icloud_email" binding:"required"`
	AppPassword string `json:"app_password" binding:"required"`
}

func (s *Server) setAppPassword(c *gin.Context) {
	id := c.Param("id")
	var req setPwdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: icloud_email, app_password 必填 — "+err.Error())
		return
	}
	if err := s.mgr.SetAppPassword(id, req.ICloudEmail, req.AppPassword); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"id": id, "icloud_email": req.ICloudEmail})
}

type updateCookiesReq struct {
	Cookies map[string]string `json:"cookies" binding:"required"`
}

func (s *Server) updateCookies(c *gin.Context) {
	id := c.Param("id")
	var req updateCookiesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: cookies 必填 — "+err.Error())
		return
	}
	if err := s.mgr.UpdateCookies(id, req.Cookies); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"id": id, "cookies_count": len(req.Cookies)})
}

// ====================================================================
// 密码登录:两步
//
//   POST /accounts/:id/login         body: {"password": "..."}
//     → 无 2FA: {"status":"ok", ...}
//     → 有 2FA: {"status":"needs_2fa","login_id":"..."} ,验证码此时已发到设备
//   POST /accounts/:id/login/verify  body: {"login_id":"...","code":"123456"}
//     → {"status":"ok", ...}
//
// 为什么必须分两步:验证码是 Apple 在密码通过之后才发的,用户不可能在提交密码
// 之前就拿到它。而且第二步必须复用第一步那个 Apple 会话 —— 重新走一遍密码流程
// 会让 Apple 重发一个新码,用户手上的码当场作废。
// ====================================================================

// pendingLoginTTL 是待验证登录在服务端的存活时间。
// 每个待验证会话占着一个 TLS 客户端,不能无限留。
const pendingLoginTTL = 5 * time.Minute

type pendingLogin struct {
	accountID string
	pending   *hme.PendingLogin
	expiresAt time.Time
}

// putPendingLogin 存一个待验证登录,返回给客户端的句柄,顺手清掉过期的。
func (s *Server) putPendingLogin(accountID string, p *hme.PendingLogin) string {
	loginID := uuid.New().String()
	now := time.Now()

	s.loginsMu.Lock()
	defer s.loginsMu.Unlock()
	for k, v := range s.logins {
		// 同一个账号只保留最新那次:旧会话里的码已经被新的一次登录顶掉了
		if v.expiresAt.Before(now) || v.accountID == accountID {
			delete(s.logins, k)
		}
	}
	s.logins[loginID] = &pendingLogin{
		accountID: accountID,
		pending:   p,
		expiresAt: now.Add(pendingLoginTTL),
	}
	return loginID
}

// takePendingLogin 取出并移除待验证登录。取不到说明超时了或者句柄对不上账号。
func (s *Server) takePendingLogin(accountID, loginID string) (*hme.PendingLogin, bool) {
	s.loginsMu.Lock()
	defer s.loginsMu.Unlock()
	entry, ok := s.logins[loginID]
	if !ok || entry.accountID != accountID {
		return nil, false
	}
	delete(s.logins, loginID)
	if entry.expiresAt.Before(time.Now()) {
		return nil, false
	}
	return entry.pending, true
}

type loginReq struct {
	Password string `json:"password" binding:"required"`
}

func (s *Server) loginAccount(c *gin.Context) {
	id := c.Param("id")
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: password 必填 — "+err.Error())
		return
	}

	res, err := s.mgr.LoginStart(id, req.Password)
	if err != nil {
		failAuth(c, "登录失败", err)
		return
	}
	if res.Pending != nil {
		ok(c, gin.H{
			"id":         id,
			"status":     "needs_2fa",
			"login_id":   s.putPendingLogin(id, res.Pending),
			"apple_id":   res.Pending.Username(),
			"expires_in": int(pendingLoginTTL.Seconds()),
		})
		return
	}
	ok(c, loginDone(id, res))
}

type verifyLoginReq struct {
	LoginID string `json:"login_id" binding:"required"`
	Code    string `json:"code" binding:"required"`
}

func (s *Server) verifyLogin(c *gin.Context) {
	id := c.Param("id")
	var req verifyLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: login_id, code 必填 — "+err.Error())
		return
	}

	pending, found := s.takePendingLogin(id, req.LoginID)
	if !found {
		fail(c, http.StatusGone, "登录会话已失效(超过 "+strconv.Itoa(int(pendingLoginTTL.Minutes()))+" 分钟或已用过),请重新输密码发起登录")
		return
	}

	res, err := s.mgr.LoginFinish(id, pending, strings.TrimSpace(req.Code))
	if err != nil {
		failAuth(c, "验证失败", err)
		return
	}
	ok(c, loginDone(id, res))
}

// loginDone 组装登录完成的响应。不回传 Cookie 明文 —— 前端不需要,
// 回传只是让一份完整的 iCloud 会话多走一趟网络。
func loginDone(id string, res *account.LoginResult) gin.H {
	return gin.H{
		"id":            id,
		"status":        "ok",
		"cookies_count": res.CookieCount,
		"validated":     res.Validated,
		"warning":       res.Warning,
	}
}

// revalidateAccount 重新校验会话并把别名计数拉平到上游真实值。
func (s *Server) revalidateAccount(c *gin.Context) {
	id := c.Param("id")
	acc, err := s.mgr.Revalidate(id)
	if err != nil {
		failUpstream(c, "重新校验失败", err)
		return
	}
	ok(c, gin.H{
		"id":               acc.ID,
		"status":           acc.Status,
		"alias_total":      acc.AliasTotal,
		"alias_active":     acc.AliasActive,
		"alias_counted_at": acc.AliasCountedAt,
		"last_validated":   acc.LastValidated,
	})
}

func (s *Server) listAliases(c *gin.Context) {
	accountID := c.Query("account_id")
	if accountID == "" {
		fail(c, http.StatusBadRequest, "参数缺失: account_id")
		return
	}
	client, err := s.mgr.HMEClient(accountID, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	aliases, err := client.ListAliases()
	_ = s.mgr.SaveCookies(accountID, client.Cookies)
	if err != nil {
		failUpstream(c, "拉取别名列表失败", err)
		return
	}
	// 这里拿到的是全量列表,先校准计数再按 token 过滤 —— 顺序反了就会把
	// 某个 token 名下的子集大小写成账号总数。
	s.mgr.SetAliasCountsFrom(accountID, aliases)

	if !isAdmin(c) {
		aliases = filterAliasesByToken(aliases, s.tokens, currentToken(c).ID)
	}
	ok(c, gin.H{
		"account_id": accountID,
		"count":      len(aliases),
		"aliases":    aliases,
	})
}

// filterAliasesByToken 保留 token 名下的别名(按 anonymousId 或 email 匹配)。
func filterAliasesByToken(aliases []hme.Alias, store *token.Store, tokenID string) []hme.Alias {
	owned := make(map[string]bool)
	ownedEmails := make(map[string]bool)
	for _, r := range store.AliasesOf(tokenID) {
		if r.AnonymousID != "" {
			owned[r.AnonymousID] = true
		}
		if r.Email != "" {
			ownedEmails[r.Email] = true
		}
	}
	out := aliases[:0]
	for _, a := range aliases {
		if owned[a.AnonymousID] || ownedEmails[a.Email] {
			out = append(out, a)
		}
	}
	return out
}

type aliasActionReq struct {
	AccountID string `json:"account_id" binding:"required"`
}

func (s *Server) deactivateAlias(c *gin.Context) {
	anonymousID := c.Param("id")
	var req aliasActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: account_id 必填 — "+err.Error())
		return
	}
	if !isAdmin(c) && !s.tokens.HasAlias(currentToken(c).ID, anonymousID) {
		fail(c, http.StatusNotFound, "别名不存在")
		return
	}

	client, err := s.mgr.HMEClient(req.AccountID, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}

	success, err := client.DeactivateHME(anonymousID)
	_ = s.mgr.SaveCookies(req.AccountID, client.Cookies)
	if err != nil {
		failUpstream(c, "停用失败", err)
		return
	}
	// 总数不变,激活数 -1
	s.mgr.ApplyAliasDelta(req.AccountID, account.AliasDeactivated)
	ok(c, gin.H{"anonymous_id": anonymousID, "success": success})
}

func (s *Server) reactivateAlias(c *gin.Context) {
	anonymousID := c.Param("id")
	var req aliasActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: account_id 必填 — "+err.Error())
		return
	}
	if !isAdmin(c) && !s.tokens.HasAlias(currentToken(c).ID, anonymousID) {
		fail(c, http.StatusNotFound, "别名不存在")
		return
	}

	client, err := s.mgr.HMEClient(req.AccountID, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}

	success, err := client.ReactivateHME(anonymousID)
	_ = s.mgr.SaveCookies(req.AccountID, client.Cookies)
	if err != nil {
		failUpstream(c, "激活失败", err)
		return
	}
	// 总数不变,激活数 +1
	s.mgr.ApplyAliasDelta(req.AccountID, account.AliasReactivated)
	ok(c, gin.H{"anonymous_id": anonymousID, "success": success})
}

func (s *Server) deleteAlias(c *gin.Context) {
	anonymousID := c.Param("id")
	var req aliasActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: account_id 必填 — "+err.Error())
		return
	}
	tok := currentToken(c)
	if !isAdmin(c) && !s.tokens.HasAlias(tok.ID, anonymousID) {
		fail(c, http.StatusNotFound, "别名不存在")
		return
	}

	client, err := s.mgr.HMEClient(req.AccountID, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}

	if err := client.Delete(anonymousID); err != nil {
		_ = s.mgr.SaveCookies(req.AccountID, client.Cookies)
		failUpstream(c, "删除失败", err)
		return
	}
	_ = s.mgr.SaveCookies(req.AccountID, client.Cookies)
	// 成功后从归属记录里移除(所有 token 都扫一遍,admin 也会解绑对应记录)
	for _, t := range s.tokens.List() {
		_ = s.tokens.UnbindAlias(t.ID, anonymousID)
	}
	// 删除走精确刷新而不是 -1:这里不知道被删的那条原本是否激活,
	// 猜错就会让 AliasActive 长期偏移。删除本来就慢且少,多一次列表请求可以接受。
	_, _, _ = s.mgr.RefreshAliasCounts(req.AccountID, client)
	ok(c, gin.H{"anonymous_id": anonymousID})
}

// isSessionError 按关键字判断错误是否由会话失效引起。
// 只在拿不到上游状态码时用 —— 响应体里也可能出现 401 这种数字,会误判。
func isSessionError(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "401") || strings.Contains(m, "403") ||
		strings.Contains(m, "session") || strings.Contains(m, "cookie") ||
		strings.Contains(m, "unauthorized") || strings.Contains(m, "认证") ||
		strings.Contains(m, "会话校验失败")
}

// reloadConfig 重新加载 accounts.json 配置文件。
func (s *Server) reloadConfig(c *gin.Context) {
	if err := s.mgr.Reload(); err != nil {
		fail(c, http.StatusInternalServerError, "重新加载配置失败: "+err.Error())
		return
	}
	ok(c, gin.H{"message": "配置已重新加载"})
}

// 确保 hme 包被引用(类型在 handler 中使用)
var _ = hme.Alias{}

// ====================================================================
// Token 管理接口 (admin only)
// ====================================================================

type tokenView struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Role       string    `json:"role"`
	AliasCount int       `json:"alias_count"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
}

func toView(t *token.Token) tokenView {
	return tokenView{
		ID:         t.ID,
		Name:       t.Name,
		Role:       string(t.Role),
		AliasCount: len(t.Aliases),
		CreatedAt:  t.CreatedAt,
		LastUsedAt: t.LastUsedAt,
	}
}

func (s *Server) listTokens(c *gin.Context) {
	all := s.tokens.List()
	out := make([]tokenView, 0, len(all))
	for _, t := range all {
		out = append(out, toView(t))
	}
	ok(c, out)
}

type createTokenReq struct {
	Name string `json:"name" binding:"required"`
	Role string `json:"role"`
}

func (s *Server) createToken(c *gin.Context) {
	var req createTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: name 必填 — "+err.Error())
		return
	}
	role := token.RoleUser
	if req.Role == string(token.RoleAdmin) {
		fail(c, http.StatusBadRequest, "不允许通过 API 创建 admin token(用 ADMIN_TOKEN 环境变量)")
		return
	}
	tk, err := s.tokens.Add(req.Name, role)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	// 创建返回包含 secret 明文(仅这一次)
	ok(c, gin.H{
		"id":         tk.ID,
		"name":       tk.Name,
		"role":       string(tk.Role),
		"secret":     tk.Secret,
		"created_at": tk.CreatedAt,
	})
}

func (s *Server) deleteToken(c *gin.Context) {
	id := c.Param("id")
	if err := s.tokens.Delete(id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"id": id})
}

// ====================================================================
// 暖池观测接口 (admin only)
// ====================================================================

type poolView struct {
	AccountID string `json:"account_id"`
	// AccountName / AccountStatus:光给 acc_xxxxxxxx 看不出这是哪个账号,
	// 状态也决定了补池会不会跳过它
	AccountName   string `json:"account_name,omitempty"`
	AccountStatus string `json:"account_status,omitempty"`
	Depth         int    `json:"depth"`
	// Target 是最低保障水位,深度会一路涨过它,不是进度条的分母
	Target    int `json:"target"`
	HourUsed  int `json:"hour_used"`
	HourlyMax int `json:"hourly_max"`
	// AliasTotal / AliasCap:补池一直囤到 Apple 的别名上限为止,所以真正的
	// 分母是这个。AliasCounted 为 false 时 AliasTotal 是"没核对过",不是 0 个。
	AliasTotal   int  `json:"alias_total"`
	AliasCounted bool `json:"alias_counted"`
	AliasCap     int  `json:"alias_cap"`
}

func (s *Server) listPool(c *gin.Context) {
	if s.pool == nil {
		ok(c, []poolView{})
		return
	}
	target := 0
	hourlyMax := 0
	if s.filler != nil {
		target = s.filler.Target()
		hourlyMax = s.filler.HourlyMax()
	}
	depths := s.pool.AllDepths()
	// 也把没有池但存在的账号显示出来
	accs := s.mgr.ListAccounts()
	seen := make(map[string]bool, len(depths))
	out := make([]poolView, 0)
	for _, acc := range accs {
		seen[acc.ID] = true
		out = append(out, poolView{
			AccountID:     acc.ID,
			AccountName:   acc.Name,
			AccountStatus: acc.Status,
			Depth:         depths[acc.ID],
			Target:        target,
			HourUsed:      s.pool.HourUsage(acc.ID),
			HourlyMax:     hourlyMax,
			AliasTotal:    acc.AliasTotal,
			AliasCounted:  acc.AliasCountedAt != "",
			AliasCap:      pool.AliasHardCap,
		})
	}
	// 处理已经删除但池里还残留的账号(异常情况)
	for id, depth := range depths {
		if seen[id] {
			continue
		}
		out = append(out, poolView{
			AccountID: id,
			Depth:     depth,
			Target:    target,
			HourUsed:  s.pool.HourUsage(id),
			HourlyMax: hourlyMax,
			AliasCap:  pool.AliasHardCap,
		})
	}
	ok(c, out)
}

// fillerStatus 汇报定时补池的运行情况:开没开、上一轮几点跑的补了几个、
// 下一轮几点跑。池深度自己会变,但看不到调度器就没法判断它是在正常工作
// 还是早就停了。
func (s *Server) fillerStatus(c *gin.Context) {
	if s.filler == nil {
		ok(c, pool.Status{})
		return
	}
	ok(c, s.filler.Status())
}
