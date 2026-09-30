package ffmpeg

import "sync"

const maxStderrBytes = 64 * 1024

// logTail retains bounded diagnostics even when FFmpeg logs for hours.
// exec.Cmd writes concurrently with status and stop diagnostics.
type logTail struct {
	mu   sync.Mutex
	data []byte
}

func (b *logTail) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if n >= maxStderrBytes {
		b.data = append(b.data[:0], p[n-maxStderrBytes:]...)
		return n, nil
	}
	if overflow := len(b.data) + n - maxStderrBytes; overflow > 0 {
		b.data = b.data[:copy(b.data, b.data[overflow:])]
	}
	b.data = append(b.data, p...)
	return n, nil
}

func (b *logTail) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}
