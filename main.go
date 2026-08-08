// Command icloud-hme 启动 iCloud Hide My Email 多账号管理平台。
//
// 两个核心 HTTP 接口:
//
//	POST /api/create  — 创建隐私邮箱别名
//	GET  /api/inbox   — 读取邮件
//
// 用法:
//
//	./icloud-hme                    # 默认 :8081
//	./icloud-hme -addr :9000        # 指定端口
//	./icloud-hme -data ./data       # 指定数据目录
//	./icloud-hme -debug             # 调试模式
//
// 环境变量:
//
//	ADMIN_TOKEN  admin 主 token(必填),用来管理 accounts / tokens
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"icloud-hme/internal/account"
	"icloud-hme/internal/server"
	"icloud-hme/internal/token"
)

func main() {
	addr := flag.String("addr", ":8081", "HTTP 监听地址")
	dataDir := flag.String("data", "./data", "数据目录 (accounts.json / tokens.json 存放位置)")
	debug := flag.Bool("debug", false, "调试模式 (启用 Gin 调试日志)")
	flag.Parse()

	log.Printf("iCloud Hide My Email 服务启动 addr=%s", *addr)

	adminSecret := os.Getenv("ADMIN_TOKEN")
	if adminSecret == "" {
		log.Fatal("必须设置环境变量 ADMIN_TOKEN(admin 主 token,用来管理账号和子 token)")
	}

	abs, err := filepath.Abs(*dataDir)
	if err != nil {
		log.Fatalf("数据目录路径错误: %v", err)
	}

	mgr, err := account.NewManager(abs)
	if err != nil {
		log.Fatalf("初始化账号管理器失败: %v", err)
	}
	log.Printf("账号加载完成 count=%d data_dir=%s", len(mgr.ListAccounts()), abs)

	tokens, err := token.NewStore(abs)
	if err != nil {
		log.Fatalf("初始化 token 存储失败: %v", err)
	}
	if _, err := tokens.EnsureAdmin(adminSecret); err != nil {
		log.Fatalf("初始化 admin token 失败: %v", err)
	}
	log.Printf("token 加载完成 count=%d", len(tokens.List()))

	srv := server.New(mgr, tokens, *debug)

	log.Printf("HTTP 服务就绪 addr=%s", *addr)
	if err := srv.Run(*addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
