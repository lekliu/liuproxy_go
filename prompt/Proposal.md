好的，我们正式启动 **阶段五：底层网络与透明代理**。

---

### **方案 (Proposal) - 迭代 5.1: 核心架构重构与技术验证**

#### **1. 目标 (Goal)**

本次迭代的核心目标有两个，且**严格分离**：

1.  **内部架构重构 (IOC - Inversion of Control)**: 解除 `Gateway` 与 `Strategy` 实例之间通过本地TCP端口的硬耦合。我们将重构系统，使得 `Dispatcher` 可以直接将客户端连接的**控制权**移交给策略实例，从而彻底移除 `localPort` 的概念，提升性能并简化配置。
2.  **技术可行性验证 (PoC - Proof of Concept)**: 在一个**独立的、最小化的** Go 程序中，验证在 Linux Docker 环境下，通过 `iptables` 和 `getsockopt` 系统调用来**拦截透明TCP流量并获取其原始目标地址**的核心技术是可行的。

**本次迭代不开发任何最终用户功能**，而是为下一阶段的 `TransparentGateway` 和防火墙开发，扫清所有架构和技术上的障碍。

#### **2. 技术方案 (Technical Plan)**

##### **第一部分：内部架构重构**

我们将按照以下顺序，对现有项目进行一次“心脏搭桥手术”。

1.  **定义 `ConnectionHandler` 接口**:
    *   在 `internal/types/types.go` 中，定义一个新的接口，它代表了任何可以处理一个网络连接的实体。
        ```go
        // ConnectionHandler defines a unified interface for handling a network connection.
        type ConnectionHandler interface {
            Handle(conn net.Conn)
        }
        ```

2.  **重构 `Strategy` 实例**:
    *   修改 `TunnelStrategy` 接口 (`internal/types/types.go`)，使其继承 `ConnectionHandler`。
    *   修改所有具体的策略实现 (`vless_strategy_native.go`, `goremote_strategy.go`, `worker_strategy.go`):
        *   移除所有与 `net.Listener` 相关的代码（`Initialize` 不再监听端口，`acceptLoop` 被移除）。
        *   实现 `Handle(conn net.Conn)` 方法。该方法将包含原 `acceptLoop` 中处理单个连接的逻辑（如SOCKS5握手、与远程服务器建立隧道、双向转发数据等）。

3.  **重构 `Dispatcher`**:
    *   修改 `RouteInfo` 结构体 (`internal/dispatcher/dispatcher.go`)。它的 `TargetAddr` 字段将不再是字符串，而是 `types.ConnectionHandler` 接口类型。
    *   `updateRoutingTables` 方法在处理规则时，如果目标是一个代理后端，它将从 `stateProvider` 获取对应的**策略实例对象引用**，并存入 `RouteInfo`。
    *   `Dispatch` 方法在决策成功后，返回的将是 `(types.ConnectionHandler, error)`，而不是 `(string, string, error)`。

4.  **适配 `Gateway`**:
    *   修改 `Gateway` 的 `handleConnection` 方法。当它从 `Dispatcher` 收到一个 `ConnectionHandler` 实例时，它将不再 `net.Dial` 一个本地端口，而是直接调用 `handler.Handle(inboundConn)`，将客户端连接的控制权完全移交。

##### **第二部分：透明代理技术验证 (PoC)**

我们将创建一个**新的、独立的 Go 程序** (`test/transparent_poc/main.go`) 和一个配套的 `iptables` 脚本。

1.  **Go PoC 程序**:
    *   程序将监听一个端口，例如 `12345`。
    *   当接收到一个TCP连接时，它会使用 `syscall` 包和 `getsockopt` 调用 `SOL_IP`, `SO_ORIGINAL_DST` (for IPv4) 或 `SOL_IPV6`, `IP6T_SO_ORIGINAL_DST` (for IPv6) 来获取该连接的原始目标IP和端口。
    *   成功获取后，将 "Accepted connection from [源地址] originally destined for [原始目标地址]" 打印到控制台，然后关闭连接。

2.  **`iptables` 脚本 (`setup_tproxy.sh`)**:
    *   该脚本将包含必要的 `iptables` 命令，用于将所有出站的 TCP 流量（例如，到 80 端口）重定向到 Go PoC 程序监听的 `12345` 端口。

