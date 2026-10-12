package homekit

import (
	"errors"
	"net/http"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/srtp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/homekit"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod map[string]struct {
			Pin           string   `yaml:"pin"`
			Name          string   `yaml:"name"`
			DeviceID      string   `yaml:"device_id"`
			DevicePrivate string   `yaml:"device_private"`
			CategoryID    string   `yaml:"category_id"`
			Pairings      []string `yaml:"pairings"`
		} `yaml:"homekit"`
	}
	app.LoadConfig(&cfg)

	log = app.GetLogger("homekit")

	streams.HandleFunc("homekit", streamHandler)

	api.HandleFunc("api/homekit", apiHomekit)
	api.HandleFunc("api/homekit/accessories", apiHomekitAccessories)
	api.HandleFunc("api/discovery/homekit", apiDiscovery)

	if cfg.Mod == nil {
		return
	}

	hosts = map[string]*server{}
	servers = map[string]*server{}
	var entries []*mdns.ServiceEntry

	for id, conf := range cfg.Mod {
		stream := streams.Get(id)
		if stream == nil {
			log.Warn().Msgf("[homekit] missing stream: %s", id)
			continue
		}

		if conf.Pin == "" {
			conf.Pin = "19550224" // default PIN
		}

		pin, err := hap.SanitizePin(conf.Pin)
		if err != nil {
			log.Error().Err(err).Caller().Send()
			continue
		}

		deviceID := calcDeviceID(conf.DeviceID, id) // random MAC-address
		name := calcName(conf.Name, deviceID)
		setupID := calcSetupID(id)

		srv := &server{
			stream:   id,
			pairings: conf.Pairings,
			setupID:  setupID,
		}

		srv.hap = &hap.Server{
			Pin:             pin,
			DeviceID:        deviceID,
			DevicePrivate:   calcDevicePrivate(conf.DevicePrivate, id),
			GetClientPublic: srv.GetPair,
		}

		srv.mdns = &mdns.ServiceEntry{
			Name: name,
			Port: uint16(api.Port),
			Info: map[string]string{
				hap.TXTConfigNumber: "1",
				hap.TXTFeatureFlags: "0",
				hap.TXTDeviceID:     deviceID,
				hap.TXTModel:        app.UserAgent,
				hap.TXTProtoVersion: "1.1",
				hap.TXTStateNumber:  "1",
				hap.TXTStatusFlags:  hap.StatusNotPaired,
				hap.TXTCategory:     calcCategoryID(conf.CategoryID),
				hap.TXTSetupHash:    hap.SetupHash(setupID, deviceID),
			},
		}
		entries = append(entries, srv.mdns)

		srv.UpdateStatus()

		if url := findHomeKitURL(stream.Sources()); url != "" {
			// 1. 作为 HomeKit 摄像头的透明代理
			srv.proxyURL = url
		} else {
			// 2. 作为基础 HomeKit 摄像头
			srv.accessory = camera.NewAccessory("AlexxIT", "go2rtc", name, "-", app.Version)
		}

		host := srv.mdns.Host(mdns.ServiceHAP)
		hosts[host] = srv
		servers[id] = srv

		log.Trace().Msgf("[homekit] new server: %s", srv.mdns)
	}

	api.HandleFunc(hap.PathPairSetup, hapHandler)
	api.HandleFunc(hap.PathPairVerify, hapHandler)

	go func() {
		if err := mdns.Serve(mdns.ServiceHAP, entries); err != nil {
			log.Error().Err(err).Caller().Send()
		}
	}()
}

var log zerolog.Logger
var hosts map[string]*server
var servers map[string]*server

func streamHandler(rawURL string) (core.Producer, error) {
	if srtp.Server == nil {
		return nil, errors.New("homekit: can't work without SRTP server")
	}

	rawURL, rawQuery, _ := strings.Cut(rawURL, "#")
	client, err := homekit.Dial(rawURL, srtp.Server)
	if client != nil && rawQuery != "" {
		query := streams.ParseQuery(rawQuery)
		client.MaxWidth = core.Atoi(query.Get("maxwidth"))
		client.MaxHeight = core.Atoi(query.Get("maxheight"))
		client.Bitrate = parseBitrate(query.Get("bitrate"))
	}

	return client, err
}

func resolve(host string) *server {
	if len(hosts) == 1 {
		for _, srv := range hosts {
			return srv
		}
	}
	if srv, ok := hosts[host]; ok {
		return srv
	}
	return nil
}

func hapHandler(w http.ResponseWriter, r *http.Request) {
	// 仅 Apple 设备支持在单个端口连接多个 HomeKit 摄像头。
	// 不支持 Home Assistant 或其他开源项目，
	// 因为它们不在请求中发送 Host 响应头。
	srv := resolve(r.Host)
	if srv == nil {
		log.Error().Msg("[homekit] unknown host: " + r.Host)
		return
	}
	srv.Handle(w, r)
}

func findHomeKitURL(sources []string) string {
	if len(sources) == 0 {
		return ""
	}

	url := sources[0]
	if strings.HasPrefix(url, "homekit") {
		return url
	}

	if strings.HasPrefix(url, "hass") {
		location, _ := streams.Location(url)
		if strings.HasPrefix(location, "homekit") {
			return location
		}
	}

	return ""
}

func parseBitrate(s string) int {
	n := len(s)
	if n == 0 {
		return 0
	}

	var k int
	switch n--; s[n] {
	case 'K':
		k = 1024
		s = s[:n]
	case 'M':
		k = 1024 * 1024
		s = s[:n]
	default:
		k = 1
	}

	return k * core.Atoi(s)
}
