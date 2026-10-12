package tcp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// Do 是支持摘要身份验证的 http.Client
func Do(req *http.Request) (*http.Response, error) {
	var secure *tls.Config

	switch req.URL.Scheme {
	case "httpx":
		secure = insecureConfig
		req.URL.Scheme = "https"
	case "https":
		if hostname := req.URL.Hostname(); IsIP(hostname) {
			secure = insecureConfig
		}
	}

	if secure != nil {
		ctx := context.WithValue(req.Context(), secureKey, secure)
		req = req.WithContext(ctx)
	}

	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()

		dial := transport.DialContext
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dial(ctx, network, addr)
			if pconn, ok := ctx.Value(connKey).(*net.Conn); ok {
				*pconn = conn
			}
			return conn, err
		}
		transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dial(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			var conf *tls.Config
			if v, ok := ctx.Value(secureKey).(*tls.Config); ok {
				conf = v
			} else if host, _, err := net.SplitHostPort(addr); err != nil {
				conf = &tls.Config{ServerName: addr}
			} else {
				conf = &tls.Config{ServerName: host}
			}

			tlsConn := tls.Client(conn, conf)
			if err = tlsConn.Handshake(); err != nil {
				return nil, err
			}

			if pconn, ok := ctx.Value(connKey).(*net.Conn); ok {
				*pconn = tlsConn
			}
			return tlsConn, err
		}

		client = &http.Client{Transport: transport}
	}

	user := req.URL.User

	// 海康威视对带响应头的基本身份验证可能不响应
	if strings.HasPrefix(req.URL.Path, "/ISAPI/") {
		req.URL.User = nil
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode == http.StatusUnauthorized && user != nil {
		Close(res)

		auth := res.Header.Get("WWW-Authenticate")
		if !strings.HasPrefix(auth, "Digest") {
			return nil, errors.New("unsupported auth: " + auth)
		}

		realm := Between(auth, `realm="`, `"`)
		nonce := Between(auth, `nonce="`, `"`)
		qop := Between(auth, `qop="`, `"`)

		username := user.Username()
		password, _ := user.Password()
		ha1 := HexMD5(username, realm, password)

		uri := req.URL.RequestURI()
		ha2 := HexMD5(req.Method, uri)

		var header string

		switch qop {
		case "":
			response := HexMD5(ha1, nonce, ha2)
			header = fmt.Sprintf(
				`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
				username, realm, nonce, uri, response,
			)
		case "auth":
			nc := "00000001"
			cnonce := core.RandString(32, 64)
			response := HexMD5(ha1, nonce, nc, cnonce, qop, ha2)
			header = fmt.Sprintf(
				`Digest username="%s", realm="%s", nonce="%s", uri="%s", qop=%s, nc=%s, cnonce="%s", response="%s"`,
				username, realm, nonce, uri, qop, nc, cnonce, response,
			)
		default:
			return nil, errors.New("unsupported qop: " + auth)
		}

		req.Header.Set("Authorization", header)

		if res, err = client.Do(req); err != nil {
			return nil, err
		}
	}

	return res, nil
}

var client *http.Client

type key string

var connKey = key("conn")
var secureKey = key("secure")

var insecureConfig = &tls.Config{
	InsecureSkipVerify: true,
	CipherSuites: []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA, tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA, tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,

		// 这些密码套件从 Go 1.22 起被禁用：https://tip.golang.org/doc/go1.22
		// 但部分摄像头依赖它们：https://github.com/AlexxIT/go2rtc/issues/1172
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256, // insecure
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384, // insecure
	},
}

func WithConn() (context.Context, *net.Conn) {
	pconn := new(net.Conn)
	return context.WithValue(context.Background(), connKey, pconn), pconn
}

func Close(res *http.Response) {
	if res.Body != nil {
		_ = res.Body.Close()
	}
}

func IsIP(hostname string) bool {
	return net.ParseIP(hostname) != nil
}
