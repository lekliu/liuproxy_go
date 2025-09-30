// --- START OF NEW FILE cmd/tester/goremote/main.go ---
package main

import (
	"encoding/json"
	"fmt"
	"liuproxy_go/internal/shared/config"
	"liuproxy_go/internal/shared/logger"
	"liuproxy_go/internal/tunnel"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"liuproxy_go/internal/shared/types"
)

const (
	defaultConfigDir    = "configs"
	iniConfigName       = "liuproxy.ini"
	goremoteProfileName = "config_goremote.json"
)

func main() {
	fmt.Println("--- LiuProxy GoRemote Strategy Tester ---")
	// 1. 加载 liuproxy.ini 获取主配置
	iniPath := filepath.Join(defaultConfigDir, iniConfigName)
	log.Printf("Loading main config from: %s", iniPath)
	appConfig := new(types.Config)
	if err := config.LoadIni(appConfig, iniPath); err != nil {
		log.Fatalf("Error loading main .ini config: %v", err)
	}

	// 2. 根据主配置初始化日志系统
	if err := logger.Init(appConfig.LogConf); err != nil {
		log.Fatalf("Error initializing logger: %v", err)
	}
	logger.Info().Msg("Logger initialized for tester.")

	// 3. 加载 goremote 专用的服务器 profile
	profilePath := filepath.Join(defaultConfigDir, goremoteProfileName)
	logger.Info().Str("path", profilePath).Msg("Loading goremote server profile")
	profile, err := loadProfile(profilePath)
	if err != nil {
		logger.Fatal().Err(err).Msg("Error loading server profile")
	}
	profile.Type = "goremote"
	profile.Active = true

	profilesForFactory := []*types.ServerProfile{profile}

	activeStrategy, err := tunnel.NewStrategy(appConfig, profilesForFactory)
	if err != nil {
		log.Fatalf("Error creating GoRemote strategy: %v", err)
	}

	if err := activeStrategy.Initialize(); err != nil {
		log.Fatalf("Error starting GoRemote strategy: %v", err)
	}
	defer activeStrategy.CloseTunnel()

	waitForSignal()
	log.Println("--- GoRemote Strategy Tester shutdown complete. ---")
}

func loadProfile(path string) (*types.ServerProfile, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var profile types.ServerProfile
	if err := json.Unmarshal(file, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse JSON config: %w", err)
	}
	return &profile, nil
}

func waitForSignal() {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		fmt.Println()
		log.Println("Signal received, shutting down...")
		done <- true
	}()
	log.Println("Strategy is running. Press Ctrl+C to exit.")
	<-done
}
