# ShareBox 开发状态（实际）

> 最后更新：2026-06-30 | **架构已转向：去掉 Hub，改用 syncthing 原生模型**
> 新设计：`docs/plans/2026-06-30-no-hub-redesign.md`。下表"设计文档功能对照"基于
> 旧三层架构，Hub/家庭组相关行已不再是目标（标注"⛔ 改方向"），保留仅作历史记录。

---

## 一、设计文档功能对照（旧架构，部分已改方向）

来源：`docs/plans/2026-06-18-family-fileshare-design.md`（架构部分已被取代）

| # | 设计功能 | 状态 | 说明 |
|---|---------|------|------|
| 1 | Hub 协调服务（用户/组管理、目录注册、信令） | 🗑️ 已删除 | `cmd/sharehub`+`internal/hub` 已移除，改用 syncthing 原生 |
| 2 | Agent 设备代理 | ⛔ 不做 | 复用 syncthing 实例 |
| 3 | 前端 Web UI - 配对面板 | 🟡 代码完成未测 | 已改为"本机二维码 + 粘贴 ID 添加设备 + 介绍人"（替代建组/邀请码） |
| 4 | 前端 Web UI - 目录浏览 | ✅ 可用 | ShareBox 页面 `/sharebox`（保留） |
| 5 | 前端 Web UI - 拖拽上传 | ✅ 可用 | 同上（保留） |
| 6 | 前端 Web UI - 下载 | ❌ 不做 | 故意不做（见 todo T3） |
| 7 | 家庭组（设备注册目录，组内互相可见） | 🟡 代码完成未测 | 由"家庭清单文件夹"实现，零服务器；后端 `/rest/sharebox/manifest` + 前端聚合面板 |
| 8 | 文件只在宿主一份，不自动同步 | 🟡 已接线未测 | pull 是 no-op；发送端投送给所有对端后自动删本机副本（30s 周期）；宿主端自动接受文件夹强制 `ignoreDelete=true` 保留文件——闭环已实现，跨设备未实测（保留） |
| 9 | P2P 直连传输 | ⚠️ 底层通 | syncthing 管道可用，配对改走原生扫码 + introducer |
| 10 | 多设备跨网络传输 | ❌ 未测试 | 缺第二台设备验证 |

---

## 二、实现计划对照

来源：`docs/plans/2026-06-18-remoteaccess-implement.md`

| # | Task | 状态 | 备注 |
|---|------|------|------|
| 1 | Protobuf 枚举 FOLDER_TYPE_REMOTE_ACCESS | ✅ | `proto/bep/bep.proto` |
| 2 | 生成 Go 代码 | ✅ | `internal/gen/bep/bep.pb.go` |
| 3 | 协议常量和配置常量 | ✅ | `lib/protocol/`, `lib/config/` |
| 4 | `folder_remoteaccess.go` | ✅ | 168 行，no-op pull + Stage/Cleanup |
| 5 | 健康检查跳过 | ✅ | `lib/model/folder.go` |
| 6 | ClusterConfig 映射 | ✅ | `lib/model/model.go` |
| 7 | Upload API + GUI 入口 | ⚠️ | API 通，GUI 只有最小改动 |
| 8 | **GUI 文件浏览器 + 上传 UI** | ✅ | ShareBox 独立页面（替换原最小改动） |
| 9 | 集成测试 | ❌ | 未做 |

---

## 三、验证清单对照

来源：`docs/verification-checklist.md`

| # | 验证项 | 状态 | 说明 |
|---|--------|------|------|
| 1-7 | 自动化验证 | ✅ | 构建、Release、API 路由等 |
| 8 | 单机启动、Web UI 可访问 | ✅ | 已验证 |
| 9 | 创建 RemoteAccess 文件夹 | ✅ | API 创建 ✅，ShareBox 页面创建 ✅ |
| 10 | **两台设备配对连接** | 🔴 未测 | 需要第二台设备 |
| 11 | **远端文件列表（不下载）** | 🔴 未测 | 需要对端有文件 |
| 12 | 上传文件 API | ✅ | curl 验证通过 |
| 13 | **上传后对端可见** | 🔴 未测 | 需要第二台设备 |
| 14 | **暂存清理** | 🟡 已接线未测 | 后台 30s 周期 + 全对端确认收到后删除，需双设备验证 |
| 15 | **不下载（RemoteAccess 路径无对端文件）** | 🔴 未测 | 需要第二台设备 |
| 16 | **普通文件夹同步回归** | 🔴 未测 | - |
| 17 | **Linux/macOS 平台** | 🔴 未测 | 无对应环境 |
| 18 | **异常处理** | 🔴 未测 | - |

---

## 四、当前可测功能清单

| 功能 | 如何测 | 结果 |
|------|--------|------|
| 启动服务 | `syncthing.exe serve --home _test_home` | ✅ |
| 打开 ShareBox 页面 | `http://127.0.0.1:4398/sharebox` | ✅ |
| 创建文件夹（含目录选择器） | 点 + → 浏览本地目录 → 创建 | ✅ |
| 浏览文件 | 点击文件夹 → 文件列表 | ✅ |
| 上传文件（拖拽/点击） | 拖文件到上传区 | ✅ |
| 上传是否落盘 | 检查对应目录 | ✅ |
| 下载文件 | 点下载按钮 | ❌ 只显示信息，不下载 |

---

## 五、当前差距与下一步（架构转向后）

| 类别 | 现状 |
|------|------|
| 架构 | 无 Hub 方案已实现：syncthing 原生配对 + 家庭清单文件夹（`docs/plans/2026-06-30-no-hub-redesign.md`） |
| 已实现 | RemoteAccess 后端、浏览/上传、源端不留副本闭环、本机身份+二维码、添加设备、收件箱（自动共享+ignoreDelete）、清单读写+全家聚合、投送通道 |
| Hub | 已删除（`cmd/sharehub`、`internal/hub`、前端组面板） |
| 测试 | **未编译（本机无 Go）、多设备场景零验证**，需两台机器实测 |

**结论：去 Hub 改造代码已完成。下一步唯一主线是编译 + 跨设备实测（配对 → 建收件箱 → 全家可见 → 投送 → 宿主留一份/本机不留），见 `docs/verification-checklist.md`。**
