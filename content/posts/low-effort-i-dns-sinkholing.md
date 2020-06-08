---
title: "Low Effort Blog Posts Part I: DNS Sinkholing"
date: 2020-06-07T21:58:09-07:00
draft: false
summary: Keep your sensitive data flowing on trusted (controlled) networks, but sinkhole them outside
---

# What
I want my laptop to route certain requests differently on my home network, and prevent them on public networks.

# Why
I wanted to try this [syslog][syslog] thing, but didn't like the idea of hardcoding a local IP. 
Even within the `192.168.0.0/16` block, switching wifi networks (e.g. going to a coffee shop) would mean my laptop would try and send logs to the configured IP.  No good.

# How
Configure your router to resolve something like `sinkhole.mydomain.com` to your desired server. 
In my case, this was the syslog server I kept on my home network within the `192.16.0.0/16` block. 
Additionally, configure `sinkhole.mydomain.com` to `127.0.0.1` (or anything in the `127.0.0.0/8` block). 
This way requests are successful, but your data never leaves your laptop. 
Requests to `sinkhole.mydomain.com` will continue to fail until you're back on your home network. 
Most modern operating systems will flush the DNS cache when switching networks, making any caching a non-issue.

[syslog]:https://en.wikipedia.org/wiki/Syslog