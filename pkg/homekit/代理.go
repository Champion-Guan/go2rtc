package homekit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/hap/hds"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
)

type ServerProxy interface {
	ServerPair
	AddConn(conn any)
	DelConn(conn any)
}

func ProxyHandler(srv ServerProxy, acc net.Conn) HandlerFunc {
	return func(con net.Conn) error {
		defer con.Close()

		pr := &Proxy{
			con: con.(*hap.Conn),
			acc: acc.(*hap.Conn),
			res: make(chan *http.Response),
		}

		// 配件（如摄像头）=> 控制器（如 iPhone）
		go pr.handleAcc()

		// 控制器 => 配件
		return pr.handleCon(srv)
	}
}

type Proxy struct {
	con *hap.Conn
	acc *hap.Conn
	res chan *http.Response
}

func (p *Proxy) handleCon(srv ServerProxy) error {
	var hdsCharIID uint64

	rd := bufio.NewReader(p.con)
	for {
		req, err := http.ReadRequest(rd)
		if err != nil {
			return err
		}

		var hdsConSalt string

		switch {
		case req.Method == "POST" && req.URL.Path == hap.PathPairings:
			var res *http.Response
			if res, err = handlePairings(req, srv); err != nil {
				return err
			}
			if err = res.Write(p.con); err != nil {
				return err
			}
			continue
		case req.Method == "PUT" && req.URL.Path == hap.PathCharacteristics && hdsCharIID != 0:
			body, _ := io.ReadAll(req.Body)
			var v hap.JSONCharacters
			_ = json.Unmarshal(body, &v)
			for _, char := range v.Value {
				if char.IID == hdsCharIID {
					var hdsReq camera.SetupDataStreamTransportRequest
					_ = tlv8.UnmarshalBase64(char.Value, &hdsReq)
					hdsConSalt = hdsReq.ControllerKeySalt
					break
				}
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		if err = req.Write(p.acc); err != nil {
			return err
		}

		res := <-p.res

		switch {
		case req.Method == "GET" && req.URL.Path == hap.PathAccessories:
			body, _ := io.ReadAll(res.Body)
			var v hap.JSONAccessories
			if err = json.Unmarshal(body, &v); err != nil {
				return err
			}
			for _, acc := range v.Value {
				if char := acc.GetCharacter(camera.TypeSetupDataStreamTransport); char != nil {
					hdsCharIID = char.IID
				}
				break
			}
			res.Body = io.NopCloser(bytes.NewReader(body))

		case hdsConSalt != "":
			body, _ := io.ReadAll(res.Body)
			var v hap.JSONCharacters
			_ = json.Unmarshal(body, &v)
			for i, char := range v.Value {
				if char.IID == hdsCharIID {
					var hdsRes camera.SetupDataStreamTransportResponse
					_ = tlv8.UnmarshalBase64(char.Value, &hdsRes)

					hdsAccSalt := hdsRes.AccessoryKeySalt
					hdsPort := int(hdsRes.TransportTypeSessionParameters.TCPListeningPort)

					// 将 accPort 切换到 conPort
					hdsPort, err = p.listenHDS(srv, hdsPort, hdsConSalt+hdsAccSalt)
					if err != nil {
						return err
					}

					hdsRes.TransportTypeSessionParameters.TCPListeningPort = uint16(hdsPort)
					if v.Value[i].Value, err = tlv8.MarshalBase64(hdsRes); err != nil {
						return err
					}
					body, _ = json.Marshal(v)
					res.ContentLength = int64(len(body))
					break
				}
			}
			res.Body = io.NopCloser(bytes.NewReader(body))
		}

		if err = res.Write(p.con); err != nil {
			return err
		}
	}
}

func (p *Proxy) handleAcc() error {
	rd := bufio.NewReader(p.acc)
	for {
		res, err := hap.ReadResponse(rd, nil)
		if err != nil {
			return err
		}

		if res.Proto == hap.ProtoEvent {
			if err = hap.WriteEvent(p.con, res); err != nil {
				return err
			}
			continue
		}

		// 必须先读取响应体，再读取下一个响应
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}
		res.Body = io.NopCloser(bytes.NewReader(body))

		p.res <- res
	}
}

func (p *Proxy) listenHDS(srv ServerProxy, accPort int, salt string) (int, error) {
	// HDS 的 TCP 端口必须不小于 32768
	ln, err := net.ListenTCP("tcp", nil)
	if err != nil {
		return 0, err
	}

	go func() {
		defer ln.Close()

		_ = ln.SetDeadline(time.Now().Add(30 * time.Second))

		// 原始控制器连接
		conn1, err := ln.Accept()
		if err != nil {
			return
		}

		defer conn1.Close()

		// 安全控制器连接，因为自身是配件，所以 controller=false
		con, err := hds.NewConn(conn1, p.con.SharedKey, salt, false)
		if err != nil {
			return
		}

		srv.AddConn(con)
		defer srv.DelConn(con)

		accIP := p.acc.RemoteAddr().(*net.TCPAddr).IP

		// 原始配件连接
		conn2, err := net.DialTCP("tcp", nil, &net.TCPAddr{IP: accIP, Port: accPort})
		if err != nil {
			return
		}
		defer conn2.Close()

		// 安全配件连接，因为自身是控制器，所以 controller=true
		acc, err := hds.NewConn(conn2, p.acc.SharedKey, salt, true)
		if err != nil {
			return
		}

		go io.Copy(con, acc)
		_, _ = io.Copy(acc, con)
	}()

	conPort := ln.Addr().(*net.TCPAddr).Port
	return conPort, nil
}
