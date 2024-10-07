package main

import (
	"flag"
	"fmt"
	"main/conf"
	"main/local"
	"main/remote"
)

var (
	Cfg = new(conf.AppConfig)
)

func main() {
	// 0.加载配置文件
	err := conf.LoadIni(Cfg, "../../conf/config.ini")
	if err != nil {
		fmt.Printf("load ini faild,err:%v\n", err)
		return
	}

	// fmt.Printf("%#v\n", cfg)

	//1. 启动服务
	// 定义 -c 和 -s 参数，用于选择执行 client 或 server
	localMode := flag.Bool("l", false, "Run as local proxy server")
	remoteMode := flag.Bool("r", false, "Run as remote proxy server")

	// 解析命令行参数
	flag.Parse()

	// 判断参数并执行对应函数
	if *localMode {
		local.RunServer(Cfg)
	} else if *remoteMode {
		remote.RunServer(Cfg)
	} else {
		fmt.Println("请只少输入一个参数， -l (近端代理服务器) or -r (远端代理服务器)")
	}

	// 保持主线程运行
	select {}
}
