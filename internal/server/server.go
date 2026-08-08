// Package server 提供 HTTP API,基于 Gin。
//
// 两个核心接口:
//   POST /api/create  — 在指定账号下创建一个 Hide My Email 别名
//   GET  /api/inbox   — 读取指定账号(或指定别名)收到的邮件
//
// 辅助接口(用于多账号管理):账号增删查、别名列表、设置 App 密码。
package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"icloud-hme/internal/account"
	"icloud-hme/internal/hme"
	"icloud-hme/internal/mail"
	"icloud-hme/internal/token"
)

// Server 封装 Gin 引擎、账号管理器与 token 存储。
type Server struct {
	mgr    *account.Manager
	tokens *token.Store
	r      *gin.Engine
}

// New 创建 Server。debug 为 true 时启用 Gin 调试日志。
func New(mgr *account.Manager, tokens *token.Store, debug bool) *Server {
	if !debug {
		gin.SetMode(gin.ReleaseMode)
	}
	s := &Server{mgr: mgr, tokens: tokens}
	s.r = gin.Default() // 自带 Logger + Recovery 中间件
	s.register()
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
	adm.POST("/reload", s.reloadConfig)
	adm.GET("/tokens", s.listTokens)
	adm.POST("/tokens", s.createToken)
	adm.DELETE("/tokens/:id", s.deleteToken)

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
}

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, apiResp{Success: true, Data: data})
}

func fail(c *gin.Context, code int, msg string) {
	c.JSON(code, apiResp{Success: false, Message: msg})
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

	client, err := s.mgr.HMEClient(req.AccountID, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}

	result, err := client.CreateAlias(req.Label, 5)

	// 操作完成后,保存可能已刷新的 Cookie（validate 会轮换 token）
	_ = s.mgr.SaveCookies(req.AccountID, client.Cookies)

	if err != nil {
		// 区分会话失效(需重新登录)与临时失败
		msg := err.Error()
		if isSessionError(msg) {
			fail(c, http.StatusUnauthorized, "iCloud 会话失效,请更新 Cookie: "+msg)
		} else {
			fail(c, http.StatusBadGateway, "创建邮箱失败: "+msg)
		}
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

	ok(c, gin.H{
		"email":        result.Email,
		"anonymous_id": result.AnonymousID,
		"label":        result.Label,
		"created_at":   result.CreatedAt,
		"account_id":   req.AccountID,
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
			fail(c, http.StatusBadGateway, "读取邮件失败: "+err.Error())
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
			fail(c, http.StatusBadGateway, "读取邮件失败: "+err.Error())
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

type loginReq struct {
	Password string `json:"password" binding:"required"`
	OTPCode  string `json:"otp_code"` // 可选 2FA 验证码
}

func (s *Server) loginAccount(c *gin.Context) {
	id := c.Param("id")
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: password 必填 — "+err.Error())
		return
	}

	var otpProvider hme.OTPProvider
	if req.OTPCode != "" {
		otp := req.OTPCode
		otpProvider = func() (string, error) {
			return otp, nil
		}
	}

	client, err := s.mgr.HMEClientWithPassword(id, req.Password, otpProvider)
	if err != nil {
		if isSessionError(err.Error()) {
			fail(c, http.StatusUnauthorized, err.Error())
		} else {
			fail(c, http.StatusBadGateway, "登录失败: "+err.Error())
		}
		return
	}

	ok(c, gin.H{
		"id":      id,
		"cookies": client.Cookies,
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
		if isSessionError(err.Error()) {
			fail(c, http.StatusUnauthorized, "iCloud 会话失效,请更新 Cookie: "+err.Error())
		} else {
			fail(c, http.StatusBadGateway, err.Error())
		}
		return
	}
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
		fail(c, http.StatusBadGateway, "停用失败: "+err.Error())
		return
	}
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
		fail(c, http.StatusBadGateway, "激活失败: "+err.Error())
		return
	}
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
		fail(c, http.StatusBadGateway, "删除失败: "+err.Error())
		return
	}
	_ = s.mgr.SaveCookies(req.AccountID, client.Cookies)
	// 成功后从归属记录里移除(所有 token 都扫一遍,admin 也会解绑对应记录)
	for _, t := range s.tokens.List() {
		_ = s.tokens.UnbindAlias(t.ID, anonymousID)
	}
	ok(c, gin.H{"anonymous_id": anonymousID})
}

// isSessionError 判断错误是否由会话失效引起。
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
