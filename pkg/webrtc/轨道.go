package webrtc

import (
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type Track struct {
	kind     string
	id       string
	streamID string
	sequence uint16
	ssrc     uint32
	writer   webrtc.TrackLocalWriter
	mu       sync.Mutex
}

func NewTrack(kind string) *Track {
	return &Track{
		kind:     kind,
		id:       "go2rtc-" + kind,
		streamID: "go2rtc",
	}
}

func (t *Track) Bind(context webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	t.mu.Lock()
	t.ssrc = uint32(context.SSRC())
	t.writer = context.WriteStream()
	t.mu.Unlock()

	for _, parameters := range context.CodecParameters() {
		// 返回首组参数
		return parameters, nil
	}

	return webrtc.RTPCodecParameters{}, nil
}

func (t *Track) Unbind(context webrtc.TrackLocalContext) error {
	t.mu.Lock()
	t.writer = nil
	t.mu.Unlock()
	return nil
}

func (t *Track) ID() string {
	return t.id
}

func (t *Track) RID() string {
	return "" // don't know what it is
}

func (t *Track) StreamID() string {
	return t.streamID
}

func (t *Track) Kind() webrtc.RTPCodecType {
	return webrtc.NewRTPCodecType(t.kind)
}

func (t *Track) WriteRTP(payloadType uint8, packet *rtp.Packet) (err error) {
	// 因 Unbind 使用互斥锁：https://github.com/AlexxIT/go2rtc/issues/994
	t.mu.Lock()

	// 在 Track.Bind 前启动 WriteRTP 的情况
	if t.writer != nil {
		// 输入包来自不同源时，需要内部计数器
		t.sequence++

		header := packet.Header
		header.SSRC = t.ssrc
		header.PayloadType = payloadType
		header.SequenceNumber = t.sequence
		_, err = t.writer.WriteRTP(&header, packet.Payload)
	}

	t.mu.Unlock()
	return
}
