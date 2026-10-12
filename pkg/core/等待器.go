package core

import (
	"sync"
)

// Waiter 支持：
// - 首次 Wait 时自动启动
// - 最后一次 Done 后阻止新的等待者
// - 完成后可以安全调用 Done
type Waiter struct {
	sync.WaitGroup
	mu    sync.Mutex
	state int // state < 0 means finish
	err   error
}

func (w *Waiter) Add(delta int) {
	w.mu.Lock()
	if w.state >= 0 {
		w.state += delta
		w.WaitGroup.Add(delta)
	}
	w.mu.Unlock()
}

func (w *Waiter) Wait() error {
	w.mu.Lock()
	// 首次等待自动启动等待器
	if w.state == 0 {
		w.state++
		w.WaitGroup.Add(1)
	}
	w.mu.Unlock()

	w.WaitGroup.Wait()

	return w.err
}

func (w *Waiter) Done(err error) {
	w.mu.Lock()

	// 仅存在任务时调用 Done
	if w.state > 0 {
		w.state--
		w.WaitGroup.Done()
	}

	// 最后一次完成后阻止等待器继续接受操作
	if w.state == 0 {
		w.state = -1
		w.err = err
	}

	w.mu.Unlock()
}

func (w *Waiter) WaitChan() <-chan error {
	var ch chan error

	w.mu.Lock()

	if w.state >= 0 {
		ch = make(chan error)
		go func() {
			ch <- w.Wait()
		}()
	}

	w.mu.Unlock()

	return ch
}
