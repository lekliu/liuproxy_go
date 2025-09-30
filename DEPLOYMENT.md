# LiuProxy v2.0 - 部署与使用指南

本文档提供了将 `LiuProxy v2.0` 部署为独立服务的详细步骤。系统支持两种主要的部署方式：

1.  **Docker 部署 (推荐)**: 最简单、最可靠的方式，将整个应用打包在一个隔离的环境中运行。
2.  **二进制部署**: 适用于不希望使用 Docker 的环境，需要手动编译和管理进程。

---

## 1. Docker 部署 (推荐)

通过 Docker Compose，您可以用一条命令完成服务的部署和启动。

### 1.1. 前提条件

*   已安装 [Docker](https://docs.docker.com/engine/install/)
*   已安装 [Docker Compose](https://docs.docker.com/compose/install/)

### 1.2. 目录结构

在您的服务器上，创建如下的目录结构：

```
/opt/liuproxy/
├── docker-compose.yml
└── configs/
    ├── liuproxy.ini
    ├── servers.json
    └── routing_rules.json
```

*   **`docker-compose.yml`**: 复制项目根目录下的同名文件到这里。
*   **`configs/`**: 将您本地项目中的 `configs` 目录完整复制到这里。
    *   **`liuproxy.ini`**: 这是您的主配置文件。
    *   **`servers.json`**: 如果您已有服务器列表，可以放在这里。如果不存在，系统首次启动时会创建一个空的。
    *   **`routing_rules.json`**: 路由规则文件，如果不存在，系统会使用默认规则。

### 1.3. 配置 `liuproxy.ini`

在启动前，请务必检查并修改 `/opt/liuproxy/configs/liuproxy.ini` 文件。

一个推荐的最小化配置如下：

```ini
[common]
mode           = local
maxConnections = 16
bufferSize     = 4096
crypt          = 125

[local]
unified_port = 9088
web_port     = 8081
; 强烈建议首次部署时设置用户名和密码
web_user     = your_admin_user
web_password = your_secure_password

[log]
level = info ; 生产环境建议使用 info 级别

[Gateway]
sticky_session_ttl = 300
```

**重要**:
*   确保 `unified_port` 和 `web_port` 没有被服务器上的其他应用占用。
*   为了安全，**务必设置 `web_user` 和 `web_password`**。

### 1.4. 启动服务

在 `/opt/liuproxy/` 目录下，执行以下命令：

```bash
docker-compose up -d
```

该命令会：
1.  在后台拉取或构建 `liuproxy` 的 Docker 镜像。
2.  创建一个名为 `liuproxy_local_service` 的容器。
3.  将本地的 `./configs` 目录挂载到容器的 `/app/configs`。
4.  将容器的 `9088` 和 `8081` 端口映射到主机的 `127.0.0.1`，这意味着只有本机可以访问。

### 1.5. 验证与管理

*   **查看日志**: `docker-compose logs -f`
*   **停止服务**: `docker-compose down`
*   **重启服务**: `docker-compose restart`

---

## 2. 二进制部署

适用于您希望直接在主机上运行 Go 程序的环境。

### 2.1. 前提条件

*   已安装 Go 1.24 或更高版本。
*   Linux 或 macOS 操作系统。

### 2.2. 编译

1.  克隆项目代码到您的服务器。
2.  在项目根目录 (`liuproxy_go/`) 下，执行编译命令：

    ```bash
    go build -o liuproxy-go ./cmd/local
    ```

    这将在当前目录下生成一个名为 `liuproxy-go` 的可执行文件。

### 2.3. 目录结构

确保您的目录结构如下：

```
/path/to/your/deployment/
├── liuproxy-go          # 编译好的二进制文件
└── configs/
    ├── liuproxy.ini
    ├── servers.json
    └── routing_rules.json
```

### 2.4. 启动服务

在部署目录下 (`/path/to/your/deployment/`)，执行以下命令启动服务：

```bash
./liuproxy-go --configdir=configs
```

*   `--configdir=configs` 参数告诉程序去当前目录下的 `configs` 文件夹中寻找配置文件。

为了让服务在后台持续运行，建议使用 `systemd` 或 `supervisor` 等进程管理工具。

---

## 3. 初次使用

部署并启动服务后：

1.  **访问 Web UI**: 打开浏览器，访问 `http://<您的服务器IP>:8081`。
2.  **登录**: 如果您在 `liuproxy.ini` 中配置了 `web_user` 和 `web_password`，浏览器会弹出认证窗口，请输入凭据登录。
3.  **添加服务器**:
    *   点击 "Add New Server" 按钮。
    *   填写您的远程服务器信息（如 VLESS 或 GoRemote 的配置）。
    *   `Local Port` 字段可以填写 `0`，让系统自动分配一个可用端口。
    *   点击 "Save"。
4.  **激活服务器**: 在服务器列表中，找到您刚刚添加的服务器，点击 "Activate" 按钮。
5.  **配置客户端**:
    *   将您的系统或浏览器的代理设置为 **SOCKS5** 或 **HTTP** 代理。
    *   服务器地址: `<您的服务器IP>`
    *   端口: `9088` (或您在 `liuproxy.ini` 中设置的 `unified_port`)
6.  **开始使用**: 现在，您的网络流量将通过 `LiuProxy` 进行代理。您可以在 Web UI 的日志面板中看到实时的系统状态信息。