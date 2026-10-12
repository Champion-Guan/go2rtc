package hls

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
)

type Session struct {
	cons     core.Consumer
	id       string
	template string
	init     []byte
	buffer   []byte
	seq      int
	alive    *time.Timer
	mu       sync.Mutex
}

func NewSession(cons core.Consumer) *Session {
	s := &Session{
		id:   core.RandString(8, 62),
		cons: cons,
	}

	// Chromecast 需要两个分片
	if _, ok := cons.(*mp4.Consumer); ok {
		s.template = `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:%d
#EXT-X-MAP:URI="init.mp4?id=` + s.id + `"
#EXTINF:0.500,
segment.m4s?id=` + s.id + `&n=%d
#EXTINF:0.500,
segment.m4s?id=` + s.id + `&n=%d`
	} else {
		s.template = `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:%d
#EXTINF:0.500,
segment.ts?id=` + s.id + `&n=%d
#EXTINF:0.500,
segment.ts?id=` + s.id + `&n=%d`
	}

	return s
}

func (s *Session) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	if s.init == nil {
		s.init = p
	} else {
		s.buffer = append(s.buffer, p...)
	}
	s.mu.Unlock()
	return len(p), nil
}

func (s *Session) Run() {
	_, _ = s.cons.(io.WriterTo).WriteTo(s)
}

func (s *Session) Main() []byte {
	type withCodecs interface {
		Codecs() []*core.Codec
	}

	codecs := mp4.MimeCodecs(s.cons.(withCodecs).Codecs())
	codecs = strings.Replace(codecs, mp4.MimeFlac, "fLaC", 1)

	// Safari 需要带宽信息，编码信息有助于流畅播放
	return []byte(`#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=192000,CODECS="` + codecs + `"
hls/playlist.m3u8?id=` + s.id)
}

func (s *Session) Playlist() []byte {
	return []byte(fmt.Sprintf(s.template, s.seq, s.seq, s.seq+1))
}

func (s *Session) Init() (init []byte) {
	for i := 0; i < 60 && init == nil; i++ {
		if i > 0 {
			time.Sleep(50 * time.Millisecond)
		}

		s.mu.Lock()
		// 有缓冲数据时才返回初始化分片
		if len(s.buffer) > 0 {
			init = s.init
		}
		s.mu.Unlock()
	}

	return
}

func (s *Session) Segment() (segment []byte) {
	for i := 0; i < 60 && segment == nil; i++ {
		if i > 0 {
			time.Sleep(50 * time.Millisecond)
		}

		s.mu.Lock()
		if len(s.buffer) > 0 {
			segment = s.buffer
			if _, ok := s.cons.(*mp4.Consumer); ok {
				s.buffer = nil
			} else {
				// TS 新分片需要以初始化数据开始
				s.buffer = s.init
			}
			s.seq++
		}
		s.mu.Unlock()
	}

	return
}
