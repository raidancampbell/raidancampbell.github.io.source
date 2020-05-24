---
title: "Design Considerations for Context in Go"
date: 2020-05-22T14:58:47-07:00
draft: false
---

## A brief introduction to Context
 > "Package context defines the Context type, which carries deadlines, cancellation signals, and other request-scoped values across API boundaries and between processes." - [golang.org](https://golang.org/pkg/context/)

Context in Go is often critiqued for handling two distinct scenarios under one tool: [cancellation][context is for cancellation] and [scoped data][context is not for cancellation]. 
While I tend to agree, this post is aimed to inform rather than persuade.

Contexts are derived: each child context is made by adding additional cancellations or values from its parent. 
As soon as those contexts fall out of scope their additions, whether they be tighter deadlines or additional data, are lost.

{{< figure src="/media/context_composability.svg" >}}

Leveraging this, we can achieve scoping outside the formal interpretation of the language. A variable stored in a context can be accessed by anyone receiving that context, whereas a variable created in a function must be passed through.  
We can use this to carve our own lines on what "scope" means for our context: 
- a single HTTP request, tracked from start to finish

{{< figure src="/media/context_http.svg" >}}

- an event processing through a chain, where each link enriches the context

{{< figure src="/media/context_chain.svg" >}}

- an event processing through a ruleset, where each rule applies its own context, which is lost after the rule completes

{{< figure src="/media/context_rulechain.svg" >}}

`context.Background()` is the "root" context.  All child contexts are derived from this, and you can have multiple root contexts.  It's not useful on its own.  
`context.TODO()` [is identical to][stdlib context todo and background] `context.Background()`, except it leverages the fact that IDEs usually flag anything "TODO" with a warning. 
`context.TODO()` is generally used for code that's missing connecting dots: somewhere up the callstack has context support, but the context was omitted somewhere up the stack.

[how to correctly use context in go]:https://medium.com/@cep21/how-to-correctly-use-context-context-in-go-1-7-8f2c0fafdf39
[context is for cancellation]:https://dave.cheney.net/2017/01/26/context-is-for-cancelation
[context is not for cancellation]:https://dave.cheney.net/2017/08/20/context-isnt-for-cancellation
[stdlib context todo and background]:https://golang.org/src/context/context.go#L200

### Context for cancellation
Imagine a scenario in which we guarantee an SLA response time of 100 milliseconds. 
Our code takes 50 milliseconds to respond, unless some cache is stale, in which case a network timeout would take 1000 milliseconds to fail.
Once the request hits our HTTP endpoint we can create a context for it and specify a deadline of, for example, 90 milliseconds. 
All context-respecting functions will first check if the deadline has passed: if it has, then they return immediately. 
For any I/O, we don't just wait on the I/O to complete.  We must also wait on the context deadline to hit.  If the deadline is hit, we return immediately.
See the [golang.org example][deadline example] for a better idea.

[deadline example]:https://golang.org/src/context/example_test.go#L57

### Context for storage
Imagine we wanted to tag an ID to each request.  This would be useful to trace logs, or identify a unique event. 
This is a perfect scenario for `context.WithValue()`. 
`WithValue` expects each value it's given to be identified by a key, similar to a `map[interface{}]interface{}`.
When adding a value to the context, it's good practice to use a key that's a struct. 
Using a struct leverages the type system to limit scope and guarantee uniqueness.

##### Here be Footguns
> Footgun: "A gun which is apparently designed for shooting yourself in the foot." - [Urban Dictionary](https://www.urbandictionary.com/define.php?term=footgun)

Golang is a statically typed language, this was chosen by the language creators and trying to subvert this is a Bad Idea™.
Context, used as a `map[interface{}]interface{}`, can hold all of your variables: you can have a function that just receives a single `context.Context`.
If I catch you doing this, I'll tell your mother. Do not, under any circumstances, store critical data in the context. 
There's no guarantees that the data will be there at runtime and everyone can read each other's variables, it's just a mess.  Please don't.

When using context for storage, the key should be an unexported struct. 
This ensures the value in the context doesn't bleed into other packages. 
For example, a context used in logging may set a transaction ID with a key only known to the logging package. 
The logging package is the only one that can access the transaction ID at compile time, preventing abuse.

## Context flows
Think long and hard if you ever write a line that looks like `x = ctx`.  If you still want to write it, think long and hard again.
The benefit of context defining its own scope is lost once you store a context in a variable.
It can be passed alongside mere mortal variables or embedded inside structs. 
When stored, a context can easily find itself alongside an existing context and cause conflicts.

An acceptable exception is serialization to pass via a channel.  Even in this scenario, consider whether this is the correct approach since it opens up opportunities to keep the context stored.
```go
package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

type structWithCtx struct {
	data string
	ctx  context.Context
}

func main() {
	workChan := make(chan structWithCtx)

	go longLivedWorker(workChan)

	for i := 0; i < 10; i++ {
		work := structWithCtx{
			data: fmt.Sprintf("work piece number %d", i),
            // the context gets serialized so that it may flow through the channel
			ctx:  context.WithValue(context.Background(), "key", "value"),
		}
		workChan <- work
	}
}

func longLivedWorker(workChan chan structWithCtx) {

	for work := range workChan {
        // once through the channel, the context is retrieved,
        // then never referenced via struct again
		ctx := work.ctx
		ctx, cancel := context.WithDeadline(ctx, time.Now().Add(100*time.Millisecond))
		defer cancel()
		doWork(ctx, work.data)
	}
}

func doWork(ctx context.Context, data string) {
	select {
	case <-time.After(time.Duration(rand.Intn(150)) * time.Millisecond):
		fmt.Printf("completed work '%s'\n", data)
	case <-ctx.Done():
		fmt.Println("quit")
	}
}
```

## Leveraging Context for Logging
Prelude: I hate logging frameworks.  It's the simplest of all problems: "take this text, put it where I want you to". 
Developers apply all their theoretical computer science might to logging frameworks in the name of composability, simplification, portability, abstraction, etc...
This usually means that users of the library have to wade through oblique abstractions and change their style to match the logging library.

Let's make a logging framework. 

Create a package called `logger`.  It houses accessor methods for the actual logger.
Inside the package, introduce a function called `Get`.  It has a parameter of a context, and returns the actual logger.
The `Get` function will create and decorate a logger with the current context's request ID, then return the logger. 
Now you can trace events, along with any other arbitrary data stored in the context when logging
```go
package logger

type key struct{}
var inst key

func Put(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, inst, id)
}

func Get(ctx context.Context) *log.Logger { // replace with your logging library of choice
	id := ctx.Value(inst)
	if id == "" {
		id = "undefined"
	}
	return log.New(os.Stdout, fmt.Sprintf("id: '%s', ", id), log.LstdFlags)
}
```
Our code would look like this: 
 ```go
func DoSomething(ctx context.Context, e Event) {
	logger.Get(ctx).Info("starting to do something")
	// or:
	log := logger.Get(ctx)
	log.Info("starting to do something")
	time.Sleep(1 * time.Second) // doing something...
	logger.Get(ctx).Info("did something!")
}
 ```

