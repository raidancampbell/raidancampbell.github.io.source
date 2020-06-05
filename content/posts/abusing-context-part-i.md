---
title: "Abusing Context in Go Part I: Serialization"
date: 2020-05-25T10:29:46-07:00
draft: false
summary: A method for serializing context to allow it to flow between networked hosts
---

At the extreme end of microservice architecture, network communication becomes the substitution for function calls.
This change forces many compromises in Go, mostly because everything sent across the network must be able to be serialized and deserialized.
Luckily, the Go standard library includes an [`encoding` package][encoding package], which handles most serialization use cases.

For most use cases encoding data via JSON will work. JSON is a rock-solid "good enough" and ubiquitous solution. 
For less textbook scenarios, Go provides the convenient [`gob` encoder][gob source] to encode/decode structs. 
Gob's easy API comes with a few caveats: functions can't be serialized and unexported values must be ["registered"][gob register].

These limitations add up to a death notice for Go's [Context][go context package] library. 
Context is an interface which contains 4 unexported implementations under the hood: [deadline][timer ctx], [cancel][cancel ctx], [value][value ctx], and [empty][empty ctx]. 
For more details on these implementations see an earlier post, [Design Considerations for Context in Go](/posts/context-in-go/). 
The constraints essentially boil down to serializing the `cancel()` function, and serializing/deserializing unexported values.

In this thought exercise of an "extreme microservice architecture", how would we allow context to flow through the network? 
Context is too complicated to store in JSON, but the cancellation functions and `valueCtx` keys make `gob` a poor choice as well. 
The best answer I came up with was to use `gob`, but manage the serialization/deserialization manually. 
It's worth mentioning that this solution is Not Safe For Production, but is interesting nonetheless.

Using `gob` solves all the serialization issues, but leaves us with two problems: `gob` can't serialize functions, and `gob` can't serialize what it can't see.

Context, at its core, can contain 3 things: a time (for `timerCtx`), a cancellation function (for `timerCtx` or `cancelCtx`), and "anything" (for `valueCtx`). 
We can get a lot further in our journey if we accept that the cancellation functions aren't special and can be recreated during deserialization. 
The next compromise is rougher: we won't allow functions in the `valueCtx`, meaning neither the key nor the value can be a `func`. 
It's not the worst compromise --I've never seen functions stored in the context-- but it's still a limitation.

With these limitations we can now serialize and deserialize `timerCtx` and `cancelCtx` (`emptyCtx` is trivial). 
The flow would be: 
 1. Receive an incoming context: use reflection to determine the concrete type
 1. If the type is `cancelCtx`, remember that we'll have to create a cancellation function during deserialization
 1. If the context is `timerCtx`, reach in and grab the `deadline` time.
 1. If we already have a `deadline` (remember, context is composed), choose the earliest of the existing and current
 1. Remember the `deadline` for deserialization
 1. If the context has a parent context, use it to recurse upwards in the context stack
 1. Create our own struct to house the deadline time and cancellation func.  This is what actually gets serialized.
 1. Serialize this struct across the wire, and receive it on the other side
 1. At deserialization, we know we need to create a `timerCtx` and cancellation function, so we create them

Or, in code:
```go
var (
	cancCtxTyp reflect.Type
	timeCtxTyp reflect.Type
)
func init() {
	// these are used as constants for type comparison.
	// unfortunately we need to access them via reflection, so an init function is required
	cancCtx, c := context.WithCancel(context.Background())
	c() // immediately cancel to prevent resource leaks
	cancCtxTyp = reflect.ValueOf(cancCtx).Elem().Type()
	timeCtx, c := context.WithDeadline(context.Background(), time.Time{})
	c()
	timeCtxTyp = reflect.ValueOf(timeCtx).Elem().Type()
}

func buildMap(ctx context.Context, s contextData) contextData {
	rs := reflect.ValueOf(ctx).Elem()
	if rs.Type() == reflect.ValueOf(context.Background()).Elem().Type() {
		// base case: if the current context is an emptyCtx, we're done.
		return s
	}
	rf := rs.FieldByName("key")
	if rf.IsValid() { // if there's a key, it's a valueCtx
		// TODO
	} else { // it's either a cancelCtx or timerCtx
		if rs.Type() == cancCtxTyp {
			s.HasCancel = true
		}
		if rs.Type() == timeCtxTyp {
			// if there's multiple deadlines in a context, choose the earliest
			deadline := rs.FieldByName("deadline")
			deadline = reflect.NewAt(deadline.Type(), unsafe.Pointer(deadline.UnsafeAddr())).Elem()
			deadlineTime := deadline.Convert(reflect.TypeOf(time.Time{})).Interface().(time.Time)
			if s.HasDeadline && deadlineTime.Before(s.Deadline) {
				s.Deadline = deadlineTime
			} else {
				s.HasDeadline = true
				s.Deadline = deadlineTime
			}
		}
	}
	parent := rs.FieldByName("Context")
	if parent.IsValid() && !parent.IsNil() {
		// if there's a parent context, recurse
		return buildMap(parent.Interface().(context.Context), s)
	}
	// not possible, but the compiler requires it.
	// the parent context would be empty, and is caught in the beginning
	return s
}
```
 
There's a couple of limitations that should be immediately obvious:
 - Time is approximate at best between any two computers
 - Context is flattened during serialization, nothing pre-serialization can be popped off post-serialization

