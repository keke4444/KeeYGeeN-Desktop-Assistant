# dtidy 技术设计（DESIGN）

> 版本：v0.1 草案 ｜ 配套文档：[PRD.md](PRD.md)、[TUI-SPEC.md](TUI-SPEC.md)、[CATEGORY-RULES.md](CATEGORY-RULES.md)

---

## 1. 技术选型

| 层面 | 选型 | 理由 |
|---|---|---|
| 语言 | Go 1.22+ | 单二进制、交叉编译简单、标准库文件能力足够、`os` 层跨平台抽象好 |
| TUI | `charmbracelet/bubbletea` + `lipgloss` + `bubbles` | Go 生态最成熟 TUI 方案，Elm 架构（Model/Update/View）便于测试与状态机化 |
| 配置 | `BurntSushi/toml` | TOML 可读性好，支持数组表（分类列表） |
| 哈希 | 标准库 `crypto/sha256` | 校验无需第三方依赖 |
| CLI 解析 | 标准库 `flag` + 手写子命令分发 | 子命令只有 5 个，引入 cobra 属于过度设计 |

**不使用**：cobra、viper、任何文件监听库、任何网络库。

## 2. 项目结构

```
桌面整理/
├── go.mod                      # module dtidy
├── main.go                     # GUI 入口：wails.Run + 嵌入 frontend/dist
├── app.go                      # 绑定给前端的 App：GetState/ListDir/MoveTo/Undo/CreateCategory…
├── app_test.go                 # 后端接口集成测试（临时桌面）
├── wails.json                  # Wails 工程配置
├── frontend/dist/              # 原生 HTML/CSS/JS（无构建步骤，直接嵌入）
│   ├── index.html
│   ├── app.css
│   └── app.js
├── cmd/dtidy-tui/main.go       # 终端版入口（子命令：tui / scan / undo / config / version）
├── internal/
│   ├── platform/               # 平台适配层（含 //go:build 的目录）
│   │   ├── desktop_windows.go  # 本轮唯一实现：注册表 User Shell Folders → Desktop
│   │   ├── desktop_other.go    # 非 Windows 留位
│   │   └── systemfiles.go      # 系统文件黑名单（desktop.ini/Thumbs.db/.DS_Store）
│   ├── config/                 # 配置：默认值、加载、校验、落盘、路径解析
│   ├── category/               # 分类注册表：home 子目录发现 + 配置补充 + 名称合法性校验
│   ├── fsops/                  # 列表/扫描、复制、校验、冲突命名、临时文件、安全删除
│   ├── journal/                # JSONL 日志写入 + 批次查询 + undo 执行
│   └── tui/                    # 可选终端界面（Bubble Tea）
├── docs/                       # 本套文档
└── build/                      # wails build 产物
```

**依赖方向**（严格单向，禁止反向 import）：

```
main(GUI) → app.go → {fsops, category, config, journal, platform}
cmd/dtidy-tui → tui → {fsops, category, config, journal}
journal → fsops
```

`app.go` 与 `tui` 是两个并列的编排层，共用 `fsops` / `category` / `journal` / `config`。所有磁盘写入只允许出现在 `fsops` 与 `journal` 两个包内（`app.go` 仅创建目录）。

## 3. 核心数据模型

```go
// fsops
type Entry struct {
    Name     string      // 文件名（不含路径）
    Path     string      // 绝对路径
    Size     int64
    ModTime  time.Time
    Ext      string      // 小写，含点，如 ".lnk"；无扩展名为 ""
    IsDir    bool        // 目录不进入可移动列表，只在分类发现阶段被读取名字
    Marked   bool        // TUI 层的多选标记
}

// fsops
type MoveResult struct {
    Src      string
    Dst      string
    Size     int64
    SHA256   string
    Verify   string      // "sha256" | "size"
    Ok       bool
    Err      error
    Skipped  bool
    Reason   string      // skipped/失败原因文案
}

// journal
type Record struct {
    TS      time.Time `json:"ts"`
    Batch   string    `json:"batch"`    // 20261020-153000-3f9a
    Action  string    `json:"action"`   // move | undo
    Src     string    `json:"src"`
    Dst     string    `json:"dst"`
    Size    int64     `json:"size"`
    SHA256  string    `json:"sha256"`
    Verify  string    `json:"verify"`
    OK      bool      `json:"ok"`
    Err     string    `json:"err,omitempty"`
}
```

