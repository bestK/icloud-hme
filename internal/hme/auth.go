// Package hme - iCloud 认证模块
//
// 基于 Go-iClient 项目实现完整的 SRP (Secure Remote Password) 登录流程,
// 支持双重认证 (2FA),登录成功后提取 session token Cookie。
package hme

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"golang.org/x/crypto/pbkdf2"

	http "github.com/bogdanfinn/fhttp"
	"icloud-hme/internal/srp"
)

// AuthEndpoints iCloud 认证 API 端点
const (
	OAuthClientID = "d39ba9916b7251055b22c7f910e2ea796ee65e98b2ddecea8f5dde8d9d1a815d"

	authStartFmt    = "https://idmsa.apple.com/appleauth/auth/authorize/signin?frame_id=auth-%s&language=en_US&skVersion=7&iframeId=auth-%s&client_id=%s&redirect_uri=https://www.icloud.com&response_type=code&response_mode=web_message&state=auth-%s&authVersion=latest"
	authFederate    = "https://idmsa.apple.com/appleauth/auth/federate?isRememberMeEnabled=true"
	authInit        = "https://idmsa.apple.com/appleauth/auth/signin/init"
	authComplete    = "https://idmsa.apple.com/appleauth/auth/signin/complete?isRememberMeEnabled=true"
	authOptions     = "https://idmsa.apple.com/appleauth/auth"
	submitSecurity  = "https://idmsa.apple.com/appleauth/auth/verify/%s/securitycode"
	authTrust       = "https://idmsa.apple.com/appleauth/auth/2sv/trust"
	authWebFmt      = "https://setup.icloud.com/setup/ws/1/accountLogin"
	authValidateFmt = "https://setup.icloud.com/setup/ws/1/validate?clientBuildNumber=%s&clientMasteringNumber=%s&clientId=%s"
)

// authState 保存认证过程中的状态
type authState struct {
	username string
	frameId  string
	clientId string
	// srpC 是 signin/init 返回的 SRP 挑战令牌(形如 "i-569-...")。
	// signin/complete 必须原样带回,Apple 靠它找回那次 SRP 会话。
	// 别和 clientId(OAuth 客户端 ID)搞混 —— 传错了 M1 就无法校验,
	// Apple 一律返 401,看起来跟密码错误一模一样。
	srpC       string
	authAttr   string
	sessionID  string
	scnt       string
	authToken  string
	trustToken string
	dsid       string
}

// AuthError 是 Apple 认证服务(idmsa.apple.com)拒绝登录时返回的错误。
//
// 和 UpstreamError 不是一回事:UpstreamError 说的是「已经拿到的会话用不了了」,
// 处理办法是重新登录;AuthError 说的是「这次登录本身没通过」。两者混在一起,
// 就会对着正在登录的用户提示「请更新 Cookie」这种毫无意义的话。
type AuthError struct {
	Status int    // idmsa 返回的 HTTP 状态码
	Reason string // 人类可读的原因
	Body   string // Apple 给的具体错误(取 serviceErrors,取不到就是响应体片段)
}

func (e *AuthError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("%s (HTTP %d)", e.Reason, e.Status)
	}
	return fmt.Sprintf("%s (HTTP %d) — %s", e.Reason, e.Status, e.Body)
}

// UpstreamStatus 让调用方不必区分错误类型就能取到上游状态码。
func (e *AuthError) UpstreamStatus() int { return e.Status }

// authFail 把 idmsa 的非预期响应包成 AuthError。调用方负责给出 reason。
func authFail(resp *http.Response, reason string) *AuthError {
	raw, _ := io.ReadAll(resp.Body)
	// Apple 把具体原因放在 serviceErrors 里,比响应体片段准
	detail := gjson.GetBytes(raw, "serviceErrors.0.message").String()
	if detail == "" {
		detail = strings.TrimSpace(string(raw))
		if len(detail) > 200 {
			detail = detail[:200]
		}
	}
	return &AuthError{Status: resp.StatusCode, Reason: reason, Body: detail}
}

