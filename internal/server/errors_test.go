package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"icloud-hme/internal/hme"
)

func TestFailUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name         string
		err          error
		wantCode     int
		wantUpstream int
	}{
		{
			// 421 在 HTTP/2 里有连接层语义,不能原样透传给浏览器
			name:         "421 映射成 401,原始码放 upstream_status",
			err:          &hme.UpstreamError{Status: 421, Body: `{"reason":"trust"}`},
			wantCode:     http.StatusUnauthorized,
			wantUpstream: 421,
		},
		{
			name:         "401 会话失效",
			err:          &hme.UpstreamError{Status: 401},
			wantCode:     http.StatusUnauthorized,
			wantUpstream: 401,
		},
		{
			name:         "包装过几层也能取到状态码",
			err:          fmt.Errorf("创建别名失败: %w", fmt.Errorf("reserve 失败: %w", &hme.UpstreamError{Status: 403})),
			wantCode:     http.StatusUnauthorized,
			wantUpstream: 403,
		},
		{
			name:         "429 限流",
			err:          &hme.UpstreamError{Status: 429},
			wantCode:     http.StatusTooManyRequests,
			wantUpstream: 429,
		},
		{
			name:         "其他上游错误走 502",
			err:          &hme.UpstreamError{Status: 503},
			wantCode:     http.StatusBadGateway,
			wantUpstream: 503,
		},
		{
			// 之所以要按状态码分档而不是匹配消息:上游响应体里也会出现这些数字
			name:         "响应体里的 401 不该被当成会话失效",
			err:          &hme.UpstreamError{Status: 503, Body: `{"code":401,"msg":"backend down"}`},
			wantCode:     http.StatusBadGateway,
			wantUpstream: 503,
		},
		{
			name:         "拿不到状态码时退回关键字匹配",
			err:          fmt.Errorf("iCloud 会话校验失败"),
			wantCode:     http.StatusUnauthorized,
			wantUpstream: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			failUpstream(c, "拉取别名列表失败", tc.err)

			if w.Code != tc.wantCode {
				t.Errorf("HTTP 状态码 = %d, 期望 %d", w.Code, tc.wantCode)
			}
			var body apiResp
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("响应不是合法 JSON: %v", err)
			}
			if body.UpstreamStatus != tc.wantUpstream {
				t.Errorf("upstream_status = %d, 期望 %d", body.UpstreamStatus, tc.wantUpstream)
			}
			if body.Scope != "upstream" {
				t.Errorf(`scope = %q, 期望 "upstream" —— 前端靠它区分面板 token 失效和 iCloud 会话失效`, body.Scope)
			}
			if body.Success {
				t.Error("success 应该是 false")
			}
		})
	}
}

func TestPendingLoginStore(t *testing.T) {
	s := &Server{logins: make(map[string]*pendingLogin)}

	loginID := s.putPendingLogin("acc_1", nil)
	if _, found := s.takePendingLogin("acc_2", loginID); found {
		t.Error("句柄不该在别的账号下生效")
	}
	if _, found := s.takePendingLogin("acc_1", loginID); !found {
		t.Fatal("同账号应该能取到")
	}
	if _, found := s.takePendingLogin("acc_1", loginID); found {
		t.Error("取过一次就该失效,验证码不能重放")
	}

	// 同一个账号再发起一次登录,旧的那份会话必须让位:Apple 已经把旧码作废了
	first := s.putPendingLogin("acc_1", nil)
	second := s.putPendingLogin("acc_1", nil)
	if _, found := s.takePendingLogin("acc_1", first); found {
		t.Error("同账号发起新登录后,旧句柄应该失效")
	}
	if _, found := s.takePendingLogin("acc_1", second); !found {
		t.Error("最新的句柄应该有效")
	}
}