#### **3. 验收标准 (Acceptance Criteria)**

1.  **重构验收**:
    *   项目可以正常编译和运行。
    *   `servers.json` 中不再需要 `localPort` 字段。
    *   通过配置浏览器或 `curl` 使用标准的SOCKS5/HTTP代理（指向 `9088`），所有路由和代理功能与重构前**表现完全一致**。

2.  **PoC 验收**:
    *   在一个Linux环境中，先运行 `setup_tproxy.sh` 脚本，再运行 Go PoC 程序。
    *   在该环境中执行 `curl http://example.com`。
    *   Go PoC 程序的控制台必须成功打印出类似 `...originally destined for 93.184.216.34:80` 的日志。

---

### **方案 (Proposal) - 迭代 5.2: 透明网关与防火墙实现**

#### **1. 目标 (Goal)**

在迭代 5.1 成功完成架构重构和技术验证的基础上，本次迭代的目标是将这些能力**产品化**，正式在 `LiuProxy` 项目中实现一个功能性的、可配置的**透明网关 (Transparent Gateway)** 和一个与之配套的**L4防火墙**。

最终产物将是一个能够在 Docker 中运行，并根据 `settings.json` 配置文件，对透明流量执行“**先防火墙过滤，后智能路由**”的网关服务。

#### **2. 技术方案 (Technical Plan)**

##### **第一部分：防火墙 (Firewall) 功能实现**

1.  **扩展配置 (`settings.json`)**:
    *   在 `settings.json` 中新增一个顶层模块 `"firewall"`。
    *   **文件**: `configs/settings.json`
        ```json
        {
          "gateway": { ... },
          "routing": { ... },
          "firewall": {
            "enabled": true,
            "rules": [
              { "priority": 10, "protocol": "tcp", "dest_port": "22", "action": "deny" },
              { "priority": 20, "source_cidr": "192.168.1.100/32", "action": "deny" },
              { "priority": 999, "action": "allow" } 
            ]
          }
        }
        ```
    *   **`FirewallRule` 结构体**: 将包含 `priority`, `protocol`, `source_cidr`, `dest_cidr`, `dest_port` (支持范围，如 "80,443,1000-2000"), 和 `action ("allow", "deny")`。

2.  **实现防火墙引擎**:
    *   在 `internal/firewall` 包中，创建一个 `Engine`。
    *   `Engine` 在初始化时加载防火墙规则，并根据 `priority` 进行排序。
    *   提供一个核心方法 `Check(connMetadata)`，输入连接的元数据（协议、源/目标IP、目标端口），返回 `ALLOW` 或 `DENY` 的决策。
    *   该引擎需要高效地处理IP CIDR匹配和端口范围匹配。

##### **第二部分：透明网关 (Transparent Gateway) 实现**

1.  **创建 `TransparentGateway` 模块**:
    *   在 `internal/gateway` 目录下，创建一个新的 `transparent_gateway.go` 文件。
    *   `TransparentGateway` 结构体将包含对 `FirewallEngine` 和 `Dispatcher` 的引用。

2.  **实现监听与流量拦截**:
    *   `TransparentGateway` 将在 `AppServer` 中被初始化。
    *   它会启动一个 `net.Listener`，监听一个专门用于接收重定向流量的端口（例如，在 `liuproxy.ini` 中配置一个新的 `tproxy_port`）。
    *   在 `acceptLoop` 中，对于每一个新接受的 `net.Conn`，它会执行以下操作：
        a.  调用在**迭代 5.1 PoC**中验证过的、与平台相关的函数，来获取连接的**原始目标地址**。
        b.  如果获取失败，则记录错误并关闭连接。

3.  **集成处理流程 (Firewall -> Dispatcher)**:
    *   成功获取原始目标地址后，`TransparentGateway` 将构造一个包含连接元数据的对象。
    *   **第一步 - 防火墙检查**: 调用 `firewallEngine.Check()`。如果返回 `DENY`，则记录日志并立即关闭连接。
    *   **第二步 - 智能分流**: 如果防火墙检查通过 (`ALLOW`)，则将元数据传递给**现有的 `dispatcher.Dispatch()`** 方法。
    *   **第三步 - 执行决策**: 根据 `Dispatcher` 的返回结果，执行相应的操作：
        *   `DIRECT`: `TransparentGateway` 直接连接原始目标，并双向转发数据。
        *   `REJECT`: 关闭连接。
        *   `ConnectionHandler` 实例: 调用 `handler.Handle(conn)`，将连接的控制权移交给对应的策略实例。

