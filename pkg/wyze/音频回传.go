package wyze

import (
	"fmt"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tutk"
	"github.com/pion/rtp"
)

func (p *Producer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	if err := p.client.StartIntercom(); err != nil {
		return fmt.Errorf("wyze: failed to enable intercom: %w", err)
	}

	// 获取摄像头音频编码信息，发送的编码通常也是其接收的编码
	tutkCodec, sampleRate, channels := p.client.GetBackchannelCodec()
	if tutkCodec == 0 {
		return fmt.Errorf("wyze: no audio codec detected from camera")
	}

	if p.client.verbose {
		fmt.Printf("[Wyze] Intercom enabled, using codec=0x%04x rate=%d ch=%d\n", tutkCodec, sampleRate, channels)
	}

	sender := core.NewSender(media, track.Codec)

	// 自行跟踪时间戳，摄像头要求从 0 开始，
	// 按每帧微秒时长递增
	var timestamp uint32 = 0
	samplesPerFrame := tutk.GetSamplesPerFrame(tutkCodec)
	frameDurationUS := samplesPerFrame * 1000000 / sampleRate

	sender.Handler = func(pkt *rtp.Packet) {
		if err := p.client.WriteAudio(tutkCodec, pkt.Payload, timestamp, sampleRate, channels); err == nil {
			p.Send += len(pkt.Payload)
		}
		timestamp += frameDurationUS
	}

	switch track.Codec.Name {
	case core.CodecAAC:
		if track.Codec.IsRTP() {
			sender.Handler = aac.RTPToADTS(codec, sender.Handler)
		} else {
			sender.Handler = aac.EncodeToADTS(codec, sender.Handler)
		}
	}

	sender.HandleRTP(track)
	p.Senders = append(p.Senders, sender)

	return nil
}
