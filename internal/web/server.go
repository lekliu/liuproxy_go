package web

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sync"

	"liuproxy_go/internal/types"
)

// 3. 将 embed.FS 直接移到这里，因为它只被这个包使用

//go:embed all:static
var staticFiles embed.FS

// StartServer 启动Web配置服务器
func StartServer(wg *sync.WaitGroup, cfg *types.Config, configPath string, onReload func() error) {
	if cfg.LocalConf.WebPort <= 0 {
		log.Println("[WebServer] Web UI is disabled (web_port is 0 or not set).")
		return
	}

	handler := NewHandler(cfg, configPath, onReload)
	mux := http.NewServeMux()

	// API 路由
	mux.HandleFunc("/api/servers", handler.HandleServers)
	mux.HandleFunc("/api/servers/activate", handler.HandleActivateServer)
	mux.HandleFunc("/api/status", handler.HandleStatus)

	// a. 从嵌入的文件系统中创建一个以 "static" 目录为根的子文件系统
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("Failed to create sub filesystem for static assets: %v", err)
	}
	// b. 创建一个基于这个子文件系统的 FileServer
	fileServer := http.FileServer(http.FS(staticFS))

	// c. 注册一个处理器，它会剥离 "/static/" 前缀，然后交给 fileServer
	//    例如: /static/style.css -> style.css -> 在 staticFS (即 embed/static) 中查找 style.css
	mux.Handle("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		http.StripPrefix("/static/", fileServer).ServeHTTP(w, r)
	}))

	// d. 注册根路径处理器，用于提供 index.html
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		// 从嵌入的文件系统中读取 index.html
		index, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "Could not load index.html", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	})

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.LocalConf.WebPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("!!! FAILED to start Web UI on %s: %v", addr, err)
		return
	}

	log.Printf(">>> SUCCESS: Web UI is listening on http://%s", addr)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := http.Serve(listener, mux); err != nil && err != http.ErrServerClosed {
			log.Printf("Web server error: %v", err)
		}
		log.Println("Web server stopped.")
	}()
}
