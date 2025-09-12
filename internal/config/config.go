// --- START OF COMPLETE REPLACEMENT for config.go ---
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"liuproxy_go/internal/types" // 导入统一的 types 包

	ini "gopkg.in/ini.v1"
)

// LoadIni 从指定的 fileName 加载配置到传入的 types.Config 结构体中。
func LoadIni(cfg *types.Config, fileName string) error {
	iniFile, err := ini.Load(fileName)
	if err != nil {
		return err
	}

	// 使用 MapTo 自动将 .ini 文件的 section 映射到 cfg 结构体的嵌入字段
	if err := iniFile.MapTo(cfg); err != nil {
		return err
	}

	// --- 手动解析 remote_ip_* 字段 ---
	localSection := iniFile.Section("local")
	rawRemoteIPs := [][][]string{} // 用一个三维数组来存储原始顺序和解析后的数据
	for i, key := range localSection.Keys() {
		if strings.HasPrefix(key.Name(), "remote_ip_") {
			parts := key.Strings(",")
			fullParts := make([]string, 8) // 确保总是有8个字段
			copy(fullParts, parts)
			// 存储原始索引和数据
			rawRemoteIPs = append(rawRemoteIPs, [][]string{{strconv.Itoa(i)}, fullParts})
		}
	}

	// 找到第一个被激活的服务器
	activeIndex := -1
	for i, entry := range rawRemoteIPs {
		if len(entry[1]) > 7 && entry[1][7] == "true" {
			activeIndex = i
			break
		}
	}
	// 重建 RemoteIPs 列表，将被激活的服务器（如果有）放在首位
	cfg.LocalConf.RemoteIPs = [][]string{}
	if activeIndex != -1 {
		cfg.LocalConf.RemoteIPs = append(cfg.LocalConf.RemoteIPs, rawRemoteIPs[activeIndex][1])
		rawRemoteIPs = append(rawRemoteIPs[:activeIndex], rawRemoteIPs[activeIndex+1:]...)
	}

	for _, entry := range rawRemoteIPs {
		cfg.LocalConf.RemoteIPs = append(cfg.LocalConf.RemoteIPs, entry[1])
	}

	// 如果没有任何激活的，并且列表不为空，则将第一个设为激活状态（内存中）
	if activeIndex == -1 && len(cfg.LocalConf.RemoteIPs) > 0 {
		cfg.LocalConf.RemoteIPs[0][7] = "true"
	}

	overrideFromEnvInt(&cfg.CommonConf.Crypt, "CRYPT_KEY")
	overrideFromEnvInt(&cfg.RemoteConf.PortWsSvr, "REMOTE_PORT_WS_SVR")

	return nil
}

// SaveIni 将内存中的 types.Config 结构体保存回指定的 fileName。
func SaveIni(cfg *types.Config, fileName string) error {
	iniFile := ini.Empty()
	err := ini.ReflectFrom(iniFile, cfg)
	if err != nil {
		return fmt.Errorf("failed to reflect config to ini object: %w", err)
	}

	localSection, err := iniFile.GetSection("local")
	if err != nil {
		localSection, _ = iniFile.NewSection("local")
	}

	// 清理旧的 remote_ip_* 键
	for _, key := range localSection.KeyStrings() {
		if strings.HasPrefix(key, "remote_ip_") {
			localSection.DeleteKey(key)
		}
	}

	// 写入新的 remote_ip_* 键
	for i, remoteCfg := range cfg.LocalConf.RemoteIPs {
		keyName := fmt.Sprintf("remote_ip_%02d", i+1)
		fullCfg := make([]string, 8)
		copy(fullCfg, remoteCfg)
		localSection.Key(keyName).SetValue(strings.Join(fullCfg, ","))
	}

	return iniFile.SaveTo(fileName)
}

// overrideFromEnvInt 是一个私有辅助函数 (保持不变)
func overrideFromEnvInt(target *int, envName string) {
	envValue := os.Getenv(envName)
	if envValue != "" {
		if intValue, err := strconv.Atoi(envValue); err == nil {
			*target = intValue
		}
	}
}
