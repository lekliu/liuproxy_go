package main

import (
	"flag"
	"fmt"
	"main/conf"
	"main/local"
	"main/remote"
	"os"
	"path/filepath"
)

var (
	Cfg = new(conf.AppConfig)
)

// findConfigFile 函数按照优先级查找配置文件路径
func findConfigFile() (string, error) {
	// 1. 检查命令行参数
	configFile := flag.String("config", "", "Path to the configuration file (e.g., -config /path/to/config.ini)")
	flag.Parse()

	if *configFile != "" {
		fmt.Println("Using config file from command-line argument:", *configFile)
		return *configFile, nil
	}

	// 2. 检查环境变量
	envConfigFile := os.Getenv("LIUPROXY_CONFIG")
	if envConfigFile != "" {
		fmt.Println("Using config file from LIUPROXY_CONFIG environment variable:", envConfigFile)
		return envConfigFile, nil
	}

	// 3. 使用相对于可执行文件的默认路径
	//    这是解决你最初问题的关键点！
	exePath, err := os.Executable() // 获取可执行文件自身的绝对路径
	if err != nil {
		return "", fmt.Errorf("could not get executable path: %w", err)
	}

	defaultPath := filepath.Join(filepath.Dir(exePath), "./conf/config.ini")
	fmt.Println("No flag or env var found. Trying default path relative to executable:", defaultPath)
	if _, err := os.Stat(defaultPath); os.IsNotExist(err) {
		defaultPath = filepath.Join(filepath.Dir(exePath), "../conf/config.ini")
		if _, err := os.Stat(defaultPath); os.IsNotExist(err) {
			defaultPath = filepath.Join(filepath.Dir(exePath), "../../conf/config.ini")
			if _, err := os.Stat(defaultPath); os.IsNotExist(err) {
				return "", fmt.Errorf("default config file not found at %s", defaultPath)
			}
		}
	}

	return defaultPath, nil
}

func main() {
	// 0. 查找配置文件路径
	configPath, err := findConfigFile()
	if err != nil {
		fmt.Println("Error:", err)
		fmt.Println("Please specify the config file via -config flag or LIUPROXY_CONFIG environment variable.")
		return
	}

	// 1. 加载配置文件
	err = conf.LoadIni(Cfg, configPath)
	if err != nil {
		fmt.Printf("Failed to load config file '%s', error: %v\n", configPath, err)
		return
	}

	// 1. 根据配置的 mode 启动服务
	switch Cfg.CommonConf.Mode {
	case "local":
		fmt.Println("Starting in LOCAL mode...")
		local.RunServer(Cfg)
	case "remote":
		fmt.Println("Starting in REMOTE mode...")
		remote.RunServer(Cfg)
	default:
		fmt.Printf("Invalid mode specified in config.ini: '%s'. Please use 'local' or 'remote'.\n", Cfg.CommonConf.Mode)
	}

	// 保持主线程运行
	select {}
}
