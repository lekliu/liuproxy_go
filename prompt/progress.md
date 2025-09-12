### **LiuProxy v2.0 总体开发计划 (状态更新)**

**总览:** 项目分为四个主要阶段。我们将按迭代推进，每个迭代都有明确的验收标准。

### **当前进度**

-   ✅ **阶段一：Go核心重构与PC端功能统一**
-   ✅ **阶段二：Android APP集成与VPN链路打通**
-   ✅ **阶段三：Cloudflare Workers后端实现**
    -   ✅ **迭代 3.1 & 3.2**: Worker脚本开发、部署、本地端适配。
    -   ✅ **迭代 3.3**: Android APP端集成Worker (UI、配置、API对接、分应用代理)。

### **下一步：阶段四：优化、测试与发布**

**目标**: 对整个项目进行全面的功能回归测试、性能优化和错误处理完善，确保软件的稳定性、健壮性和用户体验，为最终的发布做好准备。

这个阶段将分为两个迭代进行。

------

### **迭代 4.1: 完整功能回归测试与错误处理**

**目标**: 确保在引入Worker策略和分应用代理后，所有原有的功能（特别是Go Remote相关的功能）仍然能正常工作，并增强应用的错误处理能力。

**实施方案**:

1.  **Go Remote后端功能回归测试**:
    -   **PC端**:
        -   修改local.ini，重新启用Go Remote后端。
        -   **验收**: 验证PC端的SOCKS5 TCP、HTTP代理和**SOCKS5 UDP**代理是否仍然能通过Go Remote正常工作。
    -   **Android端**:
        -   在App中创建一个"Go Remote"类型的服务器配置。
        -   启动VPN。
        -   **验收**:
            -   验证TCP流量（浏览网页）是否正常。
            -   使用内置的"UDP Echo Test"功能，验证**UDP流量**是否仍然能正常通过Go Remote后端。
2.  **分应用代理功能完整测试**:
    -   **白名单模式**:
        -   启用分应用代理，选择"Proxy Mode (Whitelist)"。
        -   只勾选一个浏览器应用（如Chrome）。
        -   **验收**: 启动VPN后，只有Chrome可以上网，其他应用（如另一个浏览器或App）无法连接网络。
    -   **黑名单模式**:
        -   启用分应用代理，选择"Bypass Mode (Blacklist)"。
        -   只勾选一个浏览器应用（如Chrome）。
        -   **验收**: 启动VPN后，Chrome无法上网（流量被绕行），而其他所有应用都可以正常上网。
3.  **UI与错误处理完善**:
    -   **问题**: 当前如果WebSocket连接失败（例如Worker URL错误、网络不通），Go核心会报错，但App UI上可能没有明确提示，只是停留在“连接中”或直接失败。
    -   **方案**:
        -   在Go Mobile API (mobile/api.go) 的StartVPN中，增加对server.New()和appServer.RunMobile()返回错误的更详细处理。
        -   在Android的LiuProxyServiceManager中，捕获Mobile.startVPN可能抛出的异常，并将更具体的错误信息（如“无法连接到远程服务器”）通过广播传递给UI。
        -   在MainViewModel和MainActivity中，接收这些详细的错误广播，并通过Toast向用户显示。
    -   **验收**:
        -   故意配置一个错误的服务器地址。
        -   启动VPN时，App界面应能弹出一个清晰的错误提示，而不是简单地失败或无响应。

------

### **迭代 4.2: 性能调优与发布准备**

**目标**: 优化App的资源占用，监控电池消耗，并为最终打包发布做准备。

**实施方案**:

1.  **Go核心资源优化**:
    -   **问题**: 当前Go核心的日志级别在hev-socks5-tunnel.yaml中硬编码为debug，这在生产环境中会产生大量不必要的日志，影响性能。
    -   **方案**:
        -   在TProxyService.kt的buildConfig()方法中，增加一个逻辑：从SettingsManager读取一个用户可配置的日志级别（例如在设置中增加一个选项），并将其写入.yaml文件。默认应为warn或error。
        -   审查Go代码中的goroutine生命周期，确保没有goroutine泄漏。
2.  **Android App性能监控**:
    -   **任务**: 使用Android Studio的Profiler工具，在VPN长时间运行的情况下，监控App的CPU、内存和电池消耗。
    -   **验收**:
        -   在后台待机状态下，App的CPU占用应接近于0。
        -   没有明显的内存泄漏迹象。
        -   电池消耗在一个合理的范围内（这部分主观性较强，但应避免成为耗电大户）。
3.  **构建与发布准备**:
    -   **任务**:
        -   检查并完善app/build.gradle.kts中的versionCode和versionName。
        -   生成签名的Release APK。
        -   （可选）为不同的CPU架构（arm64-v8a, armeabi-v7a等）生成分包APK。我们之前的build.gradle.kts配置已经支持了这一点。
    -   **验收**: 能够成功构建出可以安装和运行的签名版APK。