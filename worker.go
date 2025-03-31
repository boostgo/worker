package worker

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Action func(ctx context.Context, stop func()) error

// Worker is job/cron based structure.
type Worker struct {
	ctx      context.Context
	teardown func(fn func() error)

	name         string
	fromStart    bool
	duration     time.Duration
	action       Action
	errorHandler func(error) bool
	stopper      chan struct{}
	done         chan struct{}
	timeout      time.Duration
}

// New creates [Worker] object
func New(
	ctx context.Context,
	name string,
	duration time.Duration,
	action Action,
) *Worker {
	return &Worker{
		ctx:      ctx,
		teardown: func(fn func() error) {},

		name:     name,
		duration: duration,
		action:   action,
		stopper:  make(chan struct{}, 1),
		done:     make(chan struct{}, 1),
	}
}

// FromStart sets flag for starting worker from start.
func (worker *Worker) FromStart(fromStart bool) *Worker {
	worker.fromStart = fromStart
	return worker
}

// Teardown set teardown function
func (worker *Worker) Teardown(teardown func(fn func() error)) *Worker {
	worker.teardown = teardown
	return worker
}

// Timeout sets timeout duration for working action timeout.
func (worker *Worker) Timeout(timeout time.Duration) *Worker {
	worker.timeout = timeout
	return worker
}

// ErrorHandler sets custom error handler from action
func (worker *Worker) ErrorHandler(handler func(error) bool) *Worker {
	if handler == nil {
		return worker
	}

	worker.errorHandler = handler
	return worker
}

// runAction runs provided action with context and try function and trace id.
func (worker *Worker) runAction() error {
	ctx := context.Background()
	var cancel context.CancelFunc

	if worker.duration > 0 {
		ctx, cancel = context.WithTimeout(ctx, worker.duration)
		defer cancel()
	}

	return try(ctx, func(ctx context.Context) error {
		return worker.action(ctx, func() {
			worker.stopper <- struct{}{}
		})
	})
}

// Run runs worker with provided duration
func (worker *Worker) Run() {
	if worker.fromStart {
		if err := worker.runAction(); err != nil {
			if worker.errorHandler != nil {
				if !worker.errorHandler(err) {
					worker.stopper <- struct{}{}
				}
			}
		}
	}

	go func() {
		ticker := time.NewTicker(worker.duration)
		defer ticker.Stop()

		worker.teardown(func() error {
			// teardown will make main goroutine wait till worker will not be done
			<-worker.done
			return nil
		})

		for {
			select {
			case <-worker.ctx.Done():
				worker.done <- struct{}{}
				return
			case <-worker.stopper:
				worker.done <- struct{}{}
				return
			case <-ticker.C:
				if err := worker.runAction(); err != nil {
					if worker.errorHandler != nil {
						if !worker.errorHandler(err) {
							worker.stopper <- struct{}{}
							continue
						}
					}
				}
			}
		}
	}()
}

// Run created worker object and runs by itself. It is like "short" version of using [Worker]
func Run(
	ctx context.Context,
	name string,
	duration time.Duration,
	action Action,
	fromStart ...bool,
) {
	worker := New(ctx, name, duration, action)
	if len(fromStart) > 0 {
		worker.FromStart(fromStart[0])
	}
	worker.Run()
}

func try(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	defer func() {
		if err == nil {
			err = errors.New(fmt.Sprintf("%v", recover()))
		}
	}()

	return fn(ctx)
}
