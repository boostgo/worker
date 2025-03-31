# `github.com/boostgo/worker`

# Get started

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/boostgo/worker"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(time.Second * 3)
		cancel()
	}()

	worker.Run(
		ctx,
		"test worker",
		time.Second,
		func(ctx context.Context, stop func()) error {
			fmt.Println("worker action")
			return nil
		},
		true,
	)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			fmt.Println("done")
			return
		}
	}
}

```