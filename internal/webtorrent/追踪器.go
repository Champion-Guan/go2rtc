package webtorrent

import (
	"fmt"
	"net/http"

	"github.com/AlexxIT/go2rtc/pkg/webtorrent"
	"github.com/gorilla/websocket"
)

var upgrader *websocket.Upgrader
var hashes map[string]map[string]*websocket.Conn

func tracker(w http.ResponseWriter, r *http.Request) {
	if upgrader == nil {
		upgrader = &websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 2028,
		}
		upgrader.CheckOrigin = func(r *http.Request) bool {
			return true
		}
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Warn().Err(err).Send()
		return
	}

	defer ws.Close()

	for {
		var msg webtorrent.Message
		if err = ws.ReadJSON(&msg); err != nil {
			return
		}

		//log.Trace().Msgf("[webtorrent] message=%v", msg)

		if msg.InfoHash == "" || msg.PeerId == "" {
			continue
		}

		if hashes == nil {
			hashes = map[string]map[string]*websocket.Conn{}
		}

		// 带连接提议的新旧客户端
		clients := hashes[msg.InfoHash]
		if clients == nil {
			clients = map[string]*websocket.Conn{
				msg.PeerId: ws,
			}
			hashes[msg.InfoHash] = clients
		} else {
			clients[msg.PeerId] = ws
		}

		switch {
		case msg.Offers != nil:
			// 请求心跳
			raw := fmt.Sprintf(
				`{"action":"announce","interval":120,"info_hash":"%s","complete":0,"incomplete":1}`,
				msg.InfoHash,
			)
			if err = ws.WriteMessage(websocket.TextMessage, []byte(raw)); err != nil {
				log.Warn().Err(err).Send()
				return
			}

			// 没有连接提议时跳过，适用于服务器
			if len(msg.Offers) == 0 {
				continue
			}

			// 仅获取并校验第一个连接提议
			offer := msg.Offers[0]
			if offer.OfferId == "" || offer.Offer.Type != "offer" || offer.Offer.SDP == "" {
				continue
			}

			// 向全部客户端发送连接提议，其中一个是服务器
			raw = fmt.Sprintf(
				`{"action":"announce","info_hash":"%s","peer_id":"%s","offer_id":"%s","offer":{"type":"offer","sdp":"%s"}}`,
				msg.InfoHash, msg.PeerId, offer.OfferId, offer.Offer.SDP,
			)

			for _, server := range clients {
				if server != ws {
					_ = server.WriteMessage(websocket.TextMessage, []byte(raw))
				}
			}

		case msg.OfferId != "" && msg.ToPeerId != "" && msg.Answer != nil:
			ws1, ok := clients[msg.ToPeerId]
			if !ok {
				continue
			}

			raw := fmt.Sprintf(
				`{"action":"announce","info_hash":"%s","peer_id":"%s","offer_id":"%s","answer":{"type":"answer","sdp":"%s"}}`,
				msg.InfoHash, msg.PeerId, msg.OfferId, msg.Answer.SDP,
			)
			_ = ws1.WriteMessage(websocket.TextMessage, []byte(raw))
		}
	}
}
