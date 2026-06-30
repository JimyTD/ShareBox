# ShareBox 功能验证清单

> 基于 Syncthing fork，新增 RemoteAccess 文件夹类型（远程浏览+上传，不同步本地）。

---

## ✅ 自动化验证（已通过）

| # | 验证项 | 结论 | 验证方式 |
|---|---|---|---|
| 1 | 构建通过（三平台） | ✅ | GitHub Actions build.yml |
| 2 | Release 自动出包 | ✅ | v0.1.1 三包产出 |
| 3 | 二进制可运行 | ✅ | `syncthing --version` → `v0.1.1 go1.25.11` |
| 4 | Upload API 路由注册 | ✅ | `POST /rest/folder/upload` 已注册 |
| 5 | RemoteAccess 类型常量 | ✅ | `FOLDER_TYPE_REMOTE_ACCESS=4` 已加入 protocol/config |
| 6 | `folder_remoteaccess.go` 编译 | ✅ | 无编译错误 |
| 7 | GUI RemoteAccess 入口 | ✅ | 文件夹类型选择器有 RemoteAccess 选项 |

---

## 🔴 手动验证（需运行环境，你来）

### 基础启动

| # | 验证项 | 操作 | 预期结果 | 状态 |
|---|---|---|---|---|
| 8 | 应用启动 | 双击 `syncthing.exe` 或 `syncthing serve` | Web UI 可访问 `http://localhost:8384` | ⬜ |
| 9 | 创建 RemoteAccess 文件夹 | Web UI → 添加文件夹 → 类型选 "RemoteAccess" | 创建成功，文件夹列表可见 | ⬜ |

### RemoteAccess 核心功能

| # | 验证项 | 操作 | 预期结果 | 状态 |
|---|---|---|---|---|
| 10 | 对端设备连接 | 两台设备配对，共享 RemoteAccess 文件夹 | 对端出现在文件夹的设备列表 | ⬜ |
| 11 | 远端文件列表 | 打开 RemoteAccess 文件夹 | 显示对端文件列表，**不下载文件** | ⬜ |
| 12 | 上传文件 | `POST /rest/folder/upload?folder=<id>&name=<name>` | 返回 200，文件暂存到本地 | ⬜ |
| 13 | 上传后对端可见 | 等待传输完成，检查对端 | 对端文件夹出现新文件 | ⬜ |
| 14 | 暂存清理 | 上传并等所有对端收到后（后台 30s 周期） | 发送端本地暂存文件被自动删除，且宿主端文件**保留**（宿主自动接受文件夹强制 `ignoreDelete=true`） | ⬜ |
| 15 | 不下载文件 | 检查本地磁盘 | RemoteAccess 文件夹路径下无对端文件 | ⬜ |

### Upload API 示例

```powershell
# 上传本地文件到 RemoteAccess 文件夹
curl -X POST "http://localhost:8384/rest/folder/upload?folder=<folder-id>&name=target-filename.txt" `
  -H "X-API-Key: <your-api-key>" `
  -F "file=@C:\path\to\local-file.txt"
```

---

## 🟡 回归验证（建议）

| # | 验证项 | 操作 | 预期结果 | 状态 |
|---|---|---|---|---|
| 16 | 普通文件夹同步 | 创建 SendReceive 文件夹，双向同步 | 正常工作，不受 RemoteAccess 改动影响 | ⬜ |
| 17 | Linux/macOS 平台 | 下载对应平台包，重复 8-16 | 功能一致 | ⬜ |
| 18 | 异常处理 | 上传不存在的文件、重复上传同名、断网恢复等 | 有合理报错，不崩溃 | ⬜ |

---

## 验证日志

| 日期 | 人员 | 完成项 | 发现的问题 |
|---|---|---|---|
| 2026-06-18 | JimyTD | 1-7 (自动化) | 首次 Release 403 权限已修复 |
| 2026-06-30 | - | 代码审查全模块 | 暂存清理已实现但未接线（已修复）；下载为故意不做；安全为已知缺口 |
| 2026-06-30 | - | 暂存清理闭环（代码） | 清理接线 + 修正"全对端确认才删" + 宿主 `ignoreDelete` 保留；待跨设备实测 |
