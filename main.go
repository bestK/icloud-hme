// Command icloud-hme 启动 iCloud Hide My Email 多账号管理平台。
//
// 两个核心 HTTP 接口:
//
//	POST /api/create  — 创建隐私邮箱别名(优先从暖池 pop)
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
//	ADMIN_TOKEN      admin 主 token(必填)
//	POOL_TARGET      每账号暖池最低保障水位(默认 10;<=0 禁用池)。这不是补池
//	                 的终点 —— 只要配额还有余量就会一直囤,直到账号触及 Apple
//	                 的 750 别名上限
//	POOL_INTERVAL    补池间隔(默认 15m)
//	POOL_HOURLY_MAX  每账号每小时最多真实 create 次数(默认 4;<=0 禁用池)。
//	                 池空时的实时创建也记在这本账上
//	POOL_SPACING     同一轮内两次创建之间的间隔(默认 20s),带 ±25% 抖动
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"icloud-hme/internal/account"
	"icloud-hme/internal/pool"
	"icloud-hme/internal/server"
	"icloud-hme/internal/token"
)

func main() {
	addr := flag.String("addr", ":8081", "HTTP 监听地址")
	dataDir := flag.String("data", "./data", "数据目录")
	debug := flag.Bool("debug", false, "调试模式")
	flag.Parse()

	log.Printf("iCloud Hide My Email 服务启动 addr=%s", *addr)

	adminSecret := os.Getenv("ADMIN_TOKEN")
	if adminSecret == "" {
		log.Fatal("必须设置环境变量 ADMIN_TOKEN")
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

	poolStore, err := pool.NewStore(abs)
	if err != nil {
		log.Fatalf("初始化暖池失败: %v", err)
	}
	filler := pool.NewFiller(
		mgr, poolStore,
		envInt("POOL_TARGET", 10),
		envInt("POOL_HOURLY_MAX", 4),
		envDuration("POOL_INTERVAL", 15*time.Minute),
		envDuration("POOL_SPACING", 20*time.Second),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	filler.Start(ctx)

	srv := server.New(mgr, tokens, poolStore, filler, *debug)

	log.Printf("HTTP 服务就绪 addr=%s", *addr)
	if err := srv.Run(*addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
