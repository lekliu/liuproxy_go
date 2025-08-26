package main

import (
	"flag"
	"log"

	"liuproxy_go/internal/config"
	"liuproxy_go/internal/server"
)

func main() {
	configPath := flag.String("config", "configs/remote.ini", "Path to remote config file")
	flag.Parse()

	// 1. 加载配置
	cfg := new(config.AppConfig)
	if err := config.LoadIni(cfg, *configPath); err != nil {
		log.Fatalf("Failed to load config file '%s': %v", *configPath, err)
	}

	// 2. 创建并运行服务器
	appServer := server.New(cfg)
	appServer.Run()
}
