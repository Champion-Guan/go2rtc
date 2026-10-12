package streams

import (
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Streams map[string]any    `yaml:"streams"`
		Publish map[string]any    `yaml:"publish"`
		Preload map[string]string `yaml:"preload"`
	}

	app.LoadConfig(&cfg)

	log = app.GetLogger("streams")

	for name, item := range cfg.Streams {
		streams[name] = NewStream(item)
	}

	api.HandleFunc("api/streams", apiStreams)
	api.HandleFunc("api/streams.dot", apiStreamsDOT)
	api.HandleFunc("api/preload", apiPreload)
	api.HandleFunc("api/schemes", apiSchemes)

	if cfg.Publish == nil && cfg.Preload == nil {
		return
	}

	time.AfterFunc(time.Second, func() {
		// 遍历空映射是安全的
		for name, dst := range cfg.Publish {
			if stream := Get(name); stream != nil {
				Publish(stream, dst)
			}
		}
		for name, rawQuery := range cfg.Preload {
			if err := AddPreload(name, rawQuery); err != nil {
				log.Error().Err(err).Caller().Send()
			}
		}
	})
}

func New(name string, sources ...string) (*Stream, error) {
	for _, source := range sources {
		if !HasProducer(source) {
			return nil, errors.New("streams: source not supported")
		}

		if err := Validate(source); err != nil {
			return nil, err
		}
	}

	stream := NewStream(sources)

	streamsMu.Lock()
	streams[name] = stream
	streamsMu.Unlock()

	return stream, nil
}

func Patch(name string, source string) (*Stream, error) {
	streamsMu.Lock()
	defer streamsMu.Unlock()

	// 检查源是否指向 go2rtc 中的流名称
	if u, err := url.Parse(source); err == nil && u.Scheme == "rtsp" && len(u.Path) > 1 {
		rtspName := u.Path[1:]
		if stream, ok := streams[rtspName]; ok {
			if streams[name] != stream {
				// 将 streams[name] 作为 streams[rtspName] 的别名
				streams[name] = stream
			}
			return stream, nil
		}
	}

	if stream, ok := streams[source]; ok {
		if name != source {
			// 将 streams[name] 作为 streams[source] 的别名
			streams[name] = stream
		}
		return stream, nil
	}

	// 检查源地址是否使用受支持的协议
	if !HasProducer(source) {
		return nil, errors.New("streams: source not supported")
	}

	if err := Validate(source); err != nil {
		return nil, err
	}

	// 检查同名流是否已存在
	if stream, ok := streams[name]; ok {
		stream.SetSource(source)
		return stream, nil
	}

	// 创建指定名称的新流
	stream := NewStream(source)
	streams[name] = stream
	return stream, nil
}

func GetOrPatch(query url.Values) (*Stream, error) {
	// 检查是否存在 src 参数
	source := query.Get("src")
	if source == "" {
		return nil, errors.New("streams: source empty")
	}

	// 检查 src 是否为流名称
	if stream := Get(source); stream != nil {
		return stream, nil
	}

	// 检查是否提供 name 参数
	if name := query.Get("name"); name != "" {
		return Patch(name, source)
	}

	// 返回以 src 为名称的新流
	return Patch(source, source)
}

var log zerolog.Logger

// 流映射表

var streams = map[string]*Stream{}
var streamsMu sync.Mutex

func Get(name string) *Stream {
	streamsMu.Lock()
	defer streamsMu.Unlock()
	return streams[name]
}

func Delete(name string) {
	streamsMu.Lock()
	defer streamsMu.Unlock()
	delete(streams, name)
}

func GetAllNames() []string {
	streamsMu.Lock()
	names := make([]string, 0, len(streams))
	for name := range streams {
		names = append(names, name)
	}
	streamsMu.Unlock()
	return names
}

func GetAllSources() map[string][]string {
	streamsMu.Lock()
	sources := make(map[string][]string, len(streams))
	for name, stream := range streams {
		sources[name] = stream.Sources()
	}
	streamsMu.Unlock()
	return sources
}
