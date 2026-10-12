package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSenser(t *testing.T) {
	recv := make(chan *Packet) // blocking receiver

	sender := NewSender(nil, &Codec{})
	sender.Output = func(packet *Packet) {
		recv <- packet
	}
	require.Equal(t, "new", sender.State())

	sender.Start()
	require.Equal(t, "connected", sender.State())

	sender.Input(&Packet{})
	sender.Input(&Packet{})

	require.Equal(t, 2, sender.Packets)
	require.Equal(t, 0, sender.Drops)

	// 关闭前应先读取一次，
	// 因为 Start 中的协程可能使用空通道；
	// 实际运行中通常无害，但会影响测试
	_, ok := <-recv
	require.True(t, ok)

	sender.Close()
	require.Equal(t, "closed", sender.State())

	sender.Input(&Packet{})

	require.Equal(t, 2, sender.Packets)
	require.Equal(t, 1, sender.Drops)

	// read 2nd
	_, ok = <-recv
	require.True(t, ok)

	// read 3rd
	select {
	case <-recv:
		ok = true
	default:
		ok = false
	}
	require.False(t, ok)
}
