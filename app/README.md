# 见山 · Native UI MVP

基于 Go + [MyGo](https://github.com/egoist/mygo) 的原生图片查看器。

默认是单图 + 右侧信息栏，网格模式共用同一条窄工具栏。排序、格式、搜索和 ISO 筛选都通过「工具」按需展开。默认深色，可在工具中切换浅色或跟随系统。

## 运行与构建

在 `app/` 中执行：

```sh
mise exec go@1.27.3 -- go run .
# 也可传入文件夹或图片，打开图片时会加载同文件夹的其他照片
mise exec go@1.27.3 -- go run . /path/to/photos

CGO_ENABLED=0 mise exec go@1.27.3 -- go test ./...
mise exec go@1.27.3 -- go vet ./...
mise exec go@1.27.3 -- go tool mygo build
```

MyGo 固定在 `v0.7.2-0.20261027092812-b553a90aefc5`，不依赖调研目录的本地源码。首次构建会下载 Go modules。macOS 构建输出在 `build/darwin-arm64/`：`见山.app` 与 `jianshan-0.1.0-arm64.dmg`。目前生成的是本地临时签名包，未做 Developer ID 签名和公证。

## 已实现

- 原生中文更新窗口；安装包启动后自动检查，默认每天一次，也可手动检查。新版归档通过项目专用 Ed25519 签名验证，下载和安装由用户选择。
- 打开文件夹、拖入文件夹/图片、通过系统「打开方式」打开图片；读取当前层级，忽略隐藏文件和子目录。
- JPEG、PNG、WebP、TIFF、BMP 和 GIF 首帧预览。
- RAW 优先读取最大内嵌 JPEG：识别 CR2、CR3、NEF、NRW、ARW、RAF、ORF、RW2、DNG、PEF、SRW。macOS 上无内嵌预览时尝试系统 ImageIO（`sips`）解码。已用真实 Nikon D3S NEF、Canon EOS 40D CR2 验证预览与常用 EXIF。
- 单图左右切换；网格单击选中、双击或 Enter 打开；两个视图中选中项与右侧照片信息同步。
- 单图双击返回网格；底栏放大/缩小、100% 预览像素显示和适应窗口。鼠标滚轮、触控板捏合可缩放，拖动或双指滚动可平移；按住 Ctrl 拖出矩形，松开后放大选中区域；`+` / `-` 缩放，`0` 恢复适应窗口。切图恢复默认视角，缩放基于最长边 4096 px 的现有预览。
- 11 种排序：文件名自然排序、修改时间、文件大小、文件类型、拍摄时间、像素尺寸、ISO、光圈、快门、焦距、相机。支持升降序；缺失元数据始终排在最后。
- 文件名搜索、后缀快捷筛选、仅 RAW、ISO 范围，可组合使用；无结果时可以一键清除。
- JPEG / TIFF 系 RAW 的常见 EXIF：相机、镜头（标准 LensModel 字段）、ISO、快门、光圈、焦距、拍摄时间、尺寸和方向。缺失值显示「未提供」；照片自动按 EXIF 方向转正。
- 异步扫描与按需解码，最多 2 个解码任务并发；切换目录取消旧任务，旧结果不会覆盖新目录。缩略图与大图分别设缓存预算，网格只构建可见行。

快捷键：`←` / `→` 切图，`G` 切换网格与单图，`I` 显示/隐藏信息，`⌘O` / `Ctrl+O` 打开文件夹；`Esc` 先收起工具，再切换视图。

## 当前边界

- `go run` 开发模式不会自动更新。早期未包含更新器的本地构建需先手动安装一次公开版；系统包管理器安装或目录不可写时更新器可能禁用安装。发布流程见 [RELEASING.md](../RELEASING.md)。
- RAW 显示的是相机 JPEG 预览或系统转换结果。没有 LibRaw 显影流程，也不保证所有相机型号均可读取；Windows/Linux 目前仅走内嵌 JPEG 路径。未实际验收这些平台的桌面交互。
- EXIF 是有边界的标准字段解析器，尚未接入 ExifTool、厂商 MakerNotes、XMP、GPS 和完整色彩管理。RAW 的尺寸可能是传感器尺寸；预览尺寸可能更小。
- 单图解码预览最长边最多 4096 px，缩略图最多 420 px；放大时暂不重新解码原图，GIF 显示首帧。尚无幻灯片、胶片栏、文件夹树、评分/标签、目录实时监听或会话设置持久化。
- 文件读取限制为编码文件 512 MB / 解码图像 1.2 亿像素；内嵌预览在 RAW 前 64 MB 内查找。缓存预算只计应用持有的基础 RGBA 像素，不是进程总内存上限。

## 验证与文件

`app_test.go` 覆盖默认布局、网格/单图与键盘、选中项、后缀菜单、排序、筛选恢复与输入框快捷键冲突；`media_test.go` 覆盖两种 EXIF 字节序、8 种方向、损坏文件、RAW 内嵌预览选择、目录扫描和缓存淘汰。`CGO_ENABLED=0` 的测试和 `go vet` 已通过，Windows/Linux amd64 已通过交叉编译；macOS 原生窗口已验证单图、网格和真实 RAW 切换。真实 RAW 样本通过显式环境变量进行可选验证：

```sh
JIANSHAN_RAW_FIXTURE=/path/to/file.NEF mise exec go@1.27.3 -- go test -v -run TestRealRAWFixture
```

- `main.go` / `app.go`：应用生命周期、目录任务与状态。
- `view.go` / `icons.go`：MyGo 原生界面。
- `model.go`：筛选、排序和选中项。
- `scan.go` / `metadata.go`：目录扫描和 EXIF。
- `images.go` / `raw*.go`：图像缓存、解码与 RAW 预览。
- `screenshots/`：本机原生窗口的单图/网格截图。

验收使用了 [rawpy 的测试样本](https://github.com/letmaik/rawpy/tree/main/test)（Nikon `iss030e122639.NEF`、Canon `RAW_CANON_40D_SRAW_V103.CR2`），仅下载到被忽略的 `.research/fixtures/`，未打包进应用。截图中的设计样片来源见上级 `prototype/THIRD_PARTY.md`，应用不会自动载入这些样片。
