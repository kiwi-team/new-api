package helper

import (
	"bytes"
	"io"
	"sync"
)

// StreamResponseRecorder 包装原始响应体，在数据流过时进行记录
// 不影响流式传输的实时性
type StreamResponseRecorder struct {
	originalBody io.ReadCloser
	recordedData *bytes.Buffer
	mutex        sync.Mutex
	closed       bool
}

// NewStreamResponseRecorder 创建一个新的流式响应记录器
func NewStreamResponseRecorder(originalBody io.ReadCloser) *StreamResponseRecorder {
	return &StreamResponseRecorder{
		originalBody: originalBody,
		recordedData: &bytes.Buffer{},
		mutex:        sync.Mutex{},
		closed:       false,
	}
}

// Read 实现 io.Reader 接口
// 在读取数据的同时记录到缓冲区
func (r *StreamResponseRecorder) Read(p []byte) (n int, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.closed {
		return 0, io.EOF
	}

	// 从原始响应体读取数据
	n, err = r.originalBody.Read(p)
	if n > 0 {
		// 将读取的数据记录到缓冲区
		r.recordedData.Write(p[:n])
	}

	return n, err
}

// Close 实现 io.Closer 接口
func (r *StreamResponseRecorder) Close() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true
	return r.originalBody.Close()
}

// GetRecordedData 获取记录的完整响应数据
// 这个方法应该在流式传输完成后调用
func (r *StreamResponseRecorder) GetRecordedData() []byte {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.recordedData.Bytes()
}

// GetRecordedString 获取记录的完整响应数据的字符串形式
func (r *StreamResponseRecorder) GetRecordedString() string {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.recordedData.String()
}

// Reset 重置记录器（清空已记录的数据）
func (r *StreamResponseRecorder) Reset() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.recordedData.Reset()
}
