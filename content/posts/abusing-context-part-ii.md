---
title: "Abusing Context in Go Part II: Recovery"
date: 2020-06-04T16:31:47-07:00
draft: false
summary: A method for recovering context in Go that was omitted somewhere up the callstack
---

While not required, it's highly recommended that you read Dave Cheney's post on [Dynamically Scoped Variables][dynamically scoped variables] before continuing. 

## What and Why? 

---
Consider an instance of [`context.Context`][go context package] as it flows through the code. 
Typically it's created at the beginning of a transaction and enriched or referenced throughout the transaction. 
Sometimes it's not passed everywhere -- it certainly doesn't need to be.
Consider a scenario where you have an encryption library and it contains a simple function `AESEncrypt`:
```go
package crypto
import (
"crypto/aes"
"crypto/cipher"
"crypto/rand"
"github.com/spf13/viper"
"io"
)
func AESEncrypt(keyID string, plaintext []byte) (ciphertext, nonce []byte, err error) {

    key := viper.GetString(keyID) // retrieve the key from config file
      
    block, err := aes.NewCipher([]byte(key))
    if err != nil {
      return  // key was malformed?
    }
  
    nonce = make([]byte, 12)
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
      panic(err.Error()) // if you can't read your random, you've got problems
    }
   
    aesgcm, err := cipher.NewGCM(block)
    if err != nil {
      panic(err.Error()) // inputs are effectively static
    }
   
    ciphertext = aesgcm.Seal(nil, nonce, plaintext, nil)
    return
}
```
The function reads in an encryption key, builds a cipher, and encrypts the input. 
It works faithfully for a few months, until someone decides keys need to be stored on a keyserver and retrieved at runtime. 
Now the earlier call 
```go
viper.GetString(keyID) // retrieve the key from config file
```
must change to
```go
key := keyserverClient.GetKey(context.TODO(), keyID) // retrieve the key from the keyserver
```
and now we have no context to provide. 
In a perfect world, we'd refactor the `AESEncrypt` to change the function signature to add context support.
But I'm lazy and want a different approach.

We had the context further up the callstack, but it was omitted at some point in the stack.  We want it back now.

## How?

---
### The Limitations

To begin this journey, let's note some of the restrictions placed on `contex.Context`. 
Each restriction limits the work we have to do: narrower scope means less scenarios to handle.
 - A context SHOULD be the first parameter in the function signature. The `go lint` linter `lintContextArgs` [checks for this][lint context args]
 - Context is an interface which contains four unexported implementations under the hood: [deadline][timer ctx], [cancel][cancel ctx], [value][value ctx], and [empty][empty ctx].

So we always know where a context will be in a function signature and also the legal values of its underlying concrete type.
The next tools for solving the problem come from understanding the internals of go.  Specifically, the `interface`

Interfaces, such as the `context.Context` interface, are constructed by two parts: an `itab` and the data.
The `runtime2.go` [source][interface source] has it defined simply as:
```go
type iface struct {
	tab  *itab
	data unsafe.Pointer
}
```
`itab`, short for "itable", short for "interface table", contains all the details the go linker and runtime require for [ducktyping][ducktyping] to work.
A more in-depth view is available at the [go-internals Interfaces][go-internals Interfaces] chapter.
For the time being, we need to know two things:
- The `itab` and `data` pointers make up an interface
- Different instances of the same concrete interface implementation will recycle the same `itab`

### Interfaces at Runtime
At runtime, interfaces aren't passed as the single `iface` struct we saw defined in `runtime2.go`. 
Instead, they're passed as the contents: the `itab`, `data` tuple. 
We can demonstrate this with the below trivial example: a 2 frame stack, where the caller places a single interface as a parameter to the callee. 
```go
import "context"

func main() {
	panicker(context.WithValue(context.Background(), "key", "oops!"))
}

//go:noinline
func panicker(ctx context.Context) {
	panic(ctx.Value("key"))
}
```
In the resulting output stacktrace, the callee shows two parameters:
```
panic: oops!

goroutine 1 [running]:
main.panicker(0x1099b60, 0xc000068060)
        /Users/aidan/go/src/github.com/raidancampbell.github.io.source/content/scratch/abusing-context-part-ii.go:11 +0x61
main.main()
        /Users/aidan/go/src/github.com/raidancampbell.github.io.source/content/scratch/abusing-context-part-ii.go:6 +0x7a
```
The bottom of the stack (using the "stack grows downwards" terminology, whereas the stacktrace shows current execution at the top) has the two values in question: `main.panicker(0x1099b60, 0xc000068060)`. 
While only one was actually passed to the function, two appeared in the call stack. 
Why? I'm not sure why Go does it this way. 
My guess is that the majority of the `itab` is constructed at compile (linking) time, and is required for the way interfaces are treated in go.
This theory is reinforced by the memory address offset: the `itab` appears much earlier in memory compared to the data.

