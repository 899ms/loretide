# Tailscale 远端连通与原生环境核验

用户提供两台机器在线截图，承接私网接入任务进行只读验证。Windows 本机 100.68.147.14；远端 cursor 为 100.109.104.61。

- tailscale ping 两次成功，走 DERP(nyc) 中继，约 938/298ms；未建立直接连接，不是连接失败。
- tailscale ssh box@100.109.104.61 成功执行 id/hostname 等命令，uid=1000(box)，无需输入远端密码。本轮未安装 OpenSSH Server 或 Docker。
- CPU cgroup quota 为 800000/100000，约 8 CPU；memory.max 为 17179869184（16 GiB），memory.current 约 1.53 GiB。free 显示的主机整体内存不是本环境独占可用量，不能用它推断实际还有 4 GiB 或保证全部限额随时可分配。
- 根文件系统 overlay，显示可用约 98 GiB；/home/box 同属 overlay，未证明平台回收/重建后持久性。
- Go 1.24.4、Node 20.19.2；corepack/npm/git 存在；pnpm/psql/postgres 未在 PATH 找到。sudo -n true 成功。
- 首条聚合命令退出 1 来自末尾 command -v psql 未找到，不代表 SSH 失败；第二次命令退出 0。

结论：远程终端路径已通过，可继续核对原生开发依赖及持久性。未迁移代码、安装依赖、启动应用或验证编译；现有 Hetzner 开发环境未删除。

关联：[Windows安装记录](2026-09-13-Windows安装Tailscale.md)记录客户端；[迁移与私网评估](2026-09-13-开发环境迁移与Grok私网接入评估.md)记录无 Docker 决策和现有环境；本文件记录实际连接、资源与未完成项。
