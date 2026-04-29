package helper

import (
	"io"
	"strings"
	"sync"
)

// recorderChunkSize is the fixed allocation unit used by the chunk pool.
// 32 KiB matches typical SSE chunk batches and keeps individual pool entries
// small enough that retaining one across a request is cheap.
const recorderChunkSize = 32 * 1024

// chunkPool reuses fixed-size byte slices for stream recording so that a
// burst of stream traffic does not produce equivalent burst of allocations.
// Pool entries are pointer-to-slice to satisfy the standard sync.Pool guidance
// (store pointer-like values, not slice headers).
var chunkPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, recorderChunkSize)
		return &b
	},
}

func acquireChunk() *[]byte {
	b := chunkPool.Get().(*[]byte)
	*b = (*b)[:0]
	return b
}

func releaseChunk(b *[]byte) {
	if b == nil || cap(*b) != recorderChunkSize {
		// Defensive: only recycle chunks that match the pool's expected
		// capacity. A mismatched chunk would either waste pool slots
		// (oversized) or corrupt assumptions (undersized).
		return
	}
	*b = (*b)[:0]
	chunkPool.Put(b)
}

// StreamResponseRecorder wraps an upstream response body and records every
// byte that flows through, so the full original response can later be
// persisted to the consume log. The recorder is intentionally lock-free: its
// concurrency contract is that exactly one goroutine drives I/O at a time.
//
// Concrete contract for the relay pipeline:
//   - Read is called by the SSE scanner goroutine spawned in
//     StreamScannerHandler. That is the only writer.
//   - Close is called from StreamScannerHandler's defer AFTER wg.Wait has
//     observed the scanner goroutine exiting. Read therefore cannot race
//     Close.
//   - GetRecordedData / GetRecordedString are called by the relay handler
//     after DoResponse has returned, i.e. strictly after Close.
//
// Because all access is serialized by program structure, no internal mutex
// is required.
type StreamResponseRecorder struct {
	originalBody io.ReadCloser

	// chunks holds completed (full) buffers. tail is the current append
	// target, which may not be full. size is the running total of recorded
	// bytes; it is authoritative and used to pre-size the contiguous
	// output produced by GetRecordedString / GetRecordedData.
	chunks []*[]byte
	tail   *[]byte
	size   int

	closed bool
}

// NewStreamResponseRecorder creates a recorder that wraps originalBody.
// The recorder allocates chunks lazily — no buffer is reserved until the
// first Read produces data.
func NewStreamResponseRecorder(originalBody io.ReadCloser) *StreamResponseRecorder {
	return &StreamResponseRecorder{originalBody: originalBody}
}

// Read implements io.Reader. Bytes read from the upstream body are also
// appended to the internal chunk buffer.
func (r *StreamResponseRecorder) Read(p []byte) (n int, err error) {
	if r.closed {
		return 0, io.EOF
	}
	n, err = r.originalBody.Read(p)
	if n > 0 {
		r.appendBytes(p[:n])
	}
	return n, err
}

// appendBytes copies p into the chunk buffer, allocating new chunks from the
// pool as needed. It never grows or copies an existing chunk, which is the
// key behavioral difference from bytes.Buffer (no doubling churn).
func (r *StreamResponseRecorder) appendBytes(p []byte) {
	for len(p) > 0 {
		if r.tail == nil || len(*r.tail) == cap(*r.tail) {
			if r.tail != nil {
				r.chunks = append(r.chunks, r.tail)
			}
			r.tail = acquireChunk()
		}
		space := cap(*r.tail) - len(*r.tail)
		if space > len(p) {
			space = len(p)
		}
		*r.tail = append(*r.tail, p[:space]...)
		p = p[space:]
		r.size += space
	}
}

// Close closes the wrapped upstream body. The internal recorded data is
// preserved and remains accessible via GetRecordedString / GetRecordedData
// until Release (or Reset) is called.
func (r *StreamResponseRecorder) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if r.originalBody != nil {
		return r.originalBody.Close()
	}
	return nil
}

// GetRecordedData returns the recorded bytes assembled into a single
// contiguous slice. The recorder is not modified; the caller may invoke this
// (or GetRecordedString) repeatedly until Release is called.
func (r *StreamResponseRecorder) GetRecordedData() []byte {
	if r.size == 0 {
		return nil
	}
	out := make([]byte, 0, r.size)
	for _, c := range r.chunks {
		out = append(out, *c...)
	}
	if r.tail != nil {
		out = append(out, *r.tail...)
	}
	return out
}

// GetRecordedString returns the recorded bytes as a string. Like
// GetRecordedData, this does not consume or release the internal chunks.
func (r *StreamResponseRecorder) GetRecordedString() string {
	if r.size == 0 {
		return ""
	}
	var sb strings.Builder
	sb.Grow(r.size)
	for _, c := range r.chunks {
		sb.Write(*c)
	}
	if r.tail != nil {
		sb.Write(*r.tail)
	}
	return sb.String()
}

// Reset discards any recorded bytes and recycles chunk buffers back to the
// pool. Reads after Reset will start recording from scratch.
func (r *StreamResponseRecorder) Reset() {
	r.recycleChunks()
}

// Release returns chunk buffers to the pool and clears recorded state. Call
// it once the recorded payload is no longer needed — typically as a deferred
// call right after the recorder is created in the handler. It is safe to
// call multiple times.
//
// Calling Release does NOT close the underlying body; use Close for that.
func (r *StreamResponseRecorder) Release() {
	r.recycleChunks()
}

func (r *StreamResponseRecorder) recycleChunks() {
	for _, c := range r.chunks {
		releaseChunk(c)
	}
	r.chunks = nil
	if r.tail != nil {
		releaseChunk(r.tail)
		r.tail = nil
	}
	r.size = 0
}
