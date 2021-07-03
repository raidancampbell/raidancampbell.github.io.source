---
title: "Low Effort Blog Posts Part II: Golang Profiling"
date: 2021-07-03T09:12:27-04:00
draft: false
summary: It's callstacks all the way down
---

## Golang profiling is mostly just callstack aggregation

Out of the box the Go runtime exposes a handful of profiling options, most commonly the CPU and memory profilers.

The CPU profiler is roughly the equivalent of "100 times a second, show me the stacks of all running goroutines".
"running" here means actually running, not paused for IO, sleeping, or mutex contention, so it's only useful for diagnosing high CPU usage.

The heap (memory) profiler is similar: "roughly once per 512KB of allocated memory, capture the callstack of the allocating goroutine", decorated with how much memory they allocated.
There's some more data that the memory management / garbage collector will expose (e.g. how much of this memory is in use vs ever allocated, or how many allocations were performed).