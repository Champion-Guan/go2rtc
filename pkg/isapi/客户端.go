package isapi

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

// Deprecated: 应重写为 core.Connection
type Client struct {
	core.Listener

	url     string
	channel string
	conn    net.Conn

	medias []*core.Media
	sender *core.Sender
	send   int
}

func Dial(rawURL string) (*Client, error) {
	// 检查地址是否有效
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	u.Scheme = "http"
	u.Path = ""

	client := &Client{url: u.String()}
	if err = client.Dial(); err != nil {
		return nil, err
	}
	return client, err
}

func (c *Client) Dial() (err error) {
	link := c.url + "/ISAPI/System/TwoWayAudio/channels"
	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		return err
	}

	res, err := tcp.Do(req)
	if err != nil {
		return
	}

	if res.StatusCode != http.StatusOK {
		tcp.Close(res)
		return errors.New(res.Status)
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	xml := string(b)

	codec := core.Between(xml, `<audioCompressionType>`, `<`)
	switch codec {
	case "G.711ulaw":
		codec = core.CodecPCMU
	case "G.711alaw":
		codec = core.CodecPCMA
	default:
		return nil
	}

	c.channel = core.Between(xml, `<id>`, `<`)

	media := &core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionSendonly,
		Codecs: []*core.Codec{
			{Name: codec, ClockRate: 8000},
		},
	}
	c.medias = append(c.medias, media)

	return nil
}

func (c *Client) Open() (err error) {
	// 海康威视 ISAPI 可能在上次请求未关闭时拒绝新打开请求，
	// 例如通过摄像头测试按钮或 curl 打开过音频；但即使音频已关闭，
	// 仍可发送关闭请求。因此先关闭，再重新打开。
	// 此方案虽不理想，但能工作。
	if err = c.Close(); err != nil {
		return err
	}

	link := c.url + "/ISAPI/System/TwoWayAudio/channels/" + c.channel
	req, err := http.NewRequest("PUT", link+"/open", nil)
	if err != nil {
		return err
	}

	res, err := tcp.Do(req)
	if err != nil {
		return
	}

	tcp.Close(res)

	ctx, pconn := tcp.WithConn()
	req, err = http.NewRequestWithContext(ctx, "PUT", link+"/audioData", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", "0")

	res, err = tcp.Do(req)
	if err != nil {
		return err
	}

	c.conn = *pconn

	// 阻塞直到连接关闭
	b := make([]byte, 1)
	_, _ = c.conn.Read(b)

	tcp.Close(res)

	return nil
}

func (c *Client) Close() (err error) {
	link := c.url + "/ISAPI/System/TwoWayAudio/channels/" + c.channel
	req, err := http.NewRequest("PUT", link+"/close", nil)
	if err != nil {
		return err
	}

	res, err := tcp.Do(req)
	if err != nil {
		return err
	}

	tcp.Close(res)

	return nil
}

//type XMLChannels struct {
//	Channels []Channel `xml:"TwoWayAudioChannel"`
//}

//type Channel struct {
//	ID      string `xml:"id"`
//	Enabled string `xml:"enabled"`
//	Codec   string `xml:"audioCompressionType"`
//}
