package ffmpeg

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Producer struct {
	core.Connection
	url    string
	query  url.Values
	ffmpeg core.Producer
}

// NewProducer 根据客户端能力自动选择视频和音频编码
func NewProducer(url string) (core.Producer, error) {
	p := &Producer{}

	i := strings.IndexByte(url, '#')
	p.url, p.query = url[:i], streams.ParseQuery(url[i+1:])

	// ffmpeg.NewProducer 仅支持一路音频
	if len(p.query["video"]) != 0 || len(p.query["audio"]) != 1 {
		return nil, errors.New("ffmpeg: unsupported params: " + url[i:])
	}

	p.ID = core.NewID()
	p.FormatName = "ffmpeg"
	p.Medias = []*core.Media{
		{
			// 只能支持音频，因为未知 H264 的 FmtpLine 和 MJPEG 的 PayloadType
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			// 编码从优到劣排列
			Codecs: []*core.Codec{
				// OPUS 始终标记为 OPUS/48000/2
				{Name: core.CodecOpus, ClockRate: 48000, Channels: 2},
				{Name: core.CodecPCML, ClockRate: 16000},
				{Name: core.CodecPCM, ClockRate: 16000},
				{Name: core.CodecPCMA, ClockRate: 16000},
				{Name: core.CodecPCMU, ClockRate: 16000},
				{Name: core.CodecPCML, ClockRate: 8000},
				{Name: core.CodecPCM, ClockRate: 8000},
				{Name: core.CodecPCMA, ClockRate: 8000},
				{Name: core.CodecPCMU, ClockRate: 8000},
				// AAC 在大华双向对讲中存在原因未知的问题
				{Name: core.CodecAAC, ClockRate: 16000, FmtpLine: aac.FMTP + "1408"},
			},
		},
	}
	return p, nil
}

func (p *Producer) Start() error {
	var err error
	if p.ffmpeg, err = streams.GetProducer(p.newURL()); err != nil {
		return err
	}

	for i, media := range p.ffmpeg.GetMedias() {
		track, err := p.ffmpeg.GetTrack(media, media.Codecs[0])
		if err != nil {
			return err
		}
		p.Receivers[i].Replace(track)
	}

	return p.ffmpeg.Start()
}

func (p *Producer) Stop() error {
	if p.ffmpeg == nil {
		return nil
	}
	return p.ffmpeg.Stop()
}

func (p *Producer) MarshalJSON() ([]byte, error) {
	if p.ffmpeg == nil {
		return json.Marshal(p.Connection)
	}
	return json.Marshal(p.ffmpeg)
}

func (p *Producer) newURL() string {
	s := p.url
	// 将地址中的 auto 编码改为默认配置中的已知预设
	for _, receiver := range p.Receivers {
		codec := receiver.Codec
		switch codec.Name {
		case core.CodecOpus:
			s += "#audio=opus/16000"
		case core.CodecAAC:
			s += "#audio=aac/16000"
		case core.CodecPCML:
			s += "#audio=pcml/" + strconv.Itoa(int(codec.ClockRate))
		case core.CodecPCM:
			s += "#audio=pcm/" + strconv.Itoa(int(codec.ClockRate))
		case core.CodecPCMA:
			s += "#audio=pcma/" + strconv.Itoa(int(codec.ClockRate))
		case core.CodecPCMU:
			s += "#audio=pcmu/" + strconv.Itoa(int(codec.ClockRate))
		}
	}
	// 添加其他参数
	for key, values := range p.query {
		if key != "audio" {
			for _, value := range values {
				s += "#" + key + "=" + value
			}
		}
	}

	return s
}
