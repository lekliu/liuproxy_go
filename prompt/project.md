# LiuProxy（总体）

#### **1. 核心愿景**

构建一个统一、高效、跨平台的现代化代理解决方案，通过智能客户端适配两种能力不同的后端（全功能自建服务器与轻量级Serverless），为用户提供兼具**高性能**与**高灵活性**的网络访问能力。

#### **2. 客户端核心 (Go Local)**

-   **统一入口:** 在PC端，通过**单一端口**提供服务，该端口能**自动嗅探并处理HTTP和SOCKS5（TCP/UDP）**两种代理协议。
-   **双策略隧道:** Go Local必须实现一个**后端自适应的隧道管理器**，能够根据配置无缝切换两种工作模式：
    1.  **多路复用模式 (针对Go Remote):** 通过**单一、长期**的TCP或WebSocket连接，运行自定义的**v2.2多路复用协议**，高效地并发传输多个TCP流和所有UDP数据包。
    2.  **并发独立通道模式 (针对Cloudflare Worker):** 为**每一个**新的TCP代理请求，都建立一个**独立的、一次性**的WebSocket连接。此模式下**明确不支持UDP代理**。
-   **多平台支持:** 作为独立可执行文件在PC上运行，并通过gomobile编译为.aar库集成到Android APP中。

#### **3. 后端实现 (Remote)**

提供两种功能边界清晰、接口协议**在握手阶段理念一致**的后端选项。

**3.1 后端选项A：自建Go Remote服务器 (全功能后端)**

-   **支持协议:** **TCP 和 UDP**
-   **部署:** 用户可自行部署在任何VPS或服务器上（支持Docker）。
-   **工作模式:**
    -   作为**多路复用隧道**的服务器端，监听在单一TCP或WebSocket端口上。
    -   负责解析v2.2协议，管理所有虚拟流（Stream）和UDP数据，并代表客户端与最终目标服务器进行通信。

**3.2 后端选项B：Cloudflare Workers (轻量级TCP-Only后端)**

-   **支持协议:** **仅支持TCP**
-   **部署:** 用户可将脚本部署到Cloudflare Workers平台，无需管理服务器。
-   **工作模式:**
    -   作为**并发独立通道**的服务器端。
    -   每个Worker实例的生命周期仅对应一个TCP代理会话。
    -   其逻辑为：接收WebSocket连接 -> 读取元数据中的TCP目标地址 -> 建立出站TCP连接 -> 在两个连接间透明转发数据。
-   **边缘节点优选 :**
    -   **自动 (默认):** Go Local通过域名连接，Cloudflare自动路由。
    -   **手动 (可选):** Go Local配置中可**指定一个Cloudflare边缘节点IP**强制连接。

#### **4. Android APP端**

-   **系统级代理:** 通过Android VpnService实现全局或分应用的VPN代理。
-   **性能优化:**
    -   深度集成hev-socks-tunnel作为TUN层核心。
    -   **本地DNS处理:** 必须启用hev-socks-tunnel的mapdns功能，在**设备本地高效处理DNS查询**。
-   **核心集成:** 将Go Local核心作为SOCKS5服务器，由hev-socks-tunnel连接，形成完整的TUN -> Native SOCKS5 -> Go Core -> WebSocket Tunnel数据链路。
-   **UI适配:**
    -   服务器配置界面需允许用户选择后端类型（Go Remote / Cloudflare Worker）。
    -   当用户选择Cloudflare Worker后端时，UI应明确提示**“不支持UDP代理”**。

------



### **总结：清晰的边界**

这个最终版本定义了非常清晰的能力边界：

| 功能         | Go Remote 服务器 | Cloudflare Worker |
| ------------ | ---------------- | ----------------- |
| **TCP 代理** | 是               | 是                |
| **UDP 代理** | 是               | 否                |

这个模型非常实用：

-   **日常网页浏览、视频、下载**等基于TCP的应用，用户可以选择延迟更低的Cloudflare Worker后端。
-   需要进行**游戏、VoIP通话**等基于UDP的应用时，用户可以一键切换到功能更全面的Go Remote服务器后端。





## 子项目：LiuProxy (Go 版本)

### 一、 项目简介 