// PendingLogin 是一次卡在双重认证上的登录:密码已经通过 SRP 校验,
// Apple 已经把验证码推到受信任设备,等着提交。
//
// 它必须复用发起登录的那个 Client —— sessionID / scnt 和 cookie jar 都绑在
// 那一次会话上。重新走一遍 SRP 会让 Apple 重发一个新码,用户手上那个当场作废,
// 所以「填了验证码再点一次登录」这种交互是走不通的。
type PendingLogin struct {
	c     *Client
	state *authState
}

// Client 返回承载这次登录的客户端。Submit 成功后 Cookie 就在它的 Cookies 里。
func (p *PendingLogin) Client() *Client { return p.c }

// Username 返回这次登录用的 Apple ID。
func (p *PendingLogin) Username() string { return p.state.username }

// BeginLogin 用 iCloud 账号密码走完 SRP 校验。
//
// 返回值 *PendingLogin 非 nil 表示账号启用了双重认证,验证码已发到受信任设备,
// 需要再调用 PendingLogin.Submit 提交验证码才算登录完成;为 nil 表示登录已完成,
// Cookie 可以直接从 c.Cookies 取。
func (c *Client) BeginLogin(username, password string) (*PendingLogin, error) {
	state := &authState{username: username}

	// 1. 初始化 frameId 和 clientId
	if err := c.authStart(state); err != nil {
		return nil, fmt.Errorf("auth start: %w", err)
	}

	// 2. 提交用户名
	if err := c.authFederate(state); err != nil {
		return nil, fmt.Errorf("auth federate: %w", err)
	}

	// 3. SRP 协议初始化
	params := srp.GetParams(2048)
	params.NoUserNameInX = true
	srpClient := srp.NewSRPClient(params, nil)

	// 4. 获取 salt 和 B
	authInitResp, err := c.authInit(state, base64.StdEncoding.EncodeToString(srpClient.GetABytes()))
	if err != nil {
		return nil, fmt.Errorf("auth init: %w", err)
	}

	// 5. 解码 salt 和 B
	bDec, err := base64.StdEncoding.DecodeString(authInitResp.B)
	if err != nil {
		return nil, fmt.Errorf("decode B: %w", err)
	}
	saltDec, err := base64.StdEncoding.DecodeString(authInitResp.Salt)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}

	// 6. 生成密码密钥
	passKey := srpPasswordKey(password, authInitResp.Protocol, saltDec, authInitResp.Iteration)

	// 7. 处理挑战
	srpClient.ProcessClientChanllenge([]byte(username), passKey, saltDec, bDec)

	// 8. 提交 SRP 响应 (可能触发 2FA)
	state.srpC = authInitResp.C
	needs2FA, err := c.authComplete(state, base64.StdEncoding.EncodeToString(srpClient.M1), base64.StdEncoding.EncodeToString(srpClient.M2))
	if err != nil {
		return nil, fmt.Errorf("auth complete: %w", err)
	}
	if needs2FA {
		c.log("账号启用了双重认证,等待验证码")
		return &PendingLogin{c: c, state: state}, nil
	}

	return nil, c.finishLogin(state)
}

// Submit 提交 2FA 验证码并完成登录。
func (p *PendingLogin) Submit(code string) error {
	if err := p.c.submitSecurityCode(p.state, code); err != nil {
		return err
	}
	return p.c.finishLogin(p.state)
}

// finishLogin 走「信任设备 → 换 iCloud Web Cookie」,收尾一次登录。
func (c *Client) finishLogin(state *authState) error {
	if err := c.getTrust(state); err != nil {
		return fmt.Errorf("get trust: %w", err)
	}
	if err := c.authenticateWeb(state); err != nil {
		return fmt.Errorf("authenticate web: %w", err)
	}
	c.Cookies = c.extractSessionCookies()
	c.log("登录成功,获取到 %d 个 Cookie", len(c.Cookies))
	return nil
}