## 4. 平台路径解析

| 平台 | 主方案 | 回退 |
|---|---|---|
| Windows | 读 `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders` 的 `Desktop` 值，展开 `%USERPROFILE%` 等环境变量 | `%USERPROFILE%\Desktop`；再回退 `%USERPROFILE%\OneDrive\Desktop` |
| Linux | 解析 `~/.config/user-dirs.dirs` 取 `XDG_DESKTOP_DIR`（去掉引号并展开 `$HOME`） | `~/Desktop` |
| 配置覆盖 | `config.toml` 的 `desktop` 字段非空时**最高优先级** | — |

注册表读取使用 `golang.org/x/sys/windows/registry`（仅 Windows 构建引入，Linux 构建不编译该文件）。

配置目录：

| 平台 | 路径 |
|---|---|
| Windows | `%APPDATA%\dtidy\config.toml` |
| Linux | `$XDG_CONFIG_HOME/dtidy/config.toml` → `~/.config/dtidy/config.toml` |

统一通过 `os.UserConfigDir()` 获取基目录。

## 5. 配置模型

```toml
# dtidy 配置（首次运行自动生成，分类列表默认空白）
desktop     = ""          # 留空 = 自动探测桌面（Windows 走注册表）
root_dir    = "home"      # 整理根：<桌面>\home，分类放在其下；首次启动自动创建
                          # 置空则分类直接挂在桌面下（不推荐）
discover_categories = true # 把 home 下已有的一级文件夹识别为可选分类
extra_categories = []     # 额外补充分类名；home 下没有该文件夹时，移动时按需创建
                          # 例：["取证", "repwn"]（必须是合法的 Windows 文件夹名）
exclude_categories = []   # home 下存在、但不作为分类候选的文件夹（如 ["临时"]）
conflict    = "index"     # index | timestamp | skip
verify      = "sha256"    # sha256 | size
concurrency = 4           # 复制并发数
show_hidden = false       # 是否列出隐藏文件
ignore      = ["desktop.ini", "Thumbs.db", ".DS_Store"]
confirm_above_mb = 0      # >0 时，超过该体积的文件移动前二次确认
```

加载优先级（高 → 低）：命令行参数 > 环境变量（`DTIDY_DESKTOP`、`DTIDY_ROOT`）> `config.toml` > 内置默认值。

校验规则（加载时即报错，不静默修正）：
- `conflict`/`verify` 取值必须在枚举内。
- `extra_categories` 中每个名字必须通过 Windows 文件夹名校验：非空、不含 `< > : " / \ | ? *`、不以空格或点结尾、不是保留名（`CON`/`PRN`/`AUX`/`NUL`/`COM1`-`COM9`/`LPT1`-`LPT9`）、长度 ≤ 255。桌面上发现的目录名天然合法，但仍走同一校验函数。
- **不做改名映射**：非法名字直接报错，工具不会替用户"清洗"名字（否则文件会跑到用户没预期的目录里）。
- 分类列表允许为空（桌面还没有分类文件夹且未配置 `extra_categories`）：TUI 分类行给出提示，Enter 时不执行移动。

## 6. 分类注册表（`internal/category`）

v0.2 **不做扩展名判定**，分类列表来自「home 子目录 + 配置」两部分：

```
Build(rootDir string, discover bool, extra, exclude []string) (*Registry, error)
  1. if discover:
       读 rootDir 下所有一级目录（不递归）→ 目录名加入候选（按名称排序，保证顺序稳定）
  2. 追加 extra
  3. 去除 exclude（Windows 语义下不区分大小写匹配）
  4. 去重（Windows 下按不区分大小写比较，保留首次出现的写法）
  5. 每个名字过 ValidateName()：Windows 文件夹名规则
        - 非空、≤ 255 字符、不含 < > : " / \ | ? *
        - 不以空格或 . 结尾
        - 不是保留名 CON/PRN/AUX/NUL/COM1-9/LPT1-9（含带扩展名形式，如 CON.txt）
     失败 → 返回错误，由 main 打印并退出（不进入 TUI）
  6. 得到有序分类列表 []Category{Name, Path: <rootDir>/<Name>/}
```

