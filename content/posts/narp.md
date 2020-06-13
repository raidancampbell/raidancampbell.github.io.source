---
title: "narp"
date: 2020-06-07T08:11:47-07:00
draft: false
summary: A novel network attack preventing new clients from acquiring an IP address
center: the code just checks for the presence of this flag
---
Address Resolution Protocol, detailed in [RFC 826][RFC 826], provides a simple marriage between the [link layer][link layer] and the [network layer][network layer]. 
For a given subnet, any host can broadcast an ARP request for another host on the same subnet. 
All hosts on the network receive the broadcast and answers are unauthenticated, making ARP the subject of many [network attacks][arp spoofing].

[RFC 5227][RFC 5227] expands upon the initial ARP specification by providing a new type of ARP frame, known as an ARP Probe. 
An ARP Probe is meant to prevent IP address collisions. 
When a host first wishes to use an IP address on a given network, RFC 5227 compliant operating systems must send an ARP Probe for the desired IP address. 
After a timeout period, the host considers the IP address unused, and is free to claim it.

During this timeout window, an attacker has the opportunity to maliciously answer the probe with a forged ARP reply. 
An implementation of this is available [here][narp] for educational use only.


[RFC 826]:https://tools.ietf.org/html/rfc826
[link layer]:https://en.wikipedia.org/wiki/Data_link_layer
[network layer]:https://en.wikipedia.org/wiki/Network_layer
[arp spoofing]:https://en.wikipedia.org/wiki/ARP_spoofing
[RFC 5227]:https://tools.ietf.org/html/rfc5227
[narp]:https://github.com/raidancampbell/narp