本项目旨在使用 Go 语言重构和现代化 liuproxy。新版本将专注于性能、可扩展性、易部署性和安全性，并引入对现代云原生部署（如 Cloudflare Worker）的支持。

### 二、 核心目标

1.  **高性能**: 利用 Go 语言的并发特性，实现高吞吐、低延迟的代理服务。
2.  **协议支持**:
    -   **本地代理协议**: SOCKS5 (TCP & UDP) 和 HTTP。
    -   **隧道协议**: 自定义的 v2.2 多路复用协议，运行在 WebSocket (WSS) 之上。
3.  **双策略隧道 (Dual-Strategy Tunneling)**:
    -   **Go Remote 模式**: local 与 remote 之间建立一个**长期、单一**的 WebSocket 连接，所有用户会话在此连接上进行多路复用。此模式下，remote 端需要一台独立的服务器。
    -   **Cloudflare Worker 模式 (未来)**: local 端为**每个**用户会话创建一个**短期、并发**的 WebSocket 连接，通过 Cloudflare 全球网络连接到目标。此模式为 Serverless 架构，无需独立服务器。
4.  **配置简单**: 使用 .ini 文件进行清晰、简单的配置。

### 三、 核心设计 

#### 隧道协议统一 (Unified Tunnel Protocol) - [已更新]

与旧版liuproxy不同，Go版本将**所有隧道流量统一承载于 WebSocket (或 WSS) 之上**。这带来了几个核心优势：

-   **单一入口**: remote服务器只需暴露一个端口（例如443），简化了防火墙配置和部署。
-   **高穿透性**: WSS流量与标准HTTPS流量外观一致，能有效穿透大多数网络限制。
-   **架构一致性**: 无论是连接Go-Remote还是Cloudflare Worker，local端都使用相同的底层传输协议，只是会话管理模型不同。

自定义的 **v2.2 协议** 运行在 WebSocket 的二进制消息帧之上，负责流的复用、控制信令和UDP数据的封装。

#### Local 端设计 - [已更新]

local端是整个系统的智能调度中心，其核心组件包括：

1.  **统一入口与协议嗅探 (Dispatcher)**:
    -   在本地监听一个**统一端口** (unified_port)，例如 9088。
    -   当收到新连接时，Dispatcher会“嗅探”连接的第一个字节，以判断是SOCKS5流量 (0x05)还是HTTP流量。
2.  **协议转换层 (Agent)**:
    -   **SOCKS5 Agent**: 直接处理SOCKS5协议握手。
    -   **HTTP Agent**: 负责解析HTTP CONNECT请求，提取目标地址，然后**将其转换为一个标准的TCP流创建请求**，行为上模拟成一个SOCKS5客户端。
3.  **隧道管理器 (TunnelManager)**:
    -   这是local端的核心。它是一个**全局单例**，负责维护与remote服务器池的连接。
    -   **服务器池与故障转移**: TunnelManager从配置中读取一个remote服务器列表，并实现自动故障转移。当一个连接断开时，它会自动尝试连接列表中的下一个可用服务器。
    -   所有Agent（无论是SOCKS5还是HTTP）最终都会通过这个TunnelManager来创建新的虚拟流（Stream）。
4.  **UDP管理器 (UDPManager)**:
    -   
    -   同样是**全局单例**，负责处理SOCKS5的UDP ASSOCIATE命令。
    -   它在本地创建一个UDP端口，监听来自客户端的UDP数据包，并通过TunnelManager将这些数据包封装后发送出去。

#### Remote 端设计 - [已更新]

remote端的设计目标是简洁和高效：

1.  **单一WebSocket监听器**: remote端只监听一个WebSocket路径（例如 /tunnel）。
2.  **协议解复用**: 当收到新的WebSocket连接后，它将这个连接视为一个长期的多路复用隧道。RemoteTunnel的读循环会不断读取v2.2协议包。
3.  **会话管理**:
    -   收到NewStream请求后，RemoteSessionManager会为之建立一个到最终目标的TCP连接。
    -   收到UDPData包后，RemoteUDPRelay负责将其转发到最终目标，并管理NAT映射以便回传数据。
4.  **无状态感知**: remote端是无状态的，它不关心流量的原始协议是SOCKS5还是HTTP，它只处理标准的v2.2协议包。