---
title: "Abusing Context in Go Part I: Serialization"
date: 2020-05-23T21:29:46-07:00
draft: true
---

At the extreme end of microservice architecture, network communication becomes the substitution for function calls.
This change forces many compromises in Go, mostly because everything sent across the network must be able to be serialized and deserialized.
Luckily, the Go standard library includes an `encoding` package