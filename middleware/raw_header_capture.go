package middleware

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
)

// 每个连接最多缓存这么多字节用于 raw header 解析；超过则停止 tee（不会拒绝读取）。
// 选 64KB：远大于 Go 默认 1MB 的 MaxHeaderBytes 在实际生产中的合理用量（典型 < 8KB），
// 同时避免 keep-alive 长连接上 body 字节堆积。
const rawHeaderBufCap = 64 * 1024

type ctxKeyRawHeader struct{}

// rawHeaderState 是 per-net.Conn 的捕获缓冲；保护读写并发。
type rawHeaderState struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// CaptureListener 包装 net.Listener，给每个 accept 出的连接套上 tee buffer。
// 这是把"原始线缆字节"暴露给 handler 的唯一办法 —— net/http 解析器在 readRequest
// 里就调用 textproto.CanonicalMIMEHeaderKey 把 header name 规范化了，等 handler 拿到
// r.Header 时大小写和顺序都已经丢失。
type CaptureListener struct {
	net.Listener
}

func (l *CaptureListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return c, err
	}
	return &captureConn{Conn: c, state: &rawHeaderState{}}, nil
}

type captureConn struct {
	net.Conn
	state *rawHeaderState
}

func (c *captureConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.state.mu.Lock()
		// 软上限：到容量后不再 tee，保护内存；header 通常在连接初期就出现，
		// 命中容量的多是 body 在堆积，丢掉对 header 解析没影响。
		if c.state.buf.Len()+n <= rawHeaderBufCap {
			c.state.buf.Write(p[:n])
		}
		c.state.mu.Unlock()
	}
	return n, err
}

// RawHeaderConnContext 注入 http.Server.ConnContext：把 per-conn 捕获 state 放进 ctx，
// handler 通过 r.Context().Value(...) 取出。
func RawHeaderConnContext(ctx context.Context, c net.Conn) context.Context {
	if cc, ok := c.(*captureConn); ok {
		return context.WithValue(ctx, ctxKeyRawHeader{}, cc.state)
	}
	return ctx
}

// HeaderPair 保留原貌：name 的大小写、value 的原始内容，pair 之间维持 wire 顺序。
type HeaderPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// 已知的 HTTP 请求方法；用于在缓冲里识别请求行的开头（行首 + METHOD + 空格）。
// 不在该表里的方法会回落到 canonical fallback；这覆盖了 99% 的实际流量。
var httpMethods = [][]byte{
	[]byte("GET "),
	[]byte("POST "),
	[]byte("PUT "),
	[]byte("DELETE "),
	[]byte("HEAD "),
	[]byte("OPTIONS "),
	[]byte("PATCH "),
	[]byte("CONNECT "),
	[]byte("TRACE "),
}

// findRequestLineStart 在 buf[from:] 中找下一个请求行的起始位置（行首 + 已知方法）。
// 找不到返回 -1。
func findRequestLineStart(buf []byte, from int) int {
	for i := from; i < len(buf); i++ {
		// 必须是行首：要么是 buf 起点，要么前一个字节是 '\n'
		if i != 0 && buf[i-1] != '\n' {
			continue
		}
		for _, m := range httpMethods {
			if bytes.HasPrefix(buf[i:], m) {
				return i
			}
		}
	}
	return -1
}

// ExtractRawHeaders 从 conn 缓冲里提取"当前请求"的原始 header 块。
//
// 算法：正向扫描所有形如 "<METHOD> ... \r\n...\r\n\r\n" 的块，保留最后一个 —— 即当前
// handler 对应的那一次请求。然后切掉缓冲里到该块尾为止的字节，避免长连接上越积越多。
//
// 仅适用于 HTTP/1.x：HTTP/2 走 HPACK 二进制帧，conn.Read 看到的不是 ASCII header 文本，
// 解析会失败并返回 nil（调用方应当回落到 canonical Header）。
//
// 副作用：会从 buf 头部丢弃已消费字节（state.buf.Next(blockEnd)）。
func ExtractRawHeaders(r *http.Request) ([]HeaderPair, string) {
	state, ok := r.Context().Value(ctxKeyRawHeader{}).(*rawHeaderState)
	if !ok || state == nil {
		return nil, ""
	}
	state.mu.Lock()
	defer state.mu.Unlock()

	raw := state.buf.Bytes()
	if len(raw) == 0 {
		return nil, ""
	}

	// 找最后一个完整 header 块（请求行 ... \r\n\r\n）。
	pos := 0
	rlStart, blockEnd := -1, -1
	for {
		s := findRequestLineStart(raw, pos)
		if s < 0 {
			break
		}
		e := bytes.Index(raw[s:], []byte("\r\n\r\n"))
		if e < 0 {
			break
		}
		rlStart = s
		blockEnd = s + e + 4
		pos = blockEnd
	}
	if rlStart < 0 {
		return nil, ""
	}
	block := raw[rlStart:blockEnd]

	// 跳过请求行（第一行 \r\n 之前），剩余按 \r\n 切，丢掉最后的空行
	nlIdx := bytes.Index(block, []byte("\r\n"))
	if nlIdx < 0 {
		return nil, ""
	}
	headersSection := block[nlIdx+2 : len(block)-2] // 去掉末尾 \r\n
	pairs := make([]HeaderPair, 0, 16)
	for _, line := range bytes.Split(headersSection, []byte("\r\n")) {
		if len(line) == 0 {
			continue
		}
		colon := bytes.IndexByte(line, ':')
		if colon < 0 {
			continue // 不合法行，跳过
		}
		name := string(line[:colon])
		value := strings.TrimLeft(string(line[colon+1:]), " \t")
		value = strings.TrimRight(value, " \t")
		pairs = append(pairs, HeaderPair{Name: name, Value: value})
	}

	// 截掉已消费前缀，保留可能已经流到的 body 字节（下一请求 headers 会在它之后）。
	state.buf.Next(blockEnd)

	return pairs, string(block)
}
