---
title: "Instrumenting Go Binaries for Integration Test Coverage"
date: 2020-06-06T23:30:51-07:00
draft: false
summary: An approach for measuring coverage of integration or manual testing
---

# What
Instrument a Go binary in such a way that when it executes, I can calculate the code coverage during the lifespan of its execution.

# Why
A while back one of my coworkers developed an integration test suite. 
Given a runtime of our code, it would make some API calls and make assertions on the database results. 
These kinds of test suites are excellent: you get feedback on whether your changes have impacted existing behavior (or if you're practicing Test Driven Development, whether your changes work as expected).
Since this was a new tool, I wanted to measure the coverage of the test suite. 
This way, we could intelligently pick what test data to use in the suite and ensure we're covering all desired code paths.

# How


## First Pass: MVP
First, we need to write a test for the main function. 
The goal is for execution to flow through this test and into `main`.
**The test function needs to exit of its own accord in order for the coverage data to be written**, so let's just wait a bit. 
```go
package main
import (
    "testing"
    "time"
)

func Test_main(t *testing.T) {
    go main()
    time.Sleep(1 * time.Minute)
}
```
We immediately fork off a goroutine for `main()` to do the actual work, while the original test thread spends the rest of its time waiting to quit. 
We are being a bit rude to the test suite: as our execution returns from the test function, the leaked `main()` goroutine is killed.

Now we build the test binary, specifying that all packages' coverage should be measured
```shell script
go test -c -covermode=atomic -coverpkg=all -o app.debug
```
Finally, we run the instrumented binary and specify where to store the coverage data
```shell script
./app.debug -test.coverprofile=functest.cov
```
After invoking the instrumented binary, we have 60 seconds to test before the window stops and the coverage data is written. 
It works fine, but leaves a lot to be desired.

## Second Pass: Stopping
Ideally our function should gracefully handle `^C` SIGINTs and provide a timeout for scenarios where sending a SIGINT isn't feasible (like build pipelines). 
There's many more options here: HTTP kill endpoints, reading timeouts from environment variables, etc...
Again, the point of this test function is just "run `main()`, and gracefully stop when the testing is done".
A sample to support SIGINT alongside a timeout looks like this:
```go
func Test_main(t *testing.T) {
	go main()
	time.Sleep(10 * time.Second)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
    select {
        case <- c:
            return
        case <- time.After(1 * time.Minute):
            return
    }
}
```
Our builds and invocations remain the same:
```shell script
go test -c -covermode=atomic -coverpkg=all -o app.debug
./app.debug -test.coverprofile=functest.cov
```

## Third Pass: Code worth keeping
The above two solutions share the same problem: the test is treated as a normal unit test.
During the course of normal development these tests would execute, causing long waits and creating unwanted coverage data. 
To resolve this we can add a [build constraint][build constraint] to enforce this file is only built when specifically desired.
The only code change is adding the build tag at the top of the file:
```go
// +build manual-integration

package main
import (
    "testing"
    "time"
)

func Test_main(t *testing.T) {
	go main()
	time.Sleep(10 * time.Second)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
    select {
        case <- c:
            return
        case <- time.After(1 * time.Minute):
            return
    }
}
```
At compilation time we need to specify the `manual-integration` tag
```shell script
go test -c -tags=manual-integration -covermode=atomic -coverpkg=all -o app.debug
./app.debug -test.coverprofile=functest.cov
```
This allows us to keep the test around as long as it is in its own file.
The test can safely be committed and won't impact any existing usage, as it must manually be invoked via the build tag.

[build constraint]:https://golang.org/pkg/go/build/#hdr-Build_Constraints