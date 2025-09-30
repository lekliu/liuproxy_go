package main

import (
	"flag"
	"fmt"
	"liuproxy_go/internal/app"
	"liuproxy_go/internal/shared/config"
	"liuproxy_go/internal/shared/logger"
	"liuproxy_go/internal/shared/types"
	"os"
	"path/filepath"
)

func main() {
	configDir := flag.String("configdir", "configs", "Path to config directory")
	flag.Parse()

	iniPath := filepath.Join(*configDir, "liuproxy.ini")
	serversPath := filepath.Join(*configDir, "servers.json")

	// 1. 加载 .ini 行为配置
	cfg := new(types.Config)
	if err := config.LoadIni(cfg, iniPath); err != nil {
		// Use standard fmt before logger is initialized.
		fmt.Fprintf(os.Stderr, "Fatal: Failed to load config file '%s': %v\n", iniPath, err)
		os.Exit(1)
	}

	// 1.1 初始化日志系统
	if err := logger.Init(cfg.LogConf); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// 2. 加载 servers.json 数据配置

	profiles, err := config.LoadServers(serversPath)
	if err != nil {
		logger.Fatal().Err(err).Msgf("Failed to load servers file '%s'", serversPath)
	}

	// 3. 创建并运行服务器
	appServer := app.New(cfg, iniPath, serversPath, profiles)
	appServer.Run()

}