For the purposes of this exercise, I'm calling these acceptable.

At this point we can move deadline and cancellation contexts across the network, but they're easy. 
Moving a value context across the network is more difficult: we need to serialize unknown data. Luckily, `gob` handles this!

This brings us to the final limitation of this solution: unexported struct types can't be passed through with `gob`. 
More accurately they *can* be serialized, but deserialization will fail. 
`gob` provides a [register function][gob register] to "register" an arbitrary type for serialization. 
We can get `gob` any unexported data, and `gob` will happily serialize it.  The problem comes during deserialization. 
At deserialization, `gob` would need the same types registered, but I know of no way to transmit the type information. 
Even if we could transmit it, the receiving-side would need to have the type imported somewhere at compile-time.

The above limitation isn't as severe as it may sound.  It limits scenarios like:
```go
type key struct{}
var k key{}
func createCtx(val string) context.Context {
	return context.WithValue(context.Background(), k, val)
}
```
since the information about the `key` type isn't known to the receiving side. 
Having a known set of context types (in something like a shared library) would solve this: each type just needs to be registered with `gob`. 
The workaround for this, however, is to keep context keys as unexported, but their types should be primitive.
```go
var k = "I'm the unique key"
func createCtx(val string) context.Context {
	return context.WithValue(context.Background(), k, val)
}
``` 
This same type limitation extends to the value: unexported types won't work.

Using all this, we can fill out the `TODO` portion of the above example and arrive at the below
```go
var (
	cancCtxTyp reflect.Type
	timeCtxTyp reflect.Type
)
func init() {
	// these are used as constants for type comparison.
	// unfortunately we need to access them via reflection, so an init function is required
	cancCtx, c := context.WithCancel(context.Background())
	c() // immediately cancel to prevent resource leaks
	cancCtxTyp = reflect.ValueOf(cancCtx).Elem().Type()
	timeCtx, c := context.WithDeadline(context.Background(), time.Time{})
	c()
	timeCtxTyp = reflect.ValueOf(timeCtx).Elem().Type()
}

func buildMap(ctx context.Context, s contextData) contextData {
	rs := reflect.ValueOf(ctx).Elem()
	if rs.Type() == reflect.ValueOf(context.Background()).Elem().Type() {
		// base case: if the current context is an emptyCtx, we're done.
		return s
	}
	rf := rs.FieldByName("key")
	if rf.IsValid() { // if there's a key, it's a valueCtx
		// make the key field read+write
		rf = reflect.NewAt(rf.Type(), unsafe.Pointer(rf.UnsafeAddr())).Elem()

		if rf.CanInterface() { // panic-protection
			key := rf.Interface()

			// grab the val field, make it read+write
			rv := rs.FieldByName("val")
			rv = reflect.NewAt(rv.Type(), unsafe.Pointer(rv.UnsafeAddr())).Elem()
			if rv.CanInterface() {
				val := rv.Interface()
				// only add the key if it doesn't exist.  nested contexts can have the same keys
				// but the concept is lost after serialization: you can't drop things off the stack
				// to the same layer as pre-serialization
				// we're recursing up the stack, so the first instance of the key is the one we want
				if _, exists := s.Values[key]; !exists {
					s.Values[key] = val
					// register them for serialization
					gob.Register(key)
					gob.Register(val)
				}
			}
		}
	} else { // it's either a cancelCtx or timerCtx
		if rs.Type() == cancCtxTyp {
			s.HasCancel = true
		}
		if rs.Type() == timeCtxTyp {
			// if there's multiple deadlines in a context, choose the earliest
			deadline := rs.FieldByName("deadline")
			deadline = reflect.NewAt(deadline.Type(), unsafe.Pointer(deadline.UnsafeAddr())).Elem()
			deadlineTime := deadline.Convert(reflect.TypeOf(time.Time{})).Interface().(time.Time)
			if s.HasDeadline && deadlineTime.Before(s.Deadline) {
				s.Deadline = deadlineTime
			} else {
				s.HasDeadline = true
				s.Deadline = deadlineTime
			}
		}
	}
	parent := rs.FieldByName("Context")
	if parent.IsValid() && !parent.IsNil() {
		// if there's a parent context, recurse
		return buildMap(parent.Interface().(context.Context), s)
	}
	// not possible, but the compiler requires it.
	// the parent context would be empty, and is caught in the beginning
	return s
}
```
The code is [available in `libraidan`][libraidan] as [`SerializeCtx`][SerializeCtx docs]

[encoding package]:https://golang.org/pkg/encoding/
[gob source]:https://golang.org/pkg/encoding/gob/
[gob register]:https://golang.org/pkg/encoding/gob/#Register
[go context package]:https://golang.org/pkg/context/
[timer ctx]:https://golang.org/src/context/context.go#L427
[cancel ctx]:https://golang.org/src/context/context.go#L232
[value ctx]:https://golang.org/src/context/context.go#L513
[empty ctx]:https://golang.org/src/context/context.go#L171
[gob register]:https://golang.org/pkg/encoding/gob/#Register
[libraidan]:https://github.com/raidancampbell/libraidan/
[SerializeCtx docs]:https://godoc.org/github.com/raidancampbell/libraidan/pkg/rruntime#SerializeCtx
