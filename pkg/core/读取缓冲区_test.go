package core

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadSeeker(t *testing.T) {
	b := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	buf := bytes.NewReader(b)

	rd := NewReadBuffer(buf)
	rd.BufferSize = ProbeSize

	// 1. 读取到缓冲区
	b = make([]byte, 3)
	n, err := rd.Read(b)
	require.Nil(t, err)
	require.Equal(t, []byte{0, 1, 2}, b[:n])

	// 2. 定位到起始位置
	_, err = rd.Seek(0, io.SeekStart)
	require.Nil(t, err)

	// 3. 从缓冲区读取
	b = make([]byte, 2)
	n, err = rd.Read(b)
	require.Nil(t, err)
	require.Equal(t, []byte{0, 1}, b[:n])

	// 4. 从缓冲区读取
	n, err = rd.Read(b)
	require.Nil(t, err)
	require.Equal(t, []byte{2}, b[:n])

	// 5. 读取到缓冲区
	n, err = rd.Read(b)
	require.Nil(t, err)
	require.Equal(t, []byte{3, 4}, b[:n])

	// 6. 定位到起始位置
	_, err = rd.Seek(0, io.SeekStart)
	require.Nil(t, err)

	// 7. 禁用缓冲区
	rd.BufferSize = -1

	// 8. 从缓冲区读取
	b = make([]byte, 10)
	n, err = rd.Read(b)
	require.Nil(t, err)
	require.Equal(t, []byte{0, 1, 2, 3, 4}, b[:n])

	// 9. 直接读取
	n, err = rd.Read(b)
	require.Nil(t, err)
	require.Equal(t, []byte{5, 6, 7, 8, 9}, b[:n])

	// 10. 检查缓冲区为空
	require.Nil(t, rd.buf)
}
