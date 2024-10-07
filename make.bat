cd src

SET GOOS=windows
SET GOARCH=amd64
go build -o ../bin/window/liuproxy.exe

SET GOOS=linux
SET GOARCH=amd64
go build -o  ../bin/linux/liuproxy-linux

pause