- **不做改名映射**：`re/pwn` 这类非法名直接报错，用户需自己改成 `repwn`。
- 分类列表可以为空（home 刚创建且未配置）：GUI 左栏显示「还没有分类」提示，拖拽无目标，需先新建。
- 桌面上发现的目录名天然合法，但仍走同一 `ValidateName()`，避免符号链接或其他特殊情形漏网。
- 分类名大小写按 Windows 语义不敏感；去重后保留用户原始写法。

## 7. 移动引擎（`internal/fsops`）

这是全局唯一允许「删除文件」的地方，规则极窄：**只允许删除 (a) 自己创建的临时文件，(b) 校验通过且已完成原子重命名的源文件**。

### 7.1 主流程

```
Move(src string, targetDir string, cfg) (MoveResult, error)

// targetDir 的解析（在调用 Move 之前完成）：
//   targetDir = filepath.Join(desktop, cfg.RootDir, categoryName)
//     cfg.RootDir == ""  →  <桌面>/<分类名>/        （默认；空层被 Join 自动忽略）
//     cfg.RootDir != ""  →  <桌面>/<root_dir>/<分类名>/
//   若目标分类文件夹已存在（桌面上本来就是一排分类文件夹）→ 直接复用，不重建、不报错

 1. Preflight
      - 桌面可读、目标目录可写（不存在则 MkdirAll）
      - 源存在且为普通文件（非目录、非符号链接 → 默认跳过）
      - 跨卷空间检查：目标卷剩余空间 ≥ 源大小 × 1.05
      - Windows 路径长度检查：final 路径 > 240 字符时告警/跳过
 2. ResolveConflict(targetDir, base, cfg) → finalPath
      - index     : name.ext → name (2).ext → name (3).ext …
      - timestamp : name.ext → name_20261020-153000.ext
      - skip      : 命中即返回 Skipped 结果，不动任何文件
      - 判定使用 os.Stat 循环探测，直到找到不存在的名字
 3. tmp := filepath.Join(targetDir, ".dtidy-tmp-" + randHex(8))
 4. copyFile(src, tmp)      // io.CopyBuffer，64 KiB 缓冲，保留 mtime 与权限位
 5. verify(src, tmp, cfg.Verify)
      - size : 比对字节数
      - sha256 : 比对两侧摘要
      失败 → os.Remove(tmp) → 返回失败，源文件完好
 6. os.Rename(tmp, finalPath)   // 同目录内，原子
 7. os.Remove(src)              // 仅删源文件本身
 8. journal.Append(Record{...OK:true})
```

### 7.2 失败矩阵

| 阶段失败 | 处理 |
|---|---|
| Preflight | 不产生任何副作用，源文件不动，计入报告 |
| 复制中断 | 删除临时文件，源文件不动 |
| 校验不一致 | 删除临时文件，源文件不动，报告明确提示「校验失败」 |
| Rename 失败 | 删除临时文件，源文件不动 |
| 删除源失败 | 目标副本已就位，源仍存在 → 记录 `ok=true, err="源未删除"`，报告提示存在重复副本，供人工处理（**绝不重试删除**） |
| 程序崩溃 | 残留 `.dtidy-tmp-*` 文件：下次启动扫描桌面与分类目录时提示清理（清理属于删除操作，需用户确认） |

### 7.3 并发

- 多文件移动采用有界 worker pool（`concurrency`，默认 4），仅并发「复制+校验」，重命名与删源在各自 worker 内串行完成。
- 结果按条目顺序回填，避免报告乱序。
- 扫描阶段的 size/mtime 使用 `os.ReadDir` + `Entry.Info()`，不递归。

## 8. 日志与撤销（`internal/journal`）

- 文件：`<configDir>/journal.jsonl`，追加写，每行一个 `Record`，写入后 `Sync()`。
- 批次 ID：`<yyyymmdd-HHMMSS>-<4hex>`，TUI 每次启动会话生成一个，会话内所有移动同属该批次。
- `undo` 算法：

