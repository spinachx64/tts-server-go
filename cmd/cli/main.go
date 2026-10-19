package main

import (
	"flag"
	logformat "github.com/antonfisher/nested-logrus-formatter"
	"github.com/jing332/tts-server-go/internal/tokenstore"
	"github.com/jing332/tts-server-go/server"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"os/signal"
)

var port = flag.Int64("port", 1233, "自定义监听端口")
var token = flag.String("token", "", "自定义访问令牌；客户端通过Token请求头发送，默认不验证")
var logDir = flag.String("log-dir", defaultLogDir(), "API每日JSONL日志目录")
var adminToken = flag.String("admin-token", os.Getenv("TTS_ADMIN_TOKEN"), "专用管理Token；建议使用TTS_ADMIN_TOKEN环境变量")
var tokenDB = flag.String("token-db", "", "外部分发程序维护的SQLite Token数据库；只读验证")

var useDnsEdge = flag.Bool("use-dns-edge", false, "兼容旧参数；Edge新接口始终使用DNS解析。")

func defaultLogDir() string {
	if dir := os.Getenv("TTS_LOG_DIR"); dir != "" {
		return dir
	}
	return "logs"
}

func main() {
	log.SetFormatter(&logformat.Formatter{HideKeys: true,
		TimestampFormat: "01-02|15:04:05",
	})
	flag.Parse()
	if *token != "" {
		log.Info("已启用Token验证")
	}
	if *useDnsEdge == true {
		log.Infof("使用DNS解析Edge接口")
	}

	srv := &server.GracefulServer{Token: *token, UseDnsEdge: *useDnsEdge}
	if *tokenDB != "" {
		if *token != "" {
			log.Fatal("-token-db 与 -token 不能同时设置")
		}
		store, err := tokenstore.Open(*tokenDB)
		if err != nil {
			log.Fatalf("Token数据库初始化失败: %v", err)
		}
		defer store.Close()
		srv.TokenValidator = store
		log.Info("已启用SQLite Token验证")
	}
	if *adminToken != "" {
		if len(*adminToken) < 32 {
			log.Fatal("管理Token至少32字节")
		}
		if *tokenDB == "" {
			log.Fatal("管理接口需要-token-db")
		}
		manager, err := tokenstore.OpenManager(*tokenDB)
		if err != nil {
			log.Fatalf("Token管理初始化失败: %v", err)
		}
		defer manager.Close()
		srv.AdminToken = *adminToken
		srv.TokenManager = manager
	}
	logger, err := server.NewAPILogger(*logDir)
	if err != nil {
		log.Fatalf("API日志初始化失败: %v", err)
	}
	srv.APILogger = logger
	srv.HandleFunc()

	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		srv.Close()
	}()

	if err := srv.ListenAndServe(*port); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server ListenAndServe: %v", err)
	}
	log.Infoln("服务已关闭")
}
