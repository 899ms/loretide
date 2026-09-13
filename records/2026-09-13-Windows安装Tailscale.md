# Windows 安装 Tailscale

用户说明本机尚未安装，承接私网接入工作进行安装。通过 winget 的 Tailscale.Tailscale 官方安装包安装 1.102.4，安装器哈希验证成功。

首次 MSI 返回 1603；日志明确必需服务 iphlpsvc 被禁用。普通权限 Set-Service 返回拒绝访问；通过管理员 PowerShell 将 IP Helper 改为 Automatic 并启动，确认 Auto/Running。再次安装成功。

验证：C:/Program Files/Tailscale/tailscale.exe version 返回 1.102.4；Tailscale 服务 Running；status 为 Logged out。已调用登录入口并在 Chrome 打开设备认证页面，交由用户选择账号完成登录。尚未加入 tailnet，也未连接 Grok 环境。未设置出口节点或子网路由，未安装 Docker。

关联文件：本文件记录安装、系统依赖变更和验证；[迁移与私网评估](2026-09-13-开发环境迁移与Grok私网接入评估.md)记录整体接入方案。安装程序位于 C:/Program Files/Tailscale；首次失败日志位于 Windows AppInstaller DiagOutputDir 的 Tailscale.Tailscale.1.102.4-26-09-13-13-53-13.log。设备登录链接不持久化到文档。

官方来源：https://tailscale.com/download/windows 。

## 后续用户决定：仅 Tailscale

用户明确只需要 Tailscale，不需要 OpenSSH。本机未安装 OpenSSH Server，本次不新增。远端接入方案由普通 openssh-server 改为优先验证 Tailscale 内置 SSH：需要受支持的远端环境、启用 Tailscale SSH、允许对应用户的访问规则及实际登录测试。仅安装 Tailscale 不表示远端命令执行已经可用；当前尚未验证。现有 Windows SSH 客户端与 OpenSSH Server 不同，不因该决定擅自卸载既有客户端。
