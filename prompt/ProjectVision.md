-   -   -   ---
        
            ### **LiuProxy v2.0 - 最终开发计划 (第一部分)**
        
            **第一会话目标**: 梳理并确认**项目愿景**、**核心模块职责**和**最终的架构设计**。
        
            ---
        
            ### **1. 项目最终愿景 (Project Vision)**
        
            `LiuProxy v2.0`旨在成为一个**模块化、跨平台的统一代理解决方案**。它以一个强大的Go核心为基础，通过灵活的策略部署，既能作为**个人代理客户端**（PC端、移动端）独立运行，也能演进为一个为家庭或小型办公室提供服务的**多用户智能分流网关**。
        
            该系统通过支持**自研高性能协议**、**标准行业协议 (VLESS)**以及**Serverless后端 (Cloudflare Worker)**，为用户在不同网络环境下提供了兼具性能、灵活性和高可用性的选择。
        
            ---
        
            ### **2. 核心模块与最终职责定义**
        
            项目最终将包含以下核心模块：
        
            **a. `liuproxy-gateway` (最终的Local端程序)**
            *   **定位**: 系统的智能中枢和流量入口。
            *   **职责**:
                *   **多模式监听**: 同时支持**标准代理模式** (监听`unified_port`，处理SOCKS5/HTTP请求) 和**透明网关模式** (监听`transparent_port`，拦截局域网流量)。
                *   **流量解析**: 负责从两种模式的流量中解析出`客户端IP`和`原始目标地址`。
                *   **路由决策**: 作为**唯一的决策点**，根据路由策略（`PolicyMapping`）、会话粘滞状态和负载均衡算法，为每一个连接请求选择一个合适的后端隧道或决定直连。
                *   **DNS服务 (可选)**: 运行一个轻量级DNS服务器，用于域名劫持（Fake IP）以实现更精确的域名路由。
        
            **b. `liuproxy-remote` (Go Remote后端)**
            *   **定位**: 全功能、高性能的自建后端服务器。
            *   **职责**:
                *   作为自研`v2.2`多路复用协议的服务器端。
                *   处理来自`liuproxy-gateway`的TCP虚拟流和封装的UDP数据包。
                *   提供**完整的TCP和UDP代理能力**。
        
            **c. `Cloudflare Worker` (Worker后端)**
            *   **定位**: 轻量级、高可用、免维护的Serverless后端。
            *   **职责**:
                *   处理独立的WebSocket连接。
                *   **只提供TCP代理能力**。
                *   通过集成**WARP**来优化出站路由，规避特定服务的IP风控。
        
            **d. `liuproxy-android-app` (移动端)**
            *   **定位**: 安卓平台上的系统级VPN客户端。
            *   **职责**:
                *   通过`VpnService`和`hev-socks5-tunnel`捕获设备流量。
                *   将`liuproxy-gateway`核心作为库（`.aar`）集成，作为其SOCKS5上游。
                *   提供完整的UI，用于管理服务器配置（包括VLESS）、分应用代理等功能。
                *   **清晰的定位**: App本身是一个**独立的代理客户端**，当其VPN开启时，应通过网关的`DIRECT`策略被豁免，以避免路由冲突。
        
            ---
        
            ### **3. 最终架构设计**
        
            **a. 配置文件系统**:
            *   `configs/liuproxy.ini` (原`local.ini`):
                *   存储**程序行为**配置 (端口、模式、网关IP、日志级别等)。
                *   存储**路由策略** (`[PolicyMapping]`)。
                *   由**用户手动管理**。
            *   `configs/servers.json`:
                *   存储**服务器/隧道列表** (数据)。
                *   使用灵活的JSON格式，以支持VLESS等复杂协议。
                *   由**Web UI自动管理**。
        
            **b. 内部逻辑架构 (统一的`liuproxy-gateway`程序)**
        
            ```mermaid
            graph TD
                subgraph "启动与配置加载"
                    Start --> ReadIni[读取 liuproxy.ini];
                    ReadIni --> ReadJson[读取 servers.json];
                    ReadJson --> InitTunnelPool[初始化隧道池 TunnelPoolManager];
                    InitTunnelPool --> InitRouter[初始化路由决策器 Router];
                    InitRouter --> StartListeners["启动监听器(透明/标准)"];
                end
            
                subgraph "流量处理"
                    ClientTraffic[客户端流量] --> Listeners["监听器<br>(透明/标准代理)"];
                    Listeners -- "conn, clientIP, targetAddr" --> Router;
                    Router -- "1. 匹配策略" --> PolicyCheck{"策略匹配?"};
                    
                    PolicyCheck -- "是" --> SelectStaticTunnel[选择指定隧道];
                    SelectStaticTunnel --> GetTunnel[从隧道池获取隧道实例];
                    
                    PolicyCheck -- "否 (默认)" --> SessionCheck{"会话保持?"};
                    SessionCheck -- "是" --> SelectSessionTunnel[选择已有会-话的隧道];
                    SelectSessionTunnel --> GetTunnel;
                    
                    SessionCheck -- "否" --> LoadBalance[负载均衡选新隧道];
                    LoadBalance --> UpdateSession[更新会话粘滞表];
                    UpdateSession --> GetTunnel;
            
                    GetTunnel --> ForwardToTunnel[通过隧道转发];
                end
            ```
        
            ---
            **本会话总结**:
        
            我们已经明确了项目的最终愿景、各模块的核心职责以及一个统一、解耦的内部架构。双配置文件系统也被确立为最终方案。
        
            **下一步**:
        
            在下一个会话中，我们将基于这个已确认的架构，制定出详细的、分阶段的**开发迭代计划**，明确每个迭代要开发的功能、前置依赖和验收标准。
        
            请问您对以上**愿景、职责和架构**的梳理是否满意？如果满意，我们将进入下一个会话来制定具体的开发计划。