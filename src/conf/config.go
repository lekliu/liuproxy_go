package conf

import (
	"gopkg.in/ini.v1"
)

type AppConfig struct {
	CommonConf `ini:"common"`
	LocalConf  `ini:"local"`
	RemoteConf `ini:"remote"`
}

type CommonConf struct {
	MaxConnections int `ini:"maxConnections"`
	BufferSize     int `ini:"bufferSize"`
}

type RemoteConf struct {
	PortHttpSvr   int `ini:"port_http_svr"`
	PortSocks5Svr int `ini:"port_socks5_svr"`
	PortSocks8Svr int `ini:"port_socks8_svr"`
}

type LocalConf struct {
	PortHttpFirst  int `ini:"port_http_first"`
	PortHttpSecond int `ini:"port_http_second"`
	PortSocks5     int `ini:"port_socks5"`
	RemoteIPs      [][]string
}

func LoadIni(cfg *AppConfig, fileName string) error {
	// 读取配置文件
	iniFile, err := ini.Load(fileName)
	if err != nil {
		return err
	}
	// 获取 [common] 部分
	commonSection := iniFile.Section("common")
	cfg.CommonConf.MaxConnections = commonSection.Key("maxConnections").MustInt(16)
	cfg.CommonConf.BufferSize = commonSection.Key("bufferSize").MustInt(1024)

	// 获取 [remote] 部分
	remoteSection := iniFile.Section("remote")
	cfg.RemoteConf.PortHttpSvr = remoteSection.Key("port_http_svr").MustInt(0)
	cfg.RemoteConf.PortSocks5Svr = remoteSection.Key("port_socks5_svr").MustInt(0)
	cfg.RemoteConf.PortSocks8Svr = remoteSection.Key("port_socks8_svr").MustInt(0)

	// 获取 [local] 部分
	localSection := iniFile.Section("local")
	cfg.LocalConf.PortHttpFirst = localSection.Key("port_http_first").MustInt(0)
	cfg.LocalConf.PortHttpSecond = localSection.Key("port_http_second").MustInt(0)
	cfg.LocalConf.PortSocks5 = localSection.Key("port_socks5").MustInt(0)

	// 遍历所有以 remote_ip 开头的键
	for _, key := range localSection.Keys() {
		if len(key.Name()) >= 9 && key.Name()[:9] == "remote_ip" {
			// 将值按逗号分割并存入结构体的二维数组
			cfg.LocalConf.RemoteIPs = append(cfg.LocalConf.RemoteIPs, key.Strings(","))
		}
	}

	return nil
}
