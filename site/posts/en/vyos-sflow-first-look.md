---
title: Who is eating my VyOS traffic? This little tool shows it at a glance 😭
date: 2026-11-21
summary: The office network crawls at peak hours and nobody knows who is eating the bandwidth? One binary fixes it: bandwidth, top talkers and flow records, for anything that exports sFlow, in a browser or over SSH. Up and running in 30 seconds.
---

The pain first 👇

The VyOS box at our office egress has been running for a long time.
Every workday peak, the network crawls.
Who is eating the bandwidth? No idea.
Which machine, talking to which server? No idea either.
Pure guessing. The answer was always "try a reboot" 🙂

VyOS can export sFlow samples.
So can most switches, other brands of routers, and servers.

But then what? There was nothing to look at them with ❌
The usual tools want a separate server, or a database and a whole dashboard stack.
Way too much, just to see my own traffic.

So I wrote a small tool myself. It is called vyos-sflow ✨
One binary.
Run it on the router itself,
or put it on any Linux machine and let every device send its sFlow there.

## 📦 Three steps to install

First, turn on sFlow in VyOS (skip this if it already sends to 127.0.0.1:6343):

```
configure
set system sflow interface eth0
set system sflow server 127.0.0.1 port 6343
commit
save
exit
```

Not a VyOS box? Same idea — point that device's sFlow export at this machine, port 6343, and continue below.

Then download it and run it:

```
curl -fLO https://github.com/githubflyideas/vyos-sflow/releases/download/v1.3.3/vyos-sflow-1.3.3-linux-amd64.tar.gz
tar -xzf vyos-sflow-1.3.3-linux-amd64.tar.gz
cd vyos-sflow-1.3.3-linux-amd64 && ./vyos-sflow serve
```

Open `http://ROUTER_IP:8066` in a browser, sign in with admin / tryvyossflow66, and change the password.
That is all. The charts start filling in.

![Overview page](assets/img/screenshots/vyos/web-overview.png)

## 👀 What you get

✅ Current bandwidth, and the curve for the last 48 hours

✅ Top talkers: which client and which server use the most traffic, ranked

✅ Interface traffic from the device's own counters, next to the sampled estimate, so you can see if the numbers line up

✅ The flow records behind the charts — click a spike and find out who it was

✅ If a chart is empty, the Sources page tells you why. No guessing

![Top talkers](assets/img/screenshots/vyos/web-top.png)

## 🖥️ No browser? Use the terminal

SSH into the router and run one command:

```
./vyos-sflow tui
```

The same picture, in your terminal. Press q to quit.
Nothing gets installed, which is nice for a quick look.

Small extras: the web UI speaks 15 languages and has five themes, light to dark.

## ⚠️ The honest limits

- sFlow only. No NetFlow, no IPFIX
- History is 48 hours. It answers "what is happening" and "what just happened", not long-term reporting
- Linux amd64 only. That covers VyOS and normal Linux servers, but there is no Windows or macOS build

If those three are fine for you, it does the job well.

## 🔗 Get it

Free and open source, Apache-2.0.
Search GitHub for vyos-sflow, or use the download link on this site.
If you don't like it, delete the folder and it's gone — the router itself is untouched.

If VyOS runs your office egress or an internet-café gateway, give it a try.
That "where did my traffic go" feeling is gone.

What do you use to watch your egress traffic? Tell me below 👇

#VyOS #sFlow #opensource #networking #sysadmin
