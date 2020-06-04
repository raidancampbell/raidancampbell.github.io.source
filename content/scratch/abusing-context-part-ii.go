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

		// put the two pointers into the iface struct
		idata := [2]uintptr{p1, p2}

		// declare that the iface is a context
		ctx := *(*context.Context)(unsafe.Pointer(&idata))

		// use the context
		fmt.Println(ctx.Value("key"))
	}
}