### Rebuilding an interface
Let's modify the above example to gain access to the stack as a string:
```go
package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"runtime"
)

func main() {
	panicker(context.WithValue(context.Background(), "key", "oops!"))
}

func panicker(_ context.Context) {
	var buf [8192]byte
	n := runtime.Stack(buf[:], false) // get the current callstack as a string
	sc := bufio.NewScanner(bytes.NewReader(buf[:n]))
	for sc.Scan() {
		fmt.Println(sc.Text())
	}
}
```
Minus the panic's message, the results remain the same:
```
goroutine 1 [running]:
main.panicker(0x10ee820, 0xc000090180)
        /Users/aidan/go/src/github.com/raidancampbell.github.io.source/content/scratch/abusing-context-part-ii.go:17 +0x69
main.main()
        /Users/aidan/go/src/github.com/raidancampbell.github.io.source/content/scratch/abusing-context-part-ii.go:12 +0x7a
```
We know `0x10ee820` and `0xc000090180` compose the input context for `panicker`. 
Using the [`unsafe` package][unsafe package] we can rebuild the incoming context:
```go
package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"regexp"
	"runtime"
	"unsafe"
)

func main() {
	panicker(context.WithValue(context.Background(), "key", "oops!"))
}

var twoParamPatt = regexp.MustCompile(`^.+[a-zA-Z][a-zA-Z0-9\-_]*\.[a-zA-Z][a-zA-Z0-9\-_]*\((?P<type_itab>0x[0-9a-f]+), (?P<type_data>0x[0-9a-f]+).+`)

func panicker(_ context.Context) {
	var buf [8192]byte
	n := runtime.Stack(buf[:], false) // get the current callstack as a string
	sc := bufio.NewScanner(bytes.NewReader(buf[:n]))
	for sc.Scan() {
		// match the expected regex.  for this example, we're only expecting the below match (addrs will vary):
		// main.panicker(0x10ee820, 0xc000090180)
		matches := twoParamPatt.FindStringSubmatch(sc.Text())
		if matches == nil {
			continue
		}

		// grab the two memory addresses (itab and data value)
		var p1, p2 uintptr
		_, err1 := fmt.Sscanf(matches[1], "%v", &p1)
		_, err2 := fmt.Sscanf(matches[2], "%v", &p2)
		if err1 != nil || err2 != nil {
			continue
		}

		// put the two pointers into the iface layout
		idata := [2]uintptr{p1, p2}

		// declare that the iface is a context
		ctx := *(*context.Context)(unsafe.Pointer(&idata))
		
		// use the context
		fmt.Println(ctx.Value("key"))
	}
}
```
Walking through the above code:
```go
var twoParamPatt = regexp.MustCompile(`^.+[a-zA-Z][a-zA-Z0-9\-_]*\.[a-zA-Z][a-zA-Z0-9\-_]*\((?P<type_itab>0x[0-9a-f]+), (?P<type_data>0x[0-9a-f]+).+`)
```
A rather nasty regex for matching the "<package>.<func>(arg1, arg2" part of a stacktrace.
```go
var p1, p2 uintptr
_, err1 := fmt.Sscanf(matches[1], "%v", &p1)
_, err2 := fmt.Sscanf(matches[2], "%v", &p2)
if err1 != nil || err2 != nil {
    continue
}
```
Extracting the string hex addresses into actual `uintptr`.  In the above execution, these would be `0x10ee820` and `0xc000090180`
```go
idata := [2]uintptr{p1, p2}
```
This is placing two pointers in the same layout as the `iface` struct: `itab` pointer first, data pointer second.
Remember that the internal struct for an interface looks like this:
```go
type iface struct {
	tab  *itab
	data unsafe.Pointer
}
```
```go
ctx := *(*context.Context)(unsafe.Pointer(&idata))
```
Create an unsafe pointer to the effective `iface` struct instance, then cast that pointer as a `context.Context` pointer, then dereference the `context.Context` pointer for your final value. 
At this point we've "recovered" the context, but it's a bit moot since we were given the context to begin with. 
How would we handle true context recovery if it existed somewhere up the stack, but was never passed to us?

### Context recovery through dynamic scoping
The above code has a major fatal flaw: it finds the first function up the stack with two parameters, and blindly jams them into a `context.Context`. 
In reality we don't know if the first two parameters are the `itab` and data pointers: they may be ints, or even an `itab` and data pointer for an unrelated interface.

Here we call upon our initial restrictions: `context.Context` should always be first, and we know the four possible implementations of context. 
Knowing that there's four possible implementations allows us to enumerate the legal itab pointers on invocation:
```go
func RecoverCtx() (context.Context, error) {
	return emptyItab(context.Background())
}

//go:noinline
func emptyItab(_ context.Context) (context.Context, error) {
	return valueItab(context.WithValue(context.Background(), "", ""))
}

//go:noinline
func valueItab(_ context.Context) (context.Context, error) {
	ctx, c := context.WithCancel(context.Background())
	defer c()
	return cancelItab(ctx)
}

//go:noinline
func cancelItab(_ context.Context) (context.Context, error) {
	ctx, c := context.WithDeadline(context.Background(), time.Now())
	defer c()
	return timerItab(ctx)
}

//go:noinline
func timerItab(_ context.Context) (context.Context, error) {
	return doGetCtx()
}

func doGetCtx() (context.Context, error) {
    // TODO
}
```
The above outlines a call chain: the entry point is the exported `RecoverCtx` function, which passes through several other functions on its way down to the actual implementation. 
Each of these functions receives a context, then passes it to the next function. 
When viewing the stack at runtime, it would look like this:
```
libraidan/pkg/runsafe.doGetCtx(0x0, 0x0, 0x0, 0x0)
	/Users/aidan/go/src/libraidan/pkg/runsafe/context.go:59 +0xbb
libraidan/pkg/runsafe.timerItab(0x14b5ce0, 0xc0000a44e0, 0x0, 0x0, 0x0, 0x0)
	/Users/aidan/go/src/libraidan/pkg/runsafe/context.go:52 +0x4c
libraidan/pkg/runsafe.cancelItab(0x14b5c60, 0xc0000e8c40, 0x0, 0x0, 0x0, 0x0)
	/Users/aidan/go/src/libraidan/pkg/runsafe/context.go:47 +0x1a5
libraidan/pkg/runsafe.valueItab(0x14b5d20, 0xc00009d320, 0x0, 0x0, 0x0, 0x0)
	/Users/aidan/go/src/libraidan/pkg/runsafe/context.go:40 +0x150
libraidan/pkg/runsafe.emptyItab(0x14b5ca0, 0xc0000a6008, 0x0, 0x0, 0x0, 0x0)
	/Users/aidan/go/src/libraidan/pkg/runsafe/context.go:33 +0xd7
libraidan/pkg/runsafe.RecoverCtx(0x0, 0x0, 0x0, 0x0)
``` 
Using this stack, we can build guarantees: 
- we know how many functions live above our call to `runtime.Stack`
- we know the name of each of these functions
- we know the concrete input type (effectively the `itab`) each function receives

Why all the `//go:noinline` pragmas? The compiler would inline the trivial functions, and we'd lose the parameter addresses.
The same stack, but without inlining disabled:
```
libraidan/pkg/runsafe.doGetCtx(0x13a1520, 0xc0000a6008, 0xbfae3de322870128, 0xbc60e)
	/Users/aidan/go/src/libraidan/pkg/runsafe/context.go:59 +0x5b
libraidan/pkg/runsafe.timerItab(...)
	/Users/aidan/go/src/libraidan/pkg/runsafe/context.go:52
libraidan/pkg/runsafe.cancelItab(0x13a14e0, 0xc0000e09c0, 0x0, 0x0, 0x0, 0x0)
	/Users/aidan/go/src/libraidan/pkg/runsafe/context.go:47 +0x9e
libraidan/pkg/runsafe.valueItab(0x13a15a0, 0xc00009d320, 0x0, 0x0, 0x0, 0x0)
	/Users/aidan/go/src/libraidan/pkg/runsafe/context.go:40 +0x82
libraidan/pkg/runsafe.emptyItab(0x13a1520, 0xc0000a6008, 0xc000040680, 0x100e808, 0x30, 0x130a680)
	/Users/aidan/go/src/libraidan/pkg/runsafe/context.go:33 +0x7e
libraidan/pkg/runsafe.RecoverCtx(...)
```
With the parameters elided, we lose the `itab` pointers and can't enumerate the legal values. 
In this scenario, we would be missing out on recovering certain context implementations.

With all the above in mind, we're finally ready to implement the `doGetCtx()` function:
```go
func doGetCtx() (context.Context, error) {
	var buf [8192]byte
	n := runtime.Stack(buf[:], false) // get the current callstack as a string
	sc := bufio.NewScanner(bytes.NewReader(buf[:n]))
	var (
        // hold the type itab pointers for each of the context implementations
		deadlineType, cancelType, valueType, emptyType uintptr 
         // used to count our way up the stack, 
         // as the stack is constant the lowest few levels and we need to leverage that
		stackMatch int    
	)
	for sc.Scan() { // for each line (walking up the stack from here)
		// if the line doesn't match, skip.
		matches := pattern.FindStringSubmatch(sc.Text())
		if matches == nil {
			continue
		}
		// if this is the first iteration, then it's just our function. skip it.
		if stackMatch == 0 && strings.Contains(sc.Text(), "doGetCtx") {
			continue
		}

		stackMatch++

		// grab the two memory addresses (itab and type value)
		var p1, p2 uintptr
		_, err1 := fmt.Sscanf(matches[1], "%v", &p1)
		_, err2 := fmt.Sscanf(matches[2], "%v", &p2)
		if err1 != nil || err2 != nil {
			continue
		}

		// build up the legal values for each implementation of context
		// the stackMatch must match the known location in the stack.
		// Otherwise we might return a malformed context
		if stackMatch == 1 && strings.Contains(sc.Text(), "timerItab") {
			deadlineType = p1
		} else if stackMatch == 2 && strings.Contains(sc.Text(), "cancelItab") {
			cancelType = p1
		} else if stackMatch == 3 && strings.Contains(sc.Text(), "valueItab") {
			valueType = p1
		} else if stackMatch == 4 && strings.Contains(sc.Text(), "emptyItab") {
			emptyType = p1
		} else if p1 != emptyType && p1 != valueType && p1 != cancelType && p1 != deadlineType {
			// if we're in the caller's code, and the first parameter isn't a 
			// known context implementation, then skip this stack frame
			continue
		}

		if stackMatch <= 4 { // we're still building the legal context implementations
			continue
		}

		// at this point we're done building the legal context implementations, 
		// and this matched one. rebuild a context from the addresses, and return
		idata := [2]uintptr{p1, p2}
		return *(*context.Context)(unsafe.Pointer(&idata)), nil
	}
	// no context was found.  Return a non-nil context to be polite, but also return an error.
	return context.Background(), UnrecoverableContext{}
}
```
We leverage the known frames as we move up the callstack from `doGetCtx` to `RecoverCtx`. 
Each of these frames is verified by name and location, then the `itab` pointer value is stored. 
Once the we've left the comfort of our guaranteed callstack, we need to match the first address against one of the known `itab` values. 
If one is found, then the resulting context is built and returned. 
But there's no guarantee this will happen: the context could be elided due to inlining in the caller's code, or the caller could have never had a context to begin with!

The full code, which I do not recommend using, is available in libraidan's [runsafe package][runsafe].


[dynamically scoped variables]:https://dave.cheney.net/2019/12/08/dynamically-scoped-variables-in-go
[go context package]:https://golang.org/pkg/context/
[lint context args]:https://github.com/golang/lint/blob/master/lint.go#L1413
[timer ctx]:https://golang.org/src/context/context.go#L427
[cancel ctx]:https://golang.org/src/context/context.go#L232
[value ctx]:https://golang.org/src/context/context.go#L513
[empty ctx]:https://golang.org/src/context/context.go#L171
[interface source]:https://github.com/golang/go/blob/bf86aec25972f3a100c3aa58a6abcbcc35bdea49/src/runtime/runtime2.go#L143-L146
[ducktyping]:http://en.wikipedia.org/wiki/Duck_typing
[go-internals Interfaces]:https://cmc.gitbook.io/go-internals/chapter-ii-interfaces#anatomy-of-an-interface
[unsafe package]:https://golang.org/pkg/unsafe/
[runsafe]:https://github.com/raidancampbell/libraidan/blob/master/pkg/runsafe/context.go#L23