##### **第三部分：UI & 配置**

1.  **Web UI (`index.html`, `settings.js`)**:
    *   在导航栏中新增一个 “Firewall” 菜单项。
    *   创建一个新的 `<main id="main-firewall">` 页面。
    *   在此页面上，实现对 `firewall.rules` 的完整可视化CRUD（增删改查）管理界面，功能类似于我们已经实现的 "Routing Rules" 界面。
    *   增加一个总开关，用于启用或禁用整个防火墙模块 (`firewall.enabled`)。
2.  **`SettingsManager`**:
    *   扩展 `SettingsManager` 以识别和处理新的 `"firewall"` 模块。
    *   `AppServer` 将订阅 `"firewall"` 模块的变更，并在收到更新时，通知 `FirewallEngine` 重新加载规则。

---

#### **3. 验收标准 (Acceptance Criteria)**

1.  **防火墙功能**:
    *   在UI上添加一条 `deny` 规则（例如，`dest_port: 22`）。通过透明网关 `ssh` 任何主机都应该失败。
    *   在UI上添加一条 `allow` 规则，并确保流量可以正常通过。
    *   所有防火墙活动都应有清晰的日志记录。
2.  **透明代理与分流集成**:
    *   在UI上配置一条防火墙 `allow` 规则和一条路由 `routing` 规则（例如，`domain: *.google.com -> target: MyProxy`）。
    *   通过透明网关访问 `google.com`，流量应被 `MyProxy` 策略实例代理。
    *   通过透明网关访问 `example.com`（假设无匹配规则），流量应被默认负载均衡策略处理。
3.  **配置热重载**:
    *   在系统运行时，通过UI修改任何防火墙规则，应立即生效，无需重启服务。

---


### **方案 (Proposal) - 迭代 5.3: 流量监控、容器化与文档**

#### **1. 目标 (Goal)**

在迭代 5.2 成功实现了透明网关和防火墙的核心功能后，本次迭代的目标是将这个系统**产品化**。我们将：
1.  **实现实时流量监控**: 为用户提供一个可视化的窗口，观察通过网关的网络活动。
2.  **完成Docker化部署**: 创建一个健壮、可移植的 Docker 镜像，简化部署流程。
3.  **撰写详尽的文档**: 确保用户能够理解如何部署、配置和使用这个强大的新网关模式。

最终产物将是一个功能完整、易于部署、文档齐全的网络网关解决方案。

#### **2. 技术方案 (Technical Plan)**

##### **第一部分：实时流量监控**

1.  **后端 WebSocket API**:
    *   **引入 `gorilla/websocket` 库**: 我们将在 `internal/web` 模块中引入这个行业标准的WebSocket库。
    *   **创建新的API端点**: 在 `web/handler.go` 中，新增一个处理器，路径为 `GET /api/traffic/live`。
    *   **实现 `Hub` 模式**: 这个处理器会将HTTP请求升级为WebSocket连接。我们将实现一个中央的 `Hub`，负责管理所有连接的客户端。当 `TransparentGateway` 或 `Gateway` 处理一个新的连接时，它会生成一条连接日志（一个 `struct`），并将这条日志广播给 `Hub`，再由 `Hub` 推送给所有连接的Web UI客户端。
    *   **数据结构**: 定义 `TrafficLogEntry` 结构体，包含 `Timestamp`, `Source`, `Destination`, `Protocol`, `MatchedRule`, `Action`, `Backend` 等字段，用于前后端数据交换。

2.  **前端 "Monitor" 页面**:
    *   **HTML (`index.html`)**: 在导航栏增加 "Monitor" 链接，并创建一个新的 `<main id="main-monitor">` 页面。页面主体可以是一个可滚动的日志面板。
    *   **JavaScript (`monitor.js`)**: 创建一个新的JS文件。
        *   当用户切换到 "Monitor" 页面时，它会建立到 `/api/traffic/live` 的WebSocket连接。
        *   它会监听 `onmessage` 事件，每当从后端收到一个新的 `TrafficLogEntry` JSON对象时，就将其格式化为一行日志，并动态地添加到日志面板的顶部。
        *   提供“暂停/继续”和“清空”按钮，方便用户观察。

