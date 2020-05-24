package scratch

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
			ctx:  context.WithValue(context.Background(), "key", "value"),
		}
		workChan <- work
	}
}

func longLivedWorker(workChan chan structWithCtx) {

	for work := range workChan {
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
