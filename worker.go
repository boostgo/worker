package worker

import (
	"context"
	"errors"
	"time"

	"github.com/boostgo/appx"
	"github.com/boostgo/errorx"
	"github.com/boostgo/log"
	"github.com/boostgo/trace"
)

type (
	Action     func(ctx context.Context) error
	Middleware func(ctx context.Context) error
)

// Worker is job/cron based structure.
type Worker struct {
	teardown     func(fn func() error)
	name         string
	fromStart    bool
	duration     time.Duration
	action       Action
	errorHandler func(error) bool
	stopper      chan struct{}
	done         chan struct{}
	timeout      time.Duration
	amIMaster    bool

	beforeMiddlewares []Middleware
	afterMiddlewares  []Middleware
}

// NewWorker creates [Worker] object
func NewWorker(
	name string,
	duration time.Duration,
	action Action,
) *Worker {
	return &Worker{
		teardown:  func(fn func() error) {},
		name:      name,
		duration:  duration,
		action:    action,
		stopper:   make(chan struct{}, 1),
		done:      make(chan struct{}, 1),
		amIMaster: trace.AmIMaster(),

		beforeMiddlewares: []Middleware{},
		afterMiddlewares:  []Middleware{},
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

func (worker *Worker) BeforeMiddlewares(middlewares ...Middleware) *Worker {
	if len(middlewares) == 0 {
		return worker
	}

	worker.beforeMiddlewares = append(worker.beforeMiddlewares, middlewares...)
	return worker
}

func (worker *Worker) AfterMiddlewares(middlewares ...Middleware) *Worker {
	if len(middlewares) == 0 {
		return worker
	}

	worker.afterMiddlewares = append(worker.afterMiddlewares, middlewares...)
	return worker
}

// runAction runs provided action with context and try function and trace id.
func (worker *Worker) runAction() error {
	ctx := context.Background()
	var cancel context.CancelFunc

	if worker.amIMaster {
		ctx = trace.Set(ctx)
	}

	if worker.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, worker.timeout+time.Second)
		defer cancel()
	}

	if err := errorx.TryContext(ctx, func(ctx context.Context) error {
		var locked bool
		for _, middleware := range worker.beforeMiddlewares {
			if locked {
				break
			}

			if err := middleware(ctx); err != nil {
				if errors.Is(err, ErrLocked) {
					locked = true
					continue
				}

				log.
					Error().
					Ctx(ctx).
					Err(err).
					Msg("Worker before middleware")
			}
		}

		defer func() {
			if locked {
				return
			}

			for _, middleware := range worker.afterMiddlewares {
				if err := middleware(ctx); err != nil {
					log.
						Error().
						Ctx(ctx).
						Err(err).
						Msg("Worker after middleware")
				}
			}
		}()

		if locked {
			return nil
		}

		return worker.action(ctx)
	}); err != nil {
		log.
			Namespace(worker.name).
			Error().
			Err(err).
			Msg("Worker action failed")
	}

	return nil
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
			case <-appx.Context().Done():
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
	name string,
	duration time.Duration,
	action Action,
	fromStart ...bool,
) {
	worker := NewWorker(name, duration, action)
	if len(fromStart) > 0 {
		worker.FromStart(fromStart[0])
	}
	worker.Run()
}