// srpPasswordKey 按 Apple 选定的协议派生 SRP 用的密码密钥。
//
// s2k 直接用 SHA-256 摘要,s2k_fo 用摘要的十六进制字符串。协议是 signin/init
// 响应里的 protocol 字段说的,不能假定 —— 选错这一步 M1 对不上,Apple 在
// signin/complete 上返 401,和密码错误的表现完全一样。
func srpPasswordKey(password, protocol string, salt []byte, iterations int) []byte {
	digest := sha256.Sum256([]byte(password))
	material := digest[:]
	if protocol == "s2k_fo" {
		material = []byte(hex.EncodeToString(digest[:]))
	}
	return pbkdf2.Key(material, salt, iterations, 32, sha256.New)
}

// --- 认证流程的各步骤 ---

// authStart 初始化 frameId 和 clientId
func (c *Client) authStart(state *authState) error {
	state.frameId = strings.ToLower(uuid.New().String())
	state.clientId = OAuthClientID

	req, err := http.NewRequest("GET", fmt.Sprintf(authStartFmt, state.frameId, state.frameId, state.clientId, state.frameId), nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return authFail(resp, "认证会话初始化失败")
	}

	state.authAttr = resp.Header.Get("X-Apple-Auth-Attributes")
	return nil
}

// authFederate 提交用户名
func (c *Client) authFederate(state *authState) error {
	data := `{"accountName":"` + state.username + `","rememberMe":true}`
	req, err := http.NewRequest("POST", authFederate, bytes.NewReader([]byte(data)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header = c.updateAuthHeaders(req.Header, state)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return authFail(resp, "提交账号名被拒绝")
	}
	return nil
}

// authInitResp authInit 响应
type authInitResp struct {
	Iteration int    `json:"iteration"`
	Salt      string `json:"salt"`
	Protocol  string `json:"protocol"`
	B         string `json:"b"`
	C         string `json:"c"`
}

// authInit 初始化 SRP 认证
func (c *Client) authInit(state *authState, a string) (*authInitResp, error) {
	reqBody := map[string]interface{}{
		"a":           a,
		"accountName": state.username,
		"protocols":   []string{"s2k", "s2k_fo"},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", authInit, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header = c.updateAuthHeaders(req.Header, state)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 不检查状态码就 decode,拿到的是一个字段全空的结构体,错误会推迟到
	// signin/complete 上变成一个看不懂的 401
	if resp.StatusCode != 200 {
		return nil, authFail(resp, "SRP 初始化被拒绝(账号名可能不存在)")
	}

	var result authInitResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if result.B == "" || result.Salt == "" || result.C == "" {
		return nil, fmt.Errorf("SRP 初始化响应缺字段 (b/salt/c),协议可能变了")
	}
	return &result, nil
}

// authComplete 提交 SRP 响应。needs2FA 为 true 时,验证码已发出,
// state 里的 sessionID / scnt 已经填好,可以直接提交验证码。
func (c *Client) authComplete(state *authState, m1, m2 string) (needs2FA bool, err error) {
	reqBody := map[string]interface{}{
		"accountName": state.username,
		"rememberMe":  true,
		"trustTokens": []string{},
		"m1":          m1,
		// 必须是 signin/init 返回的挑战令牌,不是 OAuth clientId
		"c":  state.srpC,
		"m2": m2,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest("POST", authComplete, bytes.NewReader(data))
	if err != nil {
		return false, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header = c.updateAuthHeaders(req.Header, state)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		return false, nil
	case 409:
		// 需要 2FA:留下 sessionID / scnt,后续提交验证码要带
		state.sessionID = resp.Header.Get("X-Apple-ID-Session-Id")
		state.scnt = resp.Header.Get("scnt")
		if state.sessionID == "" {
			return false, fmt.Errorf("需要双重认证,但响应里没有 X-Apple-ID-Session-Id")
		}
		return true, nil
	case 401, 403:
		return false, authFail(resp, "Apple 拒绝了密码校验(Apple ID 或密码不正确,或账号被锁定)")
	case 412:
		return false, authFail(resp, "需要先到 appleid.apple.com 同意隐私条款")
	default:
		return false, authFail(resp, "密码校验失败")
	}
}

// submitSecurityCode 提交受信任设备上收到的 6 位验证码。
func (c *Client) submitSecurityCode(state *authState, code string) error {
	reqBody := map[string]interface{}{
		"securityCode": map[string]string{"code": code},
	}

	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", fmt.Sprintf(submitSecurity, "trusteddevice"), bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header = c.updateAuthHeaders(req.Header, state)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		// Apple 对错码返 400。这一步失败后 sessionID 就废了,只能重新发起
		// 登录让它重发一个码。
		if resp.StatusCode == 400 {
			return authFail(resp, "验证码错误或已过期,请重新输密码发起登录")
		}
		return authFail(resp, "验证码校验失败")
	}

	if newScnt := resp.Header.Get("scnt"); newScnt != "" {
		state.scnt = newScnt
	}
	return nil
}

// getTrust 获取 trust token
func (c *Client) getTrust(state *authState) error {
	req, err := http.NewRequest("GET", authTrust, nil)
	if err != nil {
		return err
	}

	req.Header = c.updateAuthHeaders(req.Header, state)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		return authFail(resp, "信任设备失败")
	}

	state.authToken = resp.Header.Get("X-Apple-Session-Token")
	state.trustToken = resp.Header.Get("X-Apple-TwoSV-Trust-Token")
	return nil
}

// authenticateWeb 认证 iCloud Web 服务
func (c *Client) authenticateWeb(state *authState) error {
	body := fmt.Sprintf(`{"dsWebAuthToken":"%s","accountCountryCode":"USA","extended_login":true,"trustToken":"%s"}`,
		state.authToken, state.trustToken)

	req, err := http.NewRequest("POST", authWebFmt, bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.Origin())
	req.Header.Set("Accept", "*/*")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return authFail(resp, "换取 iCloud Web 会话失败")
	}

	var result struct {
		DsInfo struct {
			Dsid string `json:"dsid"`
		} `json:"dsInfo"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	state.dsid = result.DsInfo.Dsid

	// 复制 idmsa.apple.com 的 Cookie 到 icloud.com
	u1, _ := url.Parse("https://idmsa.apple.com")
	u2, _ := url.Parse("https://icloud.com")
	cookies := c.httpc.GetCookies(u1)
	c.httpc.SetCookies(u2, cookies)

	return nil
}

// extractSessionCookies 提取 session token Cookie
func (c *Client) extractSessionCookies() map[string]string {
	cookies := make(map[string]string)
	u, _ := url.Parse(c.Origin())
	for _, cookie := range c.httpc.GetCookies(u) {
		cookies[cookie.Name] = cookie.Value
	}
	return cookies
}

// updateAuthHeaders 更新认证请求所需的头部
func (c *Client) updateAuthHeaders(header http.Header, state *authState) http.Header {
	if state.scnt != "" {
		header.Set("scnt", state.scnt)
	}
	if state.sessionID != "" {
		header.Set("X-Apple-ID-Session-Id", state.sessionID)
	}

	header.Set("X-Requested-With", "XMLHttpRequest")
	header.Set("Content-Type", "application/json")
	header.Set("Accept", "application/json")
	header.Set("Referer", "https://idmsa.apple.com/")
	header.Set("Origin", "https://idmsa.apple.com")
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")

	return header
}

// Validate 验证当前 Cookie 是否有效
func (c *Client) Validate() (bool, error) {
	if len(c.Cookies) == 0 {
		return false, fmt.Errorf("无 Cookie")
	}
	// 简单实现：尝试调用 validate 端点
	err := c.ValidateSession()
	if err != nil {
		return false, err
	}
	return true, nil
}
