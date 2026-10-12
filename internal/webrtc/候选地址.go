package webrtc

import (
	"net"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/AlexxIT/go2rtc/pkg/xnet"
	pion "github.com/pion/webrtc/v4"
)

type Address struct {
	host     string
	Port     string
	Network  string
	Priority uint32
}

var stuns []string

func (a *Address) Host() string {
	if a.host == "stun" {
		ip, err := webrtc.GetCachedPublicIP(stuns...)
		if err != nil {
			return ""
		}
		return ip.String()
	}
	return a.host
}

func (a *Address) Marshal() string {
	if host := a.Host(); host != "" {
		return webrtc.CandidateICE(a.Network, host, a.Port, a.Priority)
	}
	return ""
}

var addresses []*Address
var filters webrtc.Filters

func AddCandidate(network, address string) {
	if network == "" {
		AddCandidate("tcp", address)
		AddCandidate("udp", address)
		return
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return
	}

	// 从 1 开始，使手动候选地址优先级低于内置地址
	// 后续候选地址的优先级依次降低
	candidateIndex := 1 + len(addresses)

	priority := webrtc.CandidateHostPriority(network, candidateIndex)
	addresses = append(addresses, &Address{host, port, network, priority})
}

func GetCandidates() (candidates []string) {
	for _, address := range addresses {
		if candidate := address.Marshal(); candidate != "" {
			candidates = append(candidates, candidate)
		}
	}
	return
}

// FilterCandidate 返回 true 表示候选地址通过检查
func FilterCandidate(candidate *pion.ICECandidate) bool {
	if candidate == nil {
		return false
	}

	// 移除类似 Docker 的候选 IP 地址
	if ip := net.ParseIP(candidate.Address); ip != nil && xnet.Docker.Contains(ip) {
		return false
	}

	// 主机候选地址必须在主机列表中
	if candidate.Typ == pion.ICECandidateTypeHost && filters.Candidates != nil {
		if !core.Contains(filters.Candidates, candidate.Address) {
			return false
		}
	}

	if filters.Networks != nil {
		networkType := NetworkType(candidate.Protocol.String(), candidate.Address)
		if !core.Contains(filters.Networks, networkType) {
			return false
		}
	}

	return true
}

// NetworkType 将 TCP/UDP 转换为 tcp4/tcp6/udp4/udp6
func NetworkType(network, host string) string {
	if strings.IndexByte(host, ':') >= 0 {
		return network + "6"
	} else {
		return network + "4"
	}
}

func asyncCandidates(tr *ws.Transport, cons *webrtc.Conn) {
	tr.WithContext(func(ctx map[any]any) {
		if candidates, ok := ctx["candidate"].([]string); ok {
			// 处理此前收到的候选地址
			for _, candidate := range candidates {
				_ = cons.AddCandidate(candidate)
			}

			// 移除已处理的候选地址
			delete(ctx, "candidate")
		}

		// 设置变量以处理后续候选地址
		ctx["webrtc"] = cons
	})

	for _, candidate := range GetCandidates() {
		log.Trace().Str("candidate", candidate).Msg("[webrtc] config")
		tr.Write(&ws.Message{Type: "webrtc/candidate", Value: candidate})
	}
}

func candidateHandler(tr *ws.Transport, msg *ws.Message) error {
	// 在同步函数中处理收到的候选地址
	tr.WithContext(func(ctx map[any]any) {
		candidate := msg.String()
		log.Trace().Str("candidate", candidate).Msg("[webrtc] remote")

		if cons, ok := ctx["webrtc"].(*webrtc.Conn); ok {
			// WebRTC 服务器已初始化时，立即处理候选地址
			_ = cons.AddCandidate(candidate)
		} else {
			// 否则收集候选地址，稍后处理
			list, _ := ctx["candidate"].([]string)
			ctx["candidate"] = append(list, candidate)
		}
	})

	return nil
}
