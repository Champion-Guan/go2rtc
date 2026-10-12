package webtorrent

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v4"
)

func NewClient(tracker, share, pwd string, pc *pion.PeerConnection) (*webrtc.Conn, error) {
	// 1. 创建 WebRTC 生产者
	prod := webrtc.NewConn(pc)
	prod.FormatName = "webtorrent"
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "ws"

	medias := []*core.Media{
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
		{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
	}

	// 2. 创建提议
	offer, err := prod.CreateCompleteOffer(medias)
	if err != nil {
		return nil, err
	}

	// 3. 加密提议
	nonce := strconv.FormatInt(time.Now().UnixNano(), 36)

	cipher, err := NewCipher(share, pwd, nonce)
	if err != nil {
		return nil, err
	}

	enc := cipher.Encrypt([]byte(offer))

	// 4. 连接追踪服务器
	ws, _, err := websocket.DefaultDialer.Dial(tracker, nil)
	if err != nil {
		return nil, err
	}

	defer ws.Close()

	// 5. 发送提议
	msg := fmt.Sprintf(
		`{"action":"announce","info_hash":"%s","peer_id":"%s","offers":[{"offer_id":"%s","offer":{"type":"offer","sdp":"%s"}}],"numwant":1}`,
		InfoHash(share), core.RandString(16, 36), nonce, base64.StdEncoding.EncodeToString(enc),
	)
	if err = ws.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		return nil, err
	}

	// 等待完整应答，最多 30 秒
	if err = ws.SetReadDeadline(time.Now().Add(time.Second * 30)); err != nil {
		return nil, err
	}

	for {
		// 6. 读取应答
		var v Message
		if err = ws.ReadJSON(&v); err != nil {
			return nil, err
		}

		if v.Answer == nil {
			continue
		}

		// 7. 解密应答
		enc, err = base64.StdEncoding.DecodeString(v.Answer.SDP)
		if err != nil {
			return nil, err
		}

		answer, err := cipher.Decrypt(enc)
		if err != nil {
			return nil, err
		}

		// 8. 设置应答
		if err = prod.SetAnswer(string(answer)); err != nil {
			return nil, err
		}

		return prod, nil
	}
}
