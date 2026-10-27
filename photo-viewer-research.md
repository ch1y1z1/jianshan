---
lang: zh
template: sheet
theme: paper
title: 见山 · 图片查看器设计调研
subtitle: 文件夹浏览、连续看图与拍摄参数 · 2026-10-27
cols: 3
---
建议正式应用采用 Go + MyGo 原生 UI。本轮先用可点击原型讨论布局与交互。默认工作面改为单张查看，整理工具按需展开。

## A 先体验，再讨论
[打开交互原型](./prototype/)

[详细调研与设计说明](./DESIGN.md)

- 默认中央显示单张图片，右侧显示照片信息。
- 文件夹、网格、排序、筛选和胶片栏默认收起。
- 需要整理时从工具菜单进入网格模式。
- 单张模式用方向键连续切图。

## B 两个仓库的结论
| 查到的内容 | 对本项目的影响 |
|---|---|
| MyGo 有原生 UI 和 WebView 两条路线 | 正式应用选择原生 UI |
| 云音乐桌面端使用 Go 的 `ui` 包 | 它的 website 目录是宣传站 |
| 案例采用固定侧栏与状态驱动视图 | 网格与单张共用选择、筛选和排序 |
| 案例集中管理浅深色 palette | 原型提供两种外观，照片保持视觉中心 |

[MyGo](https://github.com/egoist/mygo) · [云音乐源码](https://github.com/egoist/mygo-cloudmusic)

## C 界面结构
```flow LR
文件夹 -> 缩略图网格: 打开
缩略图网格 -> 单张查看: 双击
单张查看 -> 缩略图网格: Esc
缩略图网格 -> 信息面板: 选择
单张查看 -> 信息面板: 切图同步
```

默认单张模式隐藏左栏，右栏约 280 DIP。
胶片栏按需展开，默认不占用画面高度。
网格模式再显示文件整理工具；顶部压缩为一条窄工具带，格式列表按需展开。
正式原生版采用可拖动分栏与虚拟化网格。

[GridView](https://mygo.egoist.dev/docs/ui/grid-view) · [Split](https://mygo.egoist.dev/docs/ui/split)

## D 16 种排序
| 分组 | 字段 |
|---|---|
| 文件 | 自然文件名、后缀、大小、修改时间 |
| 拍摄 | 时间、相机、镜头、ISO、快门、光圈、焦距 |
| 图像 | 总像素、宽度、高度、宽高比 |
| 整理 | 星标 |

所有字段支持升序与降序。
缺失值始终放末尾，数值按实际数值比较。
相同主键用自然文件名作为稳定次键。

## E 筛选规则
| 入口 | 原型能力 |
|---|---|
| 后缀快捷栏 | RAW 家族与文件夹真实存在的后缀 |
| 后缀多选 | JPG + ARW 表示两者均可显示 |
| 筛选弹层 | 相机、画面方向、ISO 范围、日期、星标 |
| 条件反馈 | 可移除 chips、结果计数、空状态 |

同类条件为 OR，不同类别为 AND。
切换文件夹默认清空筛选，保留排序与视图。

参考：[Adobe Bridge 筛选规则](https://helpx.adobe.com/uk/bridge/desktop/organize-and-find-files/organize-files-and-folders/sort-and-filter-files-and-folders.html)

## F 元数据层级
| 层级 | 内容 |
|---|---|
| 第一眼 | 光圈、快门、ISO 三项并排 |
| 常用信息 | 相机、镜头、拍摄时间、焦距 |
| 文件属性 | 格式、尺寸、体积、色彩空间、修改时间 |
| 全部信息 | 分类键值表，可复制 |

切换照片时清空旧字段，防止异步结果串图。
无 EXIF 显示“未提供”，不代入上一张参数。

参考：[Bridge 元数据](https://helpx.adobe.com/bridge/desktop/organize-and-find-files/tag-and-find-files/view-and-edit-metadata.html) · [FastRawViewer](https://www.fastrawviewer.com/usermanual17/first-steps)

## G 解码边界
| 能力 | 研究结论 |
|---|---|
| MyGo 默认解码 | JPEG、PNG、BMP、WebP、GIF 首帧 |
| RAW 候选首批 | ARW、CR2/CR3、NEF、RAF、DNG |
| RAW 首屏 | 优先内嵌预览，必要时完整解码 |
| 元数据候选 | ExifTool JSON + 常驻进程 |
| 额外验证 | HEIC/TIFF、ICC 色彩、压缩变体 |

LibRaw 是原生库，可先用随包 helper 隔离。
RAW 支持要按相机和压缩方式验收。
内嵌预览不能冒充 RAW 原像素检视。

[MyGo Image](https://mygo.egoist.dev/docs/ui/image) · [LibRaw](https://www.libraw.org/supported-cameras) · [ExifTool](https://github.com/exiftool/exiftool)

## H 原型与下一步
示例照片来自 Unsplash。
文件名、RAW 后缀和拍摄参数均为演示数据。
默认视图已经调整为“单张查看 + 右侧信息”，网格与整理工具按需展开。
本地入口可读文件属性，尚未接入 RAW 与 EXIF。
正式 MyGo 应用尚未开始实现。

下一轮重点讨论：

- 信息面板是否默认常驻。
- 深色与浅色的信息密度。
- 单张模式是否收起文件夹栏。
- 常用相机格式和首个目标平台。
