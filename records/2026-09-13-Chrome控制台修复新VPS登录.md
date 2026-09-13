# Chrome 控制台修复新 VPS 登录

## 授权与定位

用户要求操作其 Chrome 浏览器修复 78.47.42.189 的 SSH 登录。操作 Hetzner 项目 10956871、服务器 165652087 的现有控制台，复用已登录的 root 会话。

实际 sshd -T：permitrootlogin prohibit-password，passwordauthentication yes。/etc/ssh/sshd_config 第 54 行为注释的默认示例。根因是 SSH 禁止 root 密码认证；控制台登录成功并不能证明 SSH 允许该认证方式。之前不能仅凭 Authentication failed 判断密码错误。

## 修复与验证

- 在确认文件不存在后新增 /etc/ssh/sshd_config.d/00-loretide-root-login.conf，仅包含 PermitRootLogin yes。
- sshd -t 成功后 systemctl reload ssh；没有重启服务器或创建账号，没有改密码、已有密钥或防火墙。
- 本机 Paramiko 固定此前 ED25519 指纹，通过交互式 getpass 使用最新密码认证，明确返回 Authenticated；远程 id 返回 uid=0(root)，hostname 为 ubuntu-8gb-nbg1-2，sshd -T 返回 permitrootlogin yes。
- 密码未保存至文件、命令行参数或版本库。主机指纹仍是固定已观察值，未补做控制台公钥指纹独立比对。
- 如需撤回本次登录策略变更，可删除本次新增的精确配置文件，运行 sshd -t 后 reload ssh；这会恢复 root 密码 SSH 限制。当前按用户要求保留 root 密码登录。

## 新机实际资源

4 CPU；总内存 7.6 GiB，可用 7.1 GiB；无 swap。根分区 75 GiB，使用 1.4 GiB，可用 71 GiB。Git 存在，Docker 未在 PATH 找到。未部署或迁移应用。新机与旧机内存规格相同，优势是专用于开发，不能承诺并行冷编译容量已提升或通过验证。

## 文件关联

- 本文件：已验证根因、修改、恢复方式与资源检查证据。
- [此前准备与失败记录](2026-09-13-附件持久化与新开发服务器准备.md)：保留先前尝试历史；登录阻塞以本次成功证据更新。
- app/compose.loretide-dev.yaml：后续部署配置，本次未修改或部署到新机。

## 操作限制与中间失败

浏览器 canvas 不支持 pressSequentially，改用逐键输入；首次管道符被键盘映射为反斜杠，仅导致只读命令报错，修正 Shift 键映射后获取配置。一次本机验证命令因 PowerShell 引号解析失败未连接服务器，修正后认证成功。
