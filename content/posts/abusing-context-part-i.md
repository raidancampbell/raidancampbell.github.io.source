---
title: "Abusing Context in Go Part I: Serialization"
date: 2020-05-23T21:29:46-07:00
draft: true
---

At the extreme end of microservice architecture, network communication becomes the substitution for function calls.
This change forces many compromises in Go, mostly because everything sent across the network must be able to be serialized and deserialized.
Luckily, the Go standard library includes an [`encoding` package][encoding package], which handles most serialization use cases.

For most use cases encoding data via JSON will work. JSON is a rock-solid "good enough" and ubiquitous solution.  
For less textbook scenarios, Go provides the convenient [`Gob` encoder][gob source] to encode/decode structs.  
Gob's easy API comes with a few caveats: functions can't be serialized and unexported values must be ["registered"][gob register].  


[encoding package]:TODO
[gob source]:TODO
[gob register]:TODO