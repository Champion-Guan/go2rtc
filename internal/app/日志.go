package app

import (
	"io"
	"os"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/creds"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
)

var MemoryLog = newBuffer()

func GetLogger(module string) zerolog.Logger {
	Logger.Trace().Str("module", module).Msgf("[log] init")

	if s, ok := modules[module]; ok {
		lvl, err := zerolog.ParseLevel(s)
		if err == nil {
			return Logger.Level(lvl)
		}
		Logger.Warn().Err(err).Caller().Send()
	}

	return Logger
}

// initLogger 支持：
// - output：空值（仅内存）、stderr、stdout
// - format：空值（自动检测颜色支持）、color、json、text
// - time：空值（禁用时间戳）、UNIXMS、UNIXMICRO、UNIXNANO
// - level:  disabled, trace, debug, info, warn, error...
func initLogger() {
	var cfg struct {
		Mod map[string]string `yaml:"log"`
	}

	cfg.Mod = modules // defaults

	LoadConfig(&cfg)

	var writer io.Writer

	switch output, path, _ := strings.Cut(modules["output"], ":"); output {
	case "stderr":
		writer = os.Stderr
	case "stdout":
		writer = os.Stdout
	case "file":
		if path == "" {
			path = "go2rtc.log"
		}
		// 失败时仅提供 MemoryLog
		writer, _ = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}

	timeFormat := modules["time"]

	if writer != nil {
		if format := modules["format"]; format != "json" {
			console := &zerolog.ConsoleWriter{Out: writer}

			switch format {
			case "text":
				console.NoColor = true
			case "color":
				console.NoColor = false // useless, but anyway
			default:
				// 自动检测输出是否支持颜色
				// go-isatty 是 go-colorable 的依赖，后者是 ConsoleWriter 的依赖
				console.NoColor = !isatty.IsTerminal(writer.(*os.File).Fd())
			}

			if timeFormat != "" {
				console.TimeFormat = "15:04:05.000"
			} else {
				console.PartsOrder = []string{
					zerolog.LevelFieldName,
					zerolog.CallerFieldName,
					zerolog.MessageFieldName,
				}
			}

			writer = console
		}

		writer = zerolog.MultiLevelWriter(writer, MemoryLog)
	} else {
		writer = MemoryLog
	}

	writer = creds.SecretWriter(writer)

	lvl, _ := zerolog.ParseLevel(modules["level"])
	Logger = zerolog.New(writer).Level(lvl)

	if timeFormat != "" {
		zerolog.TimeFieldFormat = timeFormat
		Logger = Logger.With().Timestamp().Logger()
	}
}

var Logger zerolog.Logger

// 模块日志级别
var modules = map[string]string{
	"format": "", // useless, but anyway
	"level":  "info",
	"output": "stdout", // TODO: change to stderr someday
	"time":   zerolog.TimeFormatUnixMs,
}

const (
	chunkCount = 16
	chunkSize  = 1 << 16
)

type circularBuffer struct {
	chunks [][]byte
	r, w   int
	mu     sync.Mutex
}

func newBuffer() *circularBuffer {
	b := &circularBuffer{chunks: make([][]byte, 0, chunkCount)}
	// 创建第一个数据块
	b.chunks = append(b.chunks, make([]byte, 0, chunkSize))
	return b
}

func (b *circularBuffer) Write(p []byte) (n int, err error) {
	n = len(p)

	b.mu.Lock()
	// 检查数据块是否有内容
	if len(b.chunks[b.w])+n > chunkSize {
		// 增加写入块索引
		if b.w++; b.w == chunkCount {
			b.w = 0
		}
		// 检查溢出
		if b.r == b.w {
			// 增加读取块索引
			if b.r++; b.r == chunkCount {
				b.r = 0
			}
		}
		// 检查当前数据块是否存在
		if b.w == len(b.chunks) {
			// 分配新数据块
			b.chunks = append(b.chunks, make([]byte, 0, chunkSize))
		} else {
			// 重置当前块的长度
			b.chunks[b.w] = b.chunks[b.w][:0]
		}
	}

	b.chunks[b.w] = append(b.chunks[b.w], p...)
	b.mu.Unlock()
	return
}

func (b *circularBuffer) WriteTo(w io.Writer) (n int64, err error) {
	buf := make([]byte, 0, chunkCount*chunkSize)

	// 在互斥锁内使用临时缓冲，因为 w.Write 可能耗时较长
	b.mu.Lock()
	for i := b.r; ; {
		buf = append(buf, b.chunks[i]...)
		if i == b.w {
			break
		}
		if i++; i == chunkCount {
			i = 0
		}
	}
	b.mu.Unlock()

	nn, err := w.Write(buf)
	return int64(nn), err
}

func (b *circularBuffer) Reset() {
	b.mu.Lock()
	b.chunks[0] = b.chunks[0][:0]
	b.r = 0
	b.w = 0
	b.mu.Unlock()
}
