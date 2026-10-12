package flv

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTimeToRTP(t *testing.T) {
	// Reolink 摄像头帧率为 20
	// 视频时间戳每次增加 50 ms，采样率为 90000，RTP 时间戳增加 4500
	// 音频时间戳每次增加 64 ms，采样率为 16000，RTP 时间戳增加 1024
	frameN := 1
	for i := 0; i < 32; i++ {
		// 1000ms/(90000/4500) = 50ms
		require.Equal(t, uint32(frameN*4500), TimeToRTP(uint32(frameN*50), 90000))
		// 1000ms/(16000/1024) = 64ms
		require.Equal(t, uint32(frameN*1024), TimeToRTP(uint32(frameN*64), 16000))
		frameN *= 2
	}
}
