---
title: VyOS 流量到底被谁吃了？装上这个小工具，一眼看清😭
date: 2026-11-22
summary: 出口网一到上班高峰就卡，又不知道谁在吃带宽？一个二进制搞定：带宽、Top 排行、流记录全有了，支持 sFlow 的设备都能用，浏览器和终端都能看，30 秒装完。
---

先说痛点👇

办公区出口这台 VyOS 跑了很久。
一到上班高峰，网就卡。
谁在吃带宽？不知道。
哪台机器、连的哪个服务器？也不知道。
全靠猜。问就是"重启试试"🙂

其实 VyOS 能开 sFlow，把流量采样导出来。
交换机、别的牌子的路由器、服务器，大多也能开。

但导出之后呢？没地方看❌
市面上的工具，要么另搭一台服务器，要么数据库加看板装一整套。
就为了看一眼自己的流量，太重了。

所以我自己写了个小工具，叫 vyos-sflow✨
一个二进制文件。
装在路由器上直接跑也行；
装在一台 Linux 上，收所有设备发来的 sFlow 也行。

## 📦 装它只要 3 步

先在 VyOS 里开 sFlow（已经开着的跳过）：

```
configure
set system sflow interface eth0
set system sflow server 127.0.0.1 port 6343
commit
save
exit
```

设备不是 VyOS 也一样：把它的 sFlow 发到这台机器的 6343 端口，下面照走。

然后下载、运行：

```
curl -fLO https://github.com/githubflyideas/vyos-sflow/releases/download/v1.3.3/vyos-sflow-1.3.3-linux-amd64.tar.gz
tar -xzf vyos-sflow-1.3.3-linux-amd64.tar.gz
cd vyos-sflow-1.3.3-linux-amd64 && ./vyos-sflow serve
```

浏览器打开 `http://路由器IP:8066`，用 admin / tryvyossflow66 登录，进去先改密码。
就这三步，图就出来了。

![总览页](assets/img/screenshots/vyos/web-overview.png)

## 👀 打开就能看懂

✅ 现在带宽多少，最近 48 小时的曲线长什么样

✅ Top 排行：哪个客户端、哪个服务器最能吃流量，一目了然

✅ 接口流量用的是设备自己的计数器，和采样估算摆在一起，数字对不对得上自己看

✅ 一条一条的流记录，图上的高峰点进去能查到是谁

✅ 图要是空的，Sources 页会告诉你为什么，不用自己猜

![Top 排行](assets/img/screenshots/vyos/web-top.png)

## 🖥️ 不想开浏览器也行

SSH 登上路由器，敲一行：

```
./vyos-sflow tui
```

终端里直接看同样的图，看完按 q 退出。
不装服务，不占地方，临时看一眼特别方便。

还有个小的：界面有 15 种语言、5 个主题，深色浅色都有。

## ⚠️ 先说缺点，不吹

- 只支持 sFlow。NetFlow、IPFIX 不行
- 历史只存 48 小时。它是用来看"现在"和"刚发生什么"的，不是长期报表系统
- 只有 Linux amd64 版。VyOS 和普通 Linux 服务器够用，Windows、macOS 没有

能接受这三条，它就很称手。

## 🔗 链接

免费开源，Apache-2.0。
GitHub 搜 vyos-sflow 就能找到，网站上也有下载。
装错了不要紧，删掉文件夹就干净了，路由器本身什么都不改。

办公区出口、网吧网关这种地方在用 VyOS 的，真的建议试一下。
以前那种"流量去哪了"的抓瞎感，没了。

你们那儿出口的流量都用什么看？评论区说说👇

#VyOS #网络运维 #开源工具 #sFlow #路由器 #运维日常
