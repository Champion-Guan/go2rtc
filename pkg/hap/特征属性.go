package hap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
)

// Character 使用 Aqara 属性顺序
// PW 属性应省略 Value
// PR 属性允许 Value 为空
type Character struct {
	Desc string `json:"description,omitempty"`

	IID    uint64   `json:"iid"`
	Type   string   `json:"type"`
	Format string   `json:"format"`
	Value  any      `json:"value,omitempty"`
	Perms  []string `json:"perms"`

	//MaxLen   int    `json:"maxLen,omitempty"`
	//Unit     string `json:"unit,omitempty"`
	//MinValue any    `json:"minValue,omitempty"`
	//MaxValue any    `json:"maxValue,omitempty"`
	//MinStep  any    `json:"minStep,omitempty"`
	//ValidVal []any  `json:"valid-values,omitempty"`

	listeners map[io.Writer]bool
}

func (c *Character) AddListener(w io.Writer) {
	// 待办：添加 sync.Mutex
	if c.listeners == nil {
		c.listeners = map[io.Writer]bool{}
	}
	c.listeners[w] = true
}

func (c *Character) RemoveListener(w io.Writer) {
	delete(c.listeners, w)

	if len(c.listeners) == 0 {
		c.listeners = nil
	}
}

func (c *Character) NotifyListeners(ignore io.Writer) error {
	if c.listeners == nil {
		return nil
	}

	data, err := c.GenerateEvent()
	if err != nil {
		return err
	}

	for w := range c.listeners {
		if w == ignore {
			continue
		}
		if _, err = w.Write(data); err != nil {
			// 错误无需传播，仅移除监听器
			c.RemoveListener(w)
		}
	}

	return nil
}

// GenerateEvent 生成带原始 HTTP 头的事件
func (c *Character) GenerateEvent() (data []byte, err error) {
	v := JSONCharacters{
		Value: []JSONCharacter{
			{AID: DeviceAID, IID: c.IID, Value: c.Value},
		},
	}
	if data, err = json.Marshal(v); err != nil {
		return
	}

	res := http.Response{
		StatusCode:    http.StatusOK,
		ProtoMajor:    1,
		ProtoMinor:    0,
		Header:        http.Header{"Content-Type": []string{MimeJSON}},
		ContentLength: int64(len(data)),
		Body:          io.NopCloser(bytes.NewReader(data)),
	}

	buf := bytes.NewBuffer([]byte{0})
	if err = res.Write(buf); err != nil {
		return
	}
	copy(buf.Bytes(), "EVENT")

	return buf.Bytes(), err
}

// 设置新值并通知监听器
func (c *Character) Set(v any) (err error) {
	if err = c.Write(v); err != nil {
		return
	}
	return c.NotifyListeners(nil)
}

// 以正确格式写入新值
func (c *Character) Write(v any) (err error) {
	switch c.Format {
	case "tlv8":
		c.Value, err = tlv8.MarshalBase64(v)

	case "bool":
		switch v := v.(type) {
		case bool:
			c.Value = v
		case float64:
			c.Value = v != 0
		}
	}
	return
}

// ReadTLV8 将值解析到正确结构
func (c *Character) ReadTLV8(v any) (err error) {
	if s, ok := c.Value.(string); ok {
		return tlv8.UnmarshalBase64(s, v)
	}
	return fmt.Errorf("hap: can't read value: %v", v)
}

func (c *Character) ReadBool() (bool, error) {
	if v, ok := c.Value.(bool); ok {
		return v, nil
	}
	return false, fmt.Errorf("hap: can't read value: %v", c.Value)
}

func (c *Character) String() string {
	data, err := json.Marshal(c)
	if err != nil {
		return "ERROR"
	}
	return string(data)
}
