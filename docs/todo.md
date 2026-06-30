# ShareBox TODO

> 最后更新：2026-06-30 | **架构已转向：去掉 Hub，改用 syncthing 原生模型（见 `docs/plans/2026-06-30-no-hub-redesign.md`）**

---

## 🎯 当前主线（优先）

### T0. 跨端文件投送打通

把"两台设备之间真正能投送文件"跑通。这是产品的最基础能力，其它一切后置。

- [ ] 两台设备扫码配对（syncthing 双向添加）
- [ ] 设备 A 投送文件 → 设备 B 收到
- [ ] 设备 B 浏览设备 A 的共享目录（看得到文件列表）
- [ ] 验证文件确实落到目标设备，且不在源设备多留副本（清理闭环已实现，待实测，见下）

依赖：先完成去 Hub 改造（前端扫码配对 + 家庭清单文件夹），见新设计文档。

**"源设备不多留副本"闭环（代码已完成，跨设备未实测）**：
- 发送端 RemoteAccess 文件夹后台 30s 周期清理：仅当**所有共享对端都已收到当前版本**后，才删本机暂存副本（`lib/model/folder_remoteaccess.go`，`CleanupStaging` + `cleanupLoop`）。
- 宿主端自动接受文件夹时强制 `ignoreDelete=true`（`lib/model/model.go` `handleAutoAccepts`），忽略发送端删除传播，保留文件。
- 注意：此 `ignoreDelete` 作用于所有非加密的自动接受文件夹；家庭组自动接受正是投送机制，语义一致。

---

## 📋 待办（已确认，记录备忘）

### T1. ~~薄连接器：Hub ↔ syncthing 自动配对~~ ❌ 废弃（随 Hub 一起删）

原本写了 Hub 信令 + 前端 `pairWithGroup()`/`openSharedFolder()` 来打通配对。**架构转向后整块废弃**——配对改为 syncthing 原生（扫码 + 双向添加 + introducer），不再需要薄连接器。详见 T4。

### T2. 家庭组安全 —— ✅ 已定方向：无服务器，安全交给 syncthing

**结论**：删掉 Hub 后，原来那一堆安全问题（零鉴权、弱邀请码、无 HTTPS、元数据裸奔）**全部消失**，因为根本没有自建公网服务可攻击。

新安全模型（详见 `docs/plans/2026-06-30-no-hub-redesign.md` 第七章）：
- 准入 = syncthing 双向添加 + 确认（基于公钥身份，无撞库问题）。
- 传输 = syncthing TLS / E2E；公共 discovery 只存地址映射、relay 只转密文，均零知识。
- 残余面仅"家庭清单文件夹"：全家可见、只含元数据（设备名/ID/收件箱 label）、只在已配对信任设备间同步。
- 洁癖增强（可选）：自建 `stdiscosrv` + `strelaysrv`。

### T4. 去 Hub 改造 —— ✅ 代码完成（未编译/未跨设备实测）

1. [x] 前端：本机 Device ID 展示 + 二维码（`/qr/` 端点，fetch+blob）+ 添加设备（粘贴 ID，调 `/rest/config/devices`）。
2. [x] 后端：家庭清单端点 `GET/POST /rest/sharebox/manifest`（读聚合 / 写本机 `<deviceID>.json`，用 folder Filesystem + 扫描）。
3. [x] 前端：家庭清单读/写 + 全家收件箱聚合展示（`loadFamily`/`writeMyManifest`/`renderFamily`），15s 定时刷新。
4. [x] 前端：新建收件箱（receiveonly + `ignoreDelete`）自动共享给家庭设备 + 写清单；添加新设备时 `shareEverythingWithPeers` 补共享。
5. [x] introducer：添加设备弹窗含"设为介绍人"勾选 + 文案引导。
6. [x] 删除 Hub：`cmd/sharehub/`、`internal/hub/`、前端组面板、`.gitignore` 条目（go.mod 的 sqlite 保留——`internal/db/sqlite` 也在用）。
7. [ ] **跨设备实测**：扫码/粘贴配对 → 双方互加确认 → 建收件箱 → 全家可见 → 投送 → 宿主留一份/本机不留。

**关键设计决定**：
- **`autoAcceptFolders=false`**：若为 true，对端会全量下载我的收件箱（违背"不下载/不留副本"）。改为：清单文件夹靠双方各自创建同 ID（`sharebox-manifest`）来同步；投送通道点击时显式建 RemoteAccess。
- **收件箱保留靠显式 `ignoreDelete=true`**（建收件箱时设），不再依赖 auto-accept。`model.go` 里 auto-accept 设 `ignoreDelete` 的改动现在不在主流程触发，保留作防御，无害。
- **清单/暂存用相对路径**（`sharebox-manifest`、`sharebox-send/<id>`）：后端读写与 syncthing 扫描都用同一 folder Filesystem，解析一致；跨设备实测时留意。

### T3. 文件下载 —— 暂不做（优先级低）

**决策**：本地已有文件下载没意义（直接复制粘贴即可）。真实场景"投出去想拿回来"可以靠"自己建个文件夹让对面投送回来"解决，问题不大。

**如果将来要做对端文件下载**：需要钩进 syncthing 块拉取机制，临时拉单个文件→存临时文件→吐给浏览器→清理。工作量不小，后置。

---

## ✅ 已完成（见 docs/status.md 详表）

- RemoteAccess 文件夹类型（后端）—— 保留
- ShareBox 独立前端页面（浏览/上传/目录选择器/新建文件夹）—— 保留
- 暂存清理闭环 + 宿主 `ignoreDelete`（源端不留副本/宿主留一份）—— 保留
- ~~Hub 服务（建组/加组/设备/目录注册）~~ —— 待删（架构转向）
- ~~前端家庭组面板（创建/加入组、显示成员、注册目录）~~ —— 待改（换成扫码 + 清单文件夹）
