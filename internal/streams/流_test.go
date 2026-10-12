package streams

import (
	"net/url"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestRecursion(t *testing.T) {
	// 使用指定源创建流
	stream1, err := New("from_yaml", "does_not_matter")
	require.NoError(t, err)
	require.Len(t, streams, 1)

	// 请求指向 go2rtc 的另一个未命名流
	query, err := url.ParseQuery("src=rtsp://localhost:8554/from_yaml?video")
	require.NoError(t, err)
	stream2, err := GetOrPatch(query)
	require.NoError(t, err)

	// 检查是否为同一个流
	require.Equal(t, stream1, stream2)
	// 检查流地址是否相同
	require.Equal(t, stream1.producers[0].url, stream2.producers[0].url)
	require.Len(t, streams, 2)
}

func TestTempate(t *testing.T) {
	HandleFunc("rtsp", func(url string) (core.Producer, error) { return nil, nil }) // bypass HasProducer

	// 来自 YAML 的配置
	stream1, err := New("camera.from_hass", "ffmpeg:{input}#video=copy")
	require.NoError(t, err)
	// 来自 Home Assistant 的请求
	stream2, err := Patch("camera.from_hass", "rtsp://example.com")
	require.NoError(t, err)

	require.Equal(t, stream1, stream2)
	require.Equal(t, "ffmpeg:rtsp://example.com#video=copy", stream1.producers[0].url)
}
