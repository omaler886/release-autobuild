package v2rayxhttp

import (
	"bytes"
	"io"
	"sync"

	E "github.com/sagernet/sing/common/exceptions"
)

type serverSession struct {
	mode      string
	queue     *packetQueue
	stream    *streamReader
	done      chan struct{}
	closeOnce sync.Once
}

func newServerSession(mode string) *serverSession {
	s := &serverSession{
		mode: mode,
		done: make(chan struct{}),
	}
	if mode == ModeStreamUp {
		s.stream = newStreamReader()
	} else {
		s.queue = newPacketQueue()
	}
	return s
}

func (s *serverSession) reader() io.Reader {
	if s.stream != nil {
		return s.stream
	}
	return s.queue
}

func (s *serverSession) push(seq int64, payload []byte) error {
	if s.queue == nil {
		return E.New("xhttp packet upload not available in stream-up mode")
	}
	return s.queue.Push(seq, payload)
}

func (s *serverSession) attachStream(reader io.ReadCloser) error {
	if s.stream == nil {
		return E.New("xhttp stream upload not available in packet-up mode")
	}
	return s.stream.Attach(reader)
}

func (s *serverSession) close() {
	s.closeOnce.Do(func() {
		close(s.done)
		if s.queue != nil {
			s.queue.Close()
		}
		if s.stream != nil {
			s.stream.Close()
		}
	})
}

type packetQueue struct {
	access  sync.Mutex
	cond    *sync.Cond
	chunks  map[int64][]byte
	nextSeq int64
	current *bytes.Reader
	closed  bool
}

func newPacketQueue() *packetQueue {
	q := &packetQueue{chunks: make(map[int64][]byte)}
	q.cond = sync.NewCond(&q.access)
	return q
}

func (q *packetQueue) Push(seq int64, payload []byte) error {
	q.access.Lock()
	defer q.access.Unlock()
	if q.closed {
		return io.ErrClosedPipe
	}
	if _, exists := q.chunks[seq]; exists {
		return E.New("duplicated xhttp seq: ", seq)
	}
	q.chunks[seq] = append([]byte(nil), payload...)
	q.cond.Broadcast()
	return nil
}

func (q *packetQueue) Read(p []byte) (int, error) {
	q.access.Lock()
	defer q.access.Unlock()
	for {
		if q.current != nil {
			n, err := q.current.Read(p)
			if err == io.EOF {
				q.current = nil
				if n > 0 {
					return n, nil
				}
				continue
			}
			return n, err
		}
		if chunk, found := q.chunks[q.nextSeq]; found {
			delete(q.chunks, q.nextSeq)
			q.nextSeq++
			q.current = bytes.NewReader(chunk)
			continue
		}
		if q.closed {
			return 0, io.EOF
		}
		q.cond.Wait()
	}
}

func (q *packetQueue) Close() {
	q.access.Lock()
	defer q.access.Unlock()
	q.closed = true
	q.cond.Broadcast()
}

type streamReader struct {
	access sync.Mutex
	cond   *sync.Cond
	reader io.ReadCloser
	err    error
	closed bool
}

func newStreamReader() *streamReader {
	r := &streamReader{}
	r.cond = sync.NewCond(&r.access)
	return r
}

func (r *streamReader) Attach(reader io.ReadCloser) error {
	r.access.Lock()
	defer r.access.Unlock()
	if r.closed {
		return io.ErrClosedPipe
	}
	if r.reader != nil {
		return E.New("xhttp stream already attached")
	}
	r.reader = reader
	r.cond.Broadcast()
	return nil
}

func (r *streamReader) Read(p []byte) (int, error) {
	r.access.Lock()
	for r.reader == nil && !r.closed {
		r.cond.Wait()
	}
	reader := r.reader
	err := r.err
	closed := r.closed
	r.access.Unlock()
	if err != nil {
		return 0, err
	}
	if closed && reader == nil {
		return 0, io.EOF
	}
	n, readErr := reader.Read(p)
	if readErr != nil {
		r.access.Lock()
		if r.err == nil {
			r.err = readErr
		}
		r.access.Unlock()
	}
	return n, readErr
}

func (r *streamReader) Close() {
	r.access.Lock()
	defer r.access.Unlock()
	r.closed = true
	if r.reader != nil {
		_ = r.reader.Close()
	}
	r.cond.Broadcast()
}
