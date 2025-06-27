package worker

import "github.com/boostgo/errorx"

var ErrLocked = errorx.New("worker.locker.locked")
