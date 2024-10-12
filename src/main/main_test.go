package main

import (
	"fmt"
	"main/conf"
	"main/local"
	"main/remote"
	"testing"
)

func TestLocal(t *testing.T) {
	err := conf.LoadIni(Cfg, "../../conf/config.ini")
	if err != nil {
		fmt.Printf("load ini faild,err:%v\n", err)
		return
	}
	fmt.Println(Cfg)
	local.RunServer(Cfg)
	select {}
}

func TestRemote(t *testing.T) {
	err := conf.LoadIni(Cfg, "../../conf/config.ini")
	if err != nil {
		fmt.Printf("load ini faild,err:%v\n", err)
		return
	}
	fmt.Println(Cfg)
	remote.RunServer(Cfg)
	select {}
}