##### **第二部分：容器化 (Dockerization)**

1.  **编写 `Dockerfile`**:
    *   **基础镜像**: 使用一个轻量级的Linux发行版，如 `alpine:latest`，因为它包含了 `iptables`。
    *   **多阶段构建**:
        *   第一阶段 (`builder`): 使用 `golang:alpine` 镜像来编译我们的 `liuproxy-go` 二进制文件。
        *   第二阶段 (最终镜像): 从 `alpine:latest` 开始，仅复制编译好的二进制文件、`entrypoint.sh` 脚本和默认的 `configs` 目录。
    *   **权限**: `Dockerfile` 本身不处理 `NET_ADMIN`，这将在 `docker-compose.yml` 中指定。

2.  **编写 `entrypoint.sh` 启动脚本**:
    *   这个脚本是容器启动时执行的核心。
    *   **职责**:
        a.  检查必要的环境变量（如果我们需要通过环境变量来配置 `iptables`）。
        b.  **启用内核IP转发**: `sysctl -w net.ipv4.ip_forward=1`。
        c.  **配置 `iptables`**: 执行一系列 `iptables` 命令，将容器的入站流量重定向到 `TransparentGateway` 的监听端口。这是实现透明代理的关键。
            *   例如: `iptables -t nat -A PREROUTING -p tcp --dport 80 -j REDIRECT --to-port 12345`
        d.  **启动 `liuproxy-go` 程序**: `exec /app/liuproxy-go --configdir /app/configs`。

3.  **提供 `docker-compose.yml` 示例**:
    *   这将是用户最常用的部署方式。
    *   **关键配置**:
        *   `image: liuproxy/gateway:latest`
        *   `container_name: liuproxy-gateway`
        *   `network_mode: "host"`: **这是最简单、最推荐的方式**。它让容器直接使用主机的网络栈，性能最好，且能无缝地作为局域网的网关。
        *   `cap_add: - NET_ADMIN`: **必须添加**，给予容器配置网络所需的权限。
        *   `volumes`: 将主机的 `configs` 目录挂载到容器的 `/app/configs`。
        *   `restart: unless-stopped`

##### **第三部分：文档 (Documentation)**

1.  **更新 `DEPLOYMENT.md`**:
    *   **新增 "网关模式部署 (推荐)" 章节**: 这一章将成为文档的核心。
    *   **详细步骤**:
        1.  前提条件（Linux主机, Docker, Docker Compose）。
        2.  如何准备 `docker-compose.yml` 和 `configs` 目录。
        3.  如何配置 `liuproxy.ini` 和 `settings.json`（特别是防火墙和路由规则）。
        4.  一条命令启动服务：`docker-compose up -d`。
        5.  **如何配置局域网**: 提供清晰的截图或文字说明，指导用户如何在他们的**主路由器**的DHCP设置中，将“默认网关”地址改为运行Docker的主机的IP地址。或者，如何在一台设备上手动设置网关。
        6.  验证与管理（查看日志、访问UI等）。
    *   **Windows 局限性说明**: 在文档末尾，用一个明确的警告框，解释为什么不推荐在Windows Docker (WSL 2)环境下使用此模式作为局域网网关，并说明其网络隔离的技术原因。

---

#### **3. 验收标准 (Acceptance Criteria)**

1.  **监控功能**: "Monitor" 页面能够实时、准确地显示通过网关的连接，并且不影响系统性能。
2.  **Docker部署**: 用户可以严格按照 `DEPLOYMENT.md` 中的 `docker-compose.yml` 示例，在**一台干净的Ubuntu服务器**上，用一条命令成功部署并运行一个功能完整的透明网关。
3.  **端到端测试**: 局域网内的任何一台**未做任何代理配置**的设备（如手机），在将其网关指向Docker主机后，其网络流量应能被 `LiuProxy` 成功拦截、防火墙过滤、智能分流，并能在 "Monitor" 页面上看到其活动。
4.  **文档清晰度**: 一个不熟悉项目的新用户，能够仅凭 `DEPLOYMENT.md` 文档，成功完成部署和基本配置。
