# 发布与应用更新

仓库：https://github.com/ch1y1z1/jianshan

## 持续集成

推送 `main` 或提交 PR 后，CI 在 macOS、Windows 和 Linux 上检查格式、运行 `go vet`、测试、构建正式模式二进制，并核对发布元数据。Go 与 MyGo CLI 的版本由 `app/go.mod` 固定。

推送 `v*` 标签触发发布流程：重新跑 CI，核对标签、`app/mygo.json` 和 `app/CHANGELOG.md`；构建 macOS arm64/amd64、Windows amd64、Linux amd64；验签并汇总全部归档与安装包；先上传至草稿，全部成功后公开发布。预发布标签会标记为 prerelease，不成为稳定版 latest。已公开的版本不允许被流程覆盖。

## 发布新版

1. 修改 `app/mygo.json` 的 `version`，在 `app/CHANGELOG.md` 添加对应的 `## 0.1.1` 章节。应用版本和更新说明均由此读取。
2. 提交并推送到 `main`，确认 CI 通过。
3. 在相同提交创建并推送版本标签：

   ```sh
   git tag v0.1.1
   git push origin v0.1.1
   ```

4. 在 [Actions](https://github.com/ch1y1z1/jianshan/actions/workflows/release.yml) 查看构建，在 [Releases](https://github.com/ch1y1z1/jianshan/releases) 验收安装包。失败时可重新运行失败作业；若版本已公开，修复后发布新版本。

`app/cmd/releasecheck` 会阻止标签/版本不一致、空更新记录、目标平台缺失、归档大小或签名错误，并生成覆盖全部下载文件的 `SHA256SUMS`。下载归档和 delta 必须保留原名，供 `update-<target>.json` 引用。

## 更新签名

应用内固化项目专用 Ed25519 公钥。私钥保存在本机：

```text
~/Library/Application Support/com.chiyizi.jianshan/update-keys/mygo-update.key
```

同一私钥已配置为仓库 Actions secret `MYGO_UPDATER_PRIVATE_KEY`。私钥不能提交，也不能随意重生成：更换后，现有安装将无法验证新包。请长期安全备份。迁移机器或重设 secret 时通过标准输入导入，避免将内容放入命令行参数：

```sh
gh secret set MYGO_UPDATER_PRIVATE_KEY --repo ch1y1z1/jianshan \
  < "$HOME/Library/Application Support/com.chiyizi.jianshan/update-keys/mygo-update.key"
```

本地未提供私钥的开发构建不会生成可发布的签名更新归档。发布流程缺少密钥会直接失败。

## 客户端行为

正式安装包启动后检查更新，默认每日检查一次；用户也可以从应用菜单或折叠的「工具」中手动检查。原生窗口显示版本说明，下载和安装由用户选择。开发模式、不可写安装路径和系统包管理器管理的安装可能禁用内置更新。

`updates.deltas = 3` 会尝试为最多三个已发布版本生成增量包。客户端精确匹配当前版本，优先增量；没有适用增量或应用失败时回退到全量归档。`v0.1.0` 建立首次公开签名基线，因此首版无 delta。后续版本只有补丁小于全量时才生成。保留旧版更新清单、全量归档和 delta，避免破坏未来构建的基线。

更新归档的签名独立于操作系统代码签名。当前 macOS 包仅 ad-hoc 签名，未公证；Windows 包未做证书签名。接入 Apple/Windows 分发证书需要另行配置对应凭据。
