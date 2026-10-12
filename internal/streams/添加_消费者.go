package streams

import (
	"errors"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (s *Stream) AddConsumer(cons core.Consumer) (err error) {
	// 支持不同消费者同时发起多个待处理请求
	consN := s.pending.Add(1) - 1

	var prodErrors = make([]error, len(s.producers))
	var prodMedias []*core.Media
	var prodStarts []*Producer

	// 步骤一：获取消费者媒体
	consMedias := cons.GetMedias()
	for _, consMedia := range consMedias {
		log.Trace().Msgf("[streams] check cons=%d media=%s", consN, consMedia)

	producers:
		for prodN, prod := range s.producers {
			// 检查递归请求，例如 `camera1: ffmpeg:camera1`
			if info, ok := cons.(core.Info); ok && prod.url == info.GetSource() {
				log.Trace().Msgf("[streams] skip cons=%d prod=%d", consN, prodN)
				continue
			}

			if prodErrors[prodN] != nil {
				log.Trace().Msgf("[streams] skip cons=%d prod=%d", consN, prodN)
				continue
			}

			if err = prod.Dial(); err != nil {
				log.Trace().Err(err).Msgf("[streams] dial cons=%d prod=%d", consN, prodN)
				prodErrors[prodN] = err
				continue
			}

			// 步骤二：获取生产者媒体，尚未获取轨道
			for _, prodMedia := range prod.GetMedias() {
				log.Trace().Msgf("[streams] check cons=%d prod=%d media=%s", consN, prodN, prodMedia)
				prodMedias = append(prodMedias, prodMedia)

				// 步骤三：匹配消费者与生产者编码列表
				prodCodec, consCodec := prodMedia.MatchMedia(consMedia)
				if prodCodec == nil {
					continue
				}

				var track *core.Receiver

				switch prodMedia.Direction {
				case core.DirectionRecvonly:
					log.Trace().Msgf("[streams] match cons=%d <= prod=%d", consN, prodN)

					// 步骤四：从生产者获取只接收轨道
					if track, err = prod.GetTrack(prodMedia, prodCodec); err != nil {
						log.Info().Err(err).Msg("[streams] can't get track")
						prodErrors[prodN] = err
						continue
					}
					// 步骤五：向消费者添加轨道
					if err = cons.AddTrack(consMedia, consCodec, track); err != nil {
						log.Info().Err(err).Msg("[streams] can't add track")
						continue
					}

				case core.DirectionSendonly:
					log.Trace().Msgf("[streams] match cons=%d => prod=%d", consN, prodN)

					// 步骤四：从消费者获取只接收轨道，即回传通道
					if track, err = cons.(core.Producer).GetTrack(consMedia, consCodec); err != nil {
						log.Info().Err(err).Msg("[streams] can't get track")
						continue
					}
					// 步骤五：向生产者添加轨道
					if err = prod.AddTrack(prodMedia, prodCodec, track); err != nil {
						log.Info().Err(err).Msg("[streams] can't add track")
						prodErrors[prodN] = err
						continue
					}
				}

				prodStarts = append(prodStarts, prod)

				if !consMedia.MatchAll() {
					break producers
				}
			}
		}
	}

	// 没有读取者时停止生产者
	if s.pending.Add(-1) == 0 {
		s.stopProducers()
	}

	if len(prodStarts) == 0 {
		return formatError(consMedias, prodMedias, prodErrors)
	}

	s.mu.Lock()
	s.consumers = append(s.consumers, cons)
	s.mu.Unlock()

	// 可能存在重复项，但不影响使用
	for _, prod := range prodStarts {
		prod.start()
	}

	return nil
}

func formatError(consMedias, prodMedias []*core.Media, prodErrors []error) error {
	// 1. 返回非空错误
	var text string

	for _, err := range prodErrors {
		if err != nil {
			text = appendString(text, err.Error())
		}
	}

	if len(text) != 0 {
		return errors.New("streams: " + text)
	}

	// 2. 返回“编码不匹配”错误
	if prodMedias != nil {
		var prod, cons string

		for _, media := range prodMedias {
			if media.Direction == core.DirectionRecvonly {
				for _, codec := range media.Codecs {
					prod = appendString(prod, media.Kind+":"+codec.PrintName())
				}
			}
		}

		for _, media := range consMedias {
			if media.Direction == core.DirectionSendonly {
				for _, codec := range media.Codecs {
					cons = appendString(cons, media.Kind+":"+codec.PrintName())
				}
			}
		}

		return errors.New("streams: codecs not matched: " + prod + " => " + cons)
	}

	// 3. 返回未知错误
	return errors.New("streams: unknown error")
}

func appendString(s, elem string) string {
	if strings.Contains(s, elem) {
		return s
	}
	if len(s) == 0 {
		return elem
	}
	return s + ", " + elem
}
