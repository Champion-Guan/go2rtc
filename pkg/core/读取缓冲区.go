package core

import (
	"errors"
	"io"
)

// ProbeSize
// 原作者测试中，40 Mbit/s 的 4K MPEG-TS 视频需要超过 1 MB 的探测缓冲
const ProbeSize = 5 * 1024 * 1024 // 5MB

const (
	BufferDisable       = 0
	BufferDrainAndClear = -1
)

// ReadBuffer 支持缓冲以及缓冲区内定位
// BufferSize 为正时启用缓冲
// 定位到负偏移时清空缓冲
// BufferSize 为正时，定位操作会在读取完缓冲后继续缓冲
// BufferSize 为负时，定位操作会在读取完缓冲后清空缓冲
// 读取超过 BufferSize 会报错
type ReadBuffer struct {
	io.Reader

	BufferSize int

	buf []byte
	pos int
}

func NewReadBuffer(rd io.Reader) *ReadBuffer {
	if rs, ok := rd.(*ReadBuffer); ok {
		return rs
	}
	return &ReadBuffer{Reader: rd}
}

func (r *ReadBuffer) Read(p []byte) (n int, err error) {
	// 缓冲为零时正常读取
	if r.BufferSize == BufferDisable {
		return r.Reader.Read(p)
	}

	// 缓冲非空时从缓冲读取
	if r.pos < len(r.buf) {
		n = copy(p, r.buf[r.pos:])
		r.pos += n
		return
	}

	// 缓冲为负时清空并正常读取
	if r.BufferSize < 0 {
		r.BufferSize = BufferDisable
		r.buf = nil
		r.pos = 0

		return r.Reader.Read(p)
	}

	n, err = r.Reader.Read(p)
	if len(r.buf)+n > r.BufferSize {
		return 0, errors.New("probe reader overflow")
	}
	r.buf = append(r.buf, p[:n]...)
	r.pos += n
	return
}

func (r *ReadBuffer) Close() error {
	if closer, ok := r.Reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (r *ReadBuffer) Seek(offset int64, whence int) (int64, error) {
	var pos int
	switch whence {
	case io.SeekStart:
		pos = int(offset)
	case io.SeekCurrent:
		pos = r.pos + int(offset)
	case io.SeekEnd:
		pos = len(r.buf) + int(offset)
	}

	// 偏移为负时清空缓冲
	if pos < 0 {
		r.buf = nil
		r.pos = 0
	} else if pos >= len(r.buf) {
		r.pos = len(r.buf)
	} else {
		r.pos = pos
	}

	return int64(r.pos), nil
}

func (r *ReadBuffer) Peek(n int) ([]byte, error) {
	r.BufferSize = n
	b := make([]byte, n)
	if _, err := io.ReadAtLeast(r, b, n); err != nil {
		return nil, err
	}
	r.Reset()
	return b, nil
}

func (r *ReadBuffer) Reset() {
	r.BufferSize = BufferDrainAndClear
	r.pos = 0
}
