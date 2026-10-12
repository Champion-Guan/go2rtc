package core

import (
	"bytes"
	"io"
	"net/http"
	"sync"
)

// WriteBuffer 默认向 bytes.Buffer 写入，
// 调用 WriteTo 指定新的 io.Writer 后，会执行 Reset。
// Reset 将当前缓冲写入新写入器，并切换后续写入目标。
// WriteTo 会阻塞，直到写入失败或调用 Close。
type WriteBuffer struct {
	io.Writer
	err   error
	mu    sync.Mutex
	wg    sync.WaitGroup
	state byte
}

func NewWriteBuffer(wr io.Writer) *WriteBuffer {
	if wr == nil {
		wr = bytes.NewBuffer(nil)
	}
	return &WriteBuffer{Writer: wr}
}

func (w *WriteBuffer) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	if w.err != nil {
		err = w.err
	} else if n, err = w.Writer.Write(p); err != nil {
		w.err = err
		w.done()
	} else if f, ok := w.Writer.(http.Flusher); ok {
		f.Flush()
	}
	w.mu.Unlock()
	return
}

func (w *WriteBuffer) WriteTo(wr io.Writer) (n int64, err error) {
	w.Reset(wr)
	w.wg.Wait()
	return 0, w.err // TODO: fix counter
}

func (w *WriteBuffer) Close() error {
	if closer, ok := w.Writer.(io.Closer); ok {
		return closer.Close()
	}
	w.mu.Lock()
	w.done()
	w.mu.Unlock()
	return nil
}

func (w *WriteBuffer) Reset(wr io.Writer) {
	w.mu.Lock()
	w.add()
	if buf, ok := w.Writer.(*bytes.Buffer); ok && buf.Len() != 0 {
		if _, err := io.Copy(wr, buf); err != nil {
			w.err = err
			w.done()
		}
	}
	w.Writer = wr
	w.mu.Unlock()
}

const (
	none = iota
	start
	end
)

func (w *WriteBuffer) add() {
	if w.state == none {
		w.state = start
		w.wg.Add(1)
	}
}

func (w *WriteBuffer) done() {
	if w.state == start {
		w.state = end
		w.wg.Done()
	}
}

// OnceBuffer 仅捕获第一条消息
type OnceBuffer struct {
	buf []byte
}

func (o *OnceBuffer) Write(p []byte) (n int, err error) {
	if o.buf == nil {
		o.buf = p
	}
	return 0, io.EOF
}

func (o *OnceBuffer) WriteTo(w io.Writer) (n int64, err error) {
	return io.Copy(w, bytes.NewReader(o.buf))
}

func (o *OnceBuffer) Buffer() []byte {
	return o.buf
}

func (o *OnceBuffer) Len() int {
	return len(o.buf)
}
