package main

import (
	"flag"
	"liuproxy_go/internal/types"
	"log"

	"liuproxy_go/internal/config"
	"liuproxy_go/internal/server"
)

func main() {
	// 默认配置文件路径现在指向新的 configs/ 目录
	configPath := flag.String("config", "configs/local.ini", "Path to local config file")
	flag.Parse()

	// 1. 加载配置
	cfg := new(types.Config)
	if err := config.LoadIni(cfg, *configPath); err != nil {
		log.Fatalf("Failed to load config file '%s': %v", *configPath, err)
	}

	// 2. 创建并运行服务器
	appServer := server.New(cfg, *configPath)
	appServer.Run()
}