```
undo(batchID, cfg):
  records := 读 journal.jsonl → 过滤 Action=="move" && OK && Batch==batchID
  逆序遍历 records:
    if !exists(dst) → 跳过："目标不存在，可能已手动移动"
    if exists(src)  → 跳过："源位置已有同名文件，避免覆盖"
    copyFile(dst, tmp=sameDir(src)) → verify → os.Rename(tmp, src) → os.Remove(dst)
    写一条 Action=="undo" 的 Record
  输出汇总（成功/跳过/失败）
```

- 日志永不自动轮转删除；提供 `dtidy undo --list` 列出最近批次（可选实现，列入 v0.1 的 nice-to-have）。

## 9. 错误处理与边界

| 边界 | 策略 |
|---|---|
| 权限不足（目标无写权限） | Preflight 拦截，报告标注原因，不尝试提权 |
| 长路径（Windows > 260） | 最终路径 > 240 字符时跳过并提示；不使用 `\\?\` 前缀改写路径（避免用户看到不可读路径） |
| 大小写不敏感文件系统 | 冲突探测使用 `os.Stat` 的实际语义，即 Windows 上 `A.PNG` 与 `a.png` 视为同一冲突 |
| 符号链接 | 默认跳过（不跟随、不复制链接本身），计入报告 |
| `.lnk` / `.url` 快捷方式 | 视为普通文件参与移动：移动快捷方式文件本身不改变其绝对路径指向，归类后依旧可用。v0.1 不解析 `.lnk` 目标，因此「快捷方式的目标文件本身在桌面上」时存在断链风险（PRD 待定项 7） |
| 网络盘 / 可移动盘 | 不特殊处理，依赖 Preflight 空间检查；`verify` 建议用户改为 `sha256`（默认已是） |
| 只读文件 | 复制后设置目标可写位，源文件删除按系统语义（Windows 只读文件删除会失败 → 报告提示） |
| 磁盘写满 | io.Copy 返回错误 → 走「复制中断」分支 |
| 终端 resize | View 每帧按 `tea.WindowSizeMsg` 重新计算布局（见 TUI-SPEC §5） |
| 空桌面 | 列表显示 Ubuntu 风格的空状态文案，不禁用快捷键 |
| 分类列表为空 | 列表照常显示，分类行提示「暂无分类」；按 Enter 不执行移动并提示先去桌面建一个分类文件夹（或在 `extra_categories` 里补） |
| 非法分类名 | 配置加载阶段报错退出，明确指出是哪个名字违规（不静默改名、不跳过该项） |

## 10. 测试策略

| 层级 | 范围 | 手段 |
|---|---|---|
| 单元 | `category` 名称校验（非法字符/保留名/去重）、`config` 校验、冲突命名三种策略 | 表驱动测试，`testdata/` 提供名字样本 |
| 单元 | `fsops` 的 Preflight/verify 分支 | 临时目录（`t.TempDir()`）构造真实文件 |
| 故障注入 | 校验失败、复制中断、删源失败 | `fsops` 暴露 `FaultInjector` 接口（仅测试注入，生产为 nil） |
| 集成 | 扫描 → 移动 → 撤销 全链路 | 构造假桌面，断言数量关系与日志内容（对应 PRD AC-3/4/5/6/10） |
| TUI | Update 函数状态迁移、按键映射 | 直接对 `Model.Update` 喂 `tea.KeyMsg`，断言状态与光标，不做终端渲染快照 |
| 平台 | 桌面路径解析 | Windows 侧读注册表真实值（AC-1） |

**不写**：真实终端视觉快照测试（维护成本高于收益）。

## 11. 构建与发布

- `go build ./...` 产出单二进制 `dtidy.exe`（本轮交付 Windows）。
- 交叉编译能力保留：`GOOS=windows GOARCH=amd64|arm64`。
- 发布（可选，v0.1 不强制）：打一个 zip 即可。
- 无 CGO 依赖，纯静态链接。

## 12. 里程碑拆分（供后续实施）

| 里程碑 | 内容 | 交付物 |
|---|---|---|
| M1 | `platform` + `config` + `category` | 扫描/分类可运行（`dtidy-tui scan`） |
| M2 | `fsops` + `journal` | 移动引擎与撤销链路，单元测试覆盖 |
| M3 | Wails 界面：左栏分类、表格、拖拽归类、右键菜单、撤销 | GUI 闭环（已完成） |
| M4 | 主题打磨、窄窗口适配、报告明细、文档同步 | 达到 PRD 验收标准 |

## 13. GUI 实现（Wails）

- 入口：[main.go](../main.go) `wails.Run` + `//go:embed all:frontend/dist`。
- 后端绑定：[app.go](../app.go) 暴露 `GetState`、`ListDir`、`Search`、`MoveTo`、`Undo`、`CreateCategory`、`OpenFolder`、`RevealFile`、`OpenFile`。
- 前端：原生 HTML/CSS/JS，无 npm 构建；`wails.json` 的 `frontend:install/build` 留空，构建时用 `wails build -webview2 error`（跳过网络下载）。
- 布局：左栏 `搜索框 / 打开文件夹 / 已打开目录 / 桌面 / home / 分类 / 选中状态 + 移动按钮`；右栏路径栏 + 文件区（底部状态栏已并入侧栏）；侧栏与文件区之间是 `.splitter`，拖动范围 180–520px，写入 `sidebar_width`。
- 搜索范围：`全局` 扫描桌面（含 home）、已打开文件夹与分类指定目录；`当前` 只扫描当前所在目录（分类同样适用），默认使用「当前」，切换后保存在 localStorage，搜索状态面包屑显示「当前：文件夹名」。
- 文件右键：调用 `internal/shellmenu` 的原生 Shell 菜单（IContextMenu/IContextMenu2/IContextMenu3 + TrackPopupMenuEx），支持子菜单与第三方扩展；失败时回退内置菜单。为避免卡死 UI，菜单全部在独立的常驻 STA 线程执行（宿主 1×1 离屏窗口作 owner，弹出前临时置前；拿不到前台就取消并回退内置菜单）；每个请求有 `recover` 兜底，菜单 45 秒自动 `WM_CANCELMODE`，前端 60 秒超时复位。文件行走 `ShowShellMenu(paths)`（IShellFolder::GetUIObjectOf），文件区空白处走 `ShowShellMenuBackground(dir)`（BindToObject 得到目录 IShellFolder → CreateViewObject 取 IShellView → GetItemObject(SVGIO_BACKGROUND) 取背景菜单）。
- 单实例：启动时用 `CreateMutexW`（名称前缀为窗口标题）判断；已有可见实例则还原并置前后退出，若旧进程无可见窗口则允许继续启动，避免残留进程导致打不开。菜单/COM 的所有调用都在独立 STA 线程完成，UI 线程只做界面渲染。
- 删除分类：右键「删除分类…」先弹 `MessageDialog` 确认（默认按钮为取消），确认后 `os.RemoveAll` 删除分类文件夹并清理 `category_targets`、别名、图标等配置；已打开文件夹的 × 仍然只关闭不删除。
- 侧栏自定义：`custom_folders` 持久化「打开文件夹」加入的目录，`item_aliases` / `item_icons` 保存显示名称与图标，右键菜单支持 自定义图标 / 重命名 / 指定文件夹 / 删除分类（已打开文件夹为关闭）；`hidden_categories` 记录被移除的分类，`category_targets` 记录分类指定的目标目录。
- 搜索：左栏顶部输入框（防抖 260ms，Ctrl+F 聚焦），后端 `Search(query)` 递归扫描桌面（含 home），多个关键词按空格拆分且全部命中，最多 200 条；输入后按 Tab 唤出候选下拉，可视条数由 `search_candidates`（3–30）决定，可继续 ↑↓ 滚动查看全部结果；− + 切换、Enter 打开、Esc 关闭，条目高亮使用主题色；完整结果显示在文件区。
- 候选规则：[internal/candrule](../internal/candrule/rules.go) 解析 `cand_rule_file`（txt / json / csv / tsv / ini / conf / toml）。规则分「排除」与「优先」两类：txt 每行 `+词` 优先、`-词` 排除、`#` 注释；json/toml 支持 `exclude` / `include` / `priority` / `rules` 数组；csv/tsv 每行 `词,动作`；ini 支持 `exclude=` / `priority=`。每次搜索实时读取，排除命中的名称、按优先词顺序稳定排序。
- 视图：`图标视图`（自适应网格，图标在上、名称在下）与 `详情列表`（名称/类型/大小/修改时间）可切换，选择写入 `localStorage`。
- 图标：`internal/shellicon` 先用序号 727 取 `SHGetImageList`（`SHIL_EXTRALARGE` 48×48）+ `IImageList.GetIcon`，失败回退 `SHGetFileInfoW` 32×32；`GetIconInfo + GetDIBits` 转 PNG data URL；按扩展名缓存（`.exe`/`.lnk` 等按路径），失败回退内置 SVG。
- 图片缩略图：png/jpg/gif/bmp/webp 用 Go 解码 + `x/image/draw` CatmullRom 等比缩放到长边 160px，按 `路径|mtime|大小` 缓存；超过 48 MB 不解码；失败回退类型图标。
- 缩放：CSS 变量 `--icon-scale`（0.6–2.0），Ctrl+滚轮与设置滑杆共同驱动，写入 `icon_scale`。
- 背景：`bg_image` 路径 → 读文件转 data URL 交给窗口级 `.bg-layer`，`bg_opacity` 控制透明度；`bg_fit` 控制适配方式（contain 完整显示 / cover 铺满 / stretch 拉伸 / tile 平铺）；`bg_scope` 控制作用域（window 全局 / main 整个主区含路径栏，按 `.main` 实时矩形跟随侧栏位置 / custom 拉选区域，`bg_rect` 存 [x,y,w,h] 窗口比例）；gif/webp 由浏览器直接播放动画。预设背景由 `config.UseDefaultBg`（ldflags 字符串开关，默认空 = 关闭）控制：只有第二版构建传 `-X dtidy/internal/config.UseDefaultBg=true` 时，才会在 `bg_image` 为空时自动采用 exe 同目录的 `bg.png`；正式版始终不带预设图片，即使同目录存在 `bg.png` 也不加载。
- 背景拉选：设置里选「拉选」后点「在界面上拖拽框选…」，界面出现十字遮罩，按住鼠标拖出矩形即保存并立即生效，Esc 取消。
- 路径设置：`ChooseDesktop` 切换桌面后自动使用 `<新桌面>\home`；`ChooseHome` 选择非 home 文件夹时用 `MessageDialog` 询问并创建 `home`；`root_dir` 支持绝对路径。
- 开机启动：`autostart` 为 true 时通过 `reg add HKCU\...\Run /v dtidy` 写入当前可执行文件路径，启动时自动校正；关闭时删除该项。
- 关闭行为：`close_to_taskbar` 为 true 时，Wails `OnBeforeClose` 拦截关闭并调用 `runtime.WindowHide`（返回 true 阻止退出），窗口隐藏后由托盘图标或热键唤出；为 false 时正常关闭。设置面板「启动与关闭」区域同时提供开机自启与关闭到系统托盘两个勾选项。
- 系统托盘：[tray_windows.go](../tray_windows.go) 直接调用 `Shell_NotifyIconW`（NIM_ADD/NIM_MODIFY/NIM_DELETE）在任务栏通知区域显示小图标。图标使用 `//go:embed build/windows/tray.ico` 内嵌的透明背景多尺寸 ICO（16/20/24/32/40/48/64，由 logo 裁出眼睛主体并去除白底），运行时写入临时文件后用 `LoadImageW(LR_LOADFROMFILE)` 按 `SM_CXSMICON` 尺寸加载；失败回退 exe 资源图标与 `IDI_APPLICATION`。托盘窗口与消息循环运行在独立常驻线程（`LockOSThread`）：左键单击 `WM_LBUTTONUP` 切换主窗口显隐，右键 `WM_RBUTTONUP` 弹出「显示主窗口 / 退出程序」菜单（`TPM_RETURNCMD`，主窗口可见时作为菜单 owner 保证外部点击可关闭）；监听 `TaskbarCreated` 消息，资源管理器重启后自动重新添加图标；`OnShutdown` 时删除图标。
- 配色：`theme_accent`（整体颜色）驱动 `--orange` 与高亮派生色，并作为整个界面底色的基色（压暗后用于 `--bg` 与面板派生色）；`theme_page_custom` 为 true 时 `theme_page` 单独覆盖页面底色；`theme_back` + `follow_theme_back` 控制「返回上一级」按钮颜色，跟随开启时忽略自定义色。设置弹窗背景使用 `--panel-elev`、输入框使用 `--input-bg`，与界面配色一致并随页面颜色预览实时变化。
- 分类栏位置：`categories_position` 映射为 `.app.pos-left / pos-right / pos-top`，顶部模式把侧栏变成横向条。
- 热键：`internal/hotkey` 独立线程 `RegisterHotKey` + `GetMessageW` 循环；`WM_APP+1` 消息用于运行时改绑，`WM_APP+2` 退出并注销。回调走 [windowtoggle_windows.go](../windowtoggle_windows.go)：窗口可见且在前台时 `ShowWindow(SW_HIDE)`，隐藏/最小化时先 `ShowWindow(SW_RESTORE)` 再置前，焦点在其它程序时只置前不隐藏。
- 命名与安装：窗口标题、侧栏品牌、可执行文件统一为 `KeeYGeeN桌面助手`（`wails.json` 的 `outputfilename`）；安装包由 [installer/KeeYGeeN桌面助手.iss](../installer/KeeYGeeN桌面助手.iss) 生成，按用户目录安装，提供桌面快捷方式。应用图标为 `build/windows/icon.ico`（由 KeeYGeeN logo 生成的 16–256px 多尺寸 ICO），侧栏品牌图使用 `frontend/dist/logo.png`。
- 交互：多选（Ctrl/Shift）、拖拽到左栏节点移动、右键菜单、双击打开/进入、F5 刷新、Esc 清空选择。
- 最近：[internal/recent](../internal/recent/store.go) 把记录持久化到配置目录 `recent.json`（`tasks` 任务分组 + `items` 记录：路径、归属任务、次数、最近使用时间），默认最近最多 200 条；[app_recent.go](../app_recent.go) 暴露 `GetRecentGroups` / `ListRecent` / `MoveRecent` / `RemoveRecent` / `CreateRecentTask` / `RenameRecentTask` / `DeleteRecentTask`。程序内 `OpenFile` 直接调用 `recordOpen`；系统里打开的文件由 `recent.Store.StartWatch` 每秒轮询 `%APPDATA%\Microsoft\Windows\Recent` 的新 `.lnk`，用 `IShellLink`+`IPersistFile` 解析目标后合并（去重按路径大小写不敏感；系统目录、临时目录、`.lnk` 与本程序自身会被过滤）。删除记录只改 JSON，不动磁盘文件；任务分组同样只是记录分组，不创建文件夹。全局实时「文件被打开」事件在 Windows 上需要内核驱动或管理员权限的 ETW 会话，普通权限运行下不可用，因此采用轮询方案。
- 最近界面：路径栏在「最近」视图显示「最近」+「＋ 新建任务」；任务标签单独渲染到 `#recentTabBar` 一行（胶囊样式，`border-radius:999px`），作为记录拖放目标。标签颜色由 `theme_recent` / `follow_recent` 驱动：`applyTheme` 计算后写入 `--recent-tab-color` 与 `--recent-tab-rgb`，设置面板提供与「返回上一级按钮」同款的跟随开关 + 取色器。
- 字体：配置 `font_family` / `font_size` / `font_color` 为全局，`[font_parts.<sidebar|pathbar|file|settings>]` 为分区覆盖（空值继承全局）。`applyFont` 把全局值写入 `--app-font` / `--fs-scale` / `--text`、`--muted`，分区值写入对应容器的同名变量；CSS 中所有 `font-size` 统一改为 `calc(<原字号>px * var(--fs-scale, 1))`，分区通过自定义属性的继承实现独立缩放。设置面板「字体 / 分区字体」两行提供实时预览，保存后持久化。
- 安全边界：`allowedDest` / `allowedSource` 限定桌面、home、已打开文件夹与分类指定目录内部；文件夹一律拒绝移动。

