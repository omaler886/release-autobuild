package splithttp

import (
	"errors"
	"io"
	"sync"

	"github.com/metacubex/mihomo/common/buf"
)

type Packet struct {
	Reader io.ReadCloser
	Buffer *buf.Buffer
	Seq    uint64
}

type uploadQueue struct {
	mu         sync.Mutex
	cond       *sync.Cond
	window     [512]Packet // 🚀 环形滑动窗口
	nextSeq    uint64
	closed     bool
	reader     io.ReadCloser
	maxPackets int
}

func NewUploadQueue(maxPackets int) *uploadQueue {
	if maxPackets < 512 {
		maxPackets = 512
	}
	q := &uploadQueue{maxPackets: 512} // 强制对齐 512 窗口
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *uploadQueue) Push(p Packet) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		if p.Buffer != nil {
			p.Buffer.Release()
		}
		return io.ErrClosedPipe
	}

	if p.Reader != nil {
		if q.reader != nil {
			return errors.New("reader exists")
		}
		q.reader = p.Reader
		q.cond.Signal()
		return nil
	}

	// O(1) 重组逻辑
	pos := p.Seq % 512
	if p.Seq < q.nextSeq || p.Seq >= q.nextSeq+512 || q.window[pos].Buffer != nil {
		if p.Buffer != nil {
			p.Buffer.Release()
		}
		return errors.New("packet out of window or duplicate")
	}

	q.window[pos] = p
	q.cond.Signal()
	return nil
}

func (q *uploadQueue) Read(b []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		if q.reader != nil {
			r := q.reader
			q.mu.Unlock()
			n, err := r.Read(b)
			q.mu.Lock()
			return n, err
		}

		// 检查窗口当前位置
		pos := q.nextSeq % 512
		packet := q.window[pos]
		if packet.Buffer != nil {
			n := copy(b, packet.Buffer.Bytes())
			packet.Buffer.Advance(n)
			if packet.Buffer.IsEmpty() {
				packet.Buffer.Release()
				q.window[pos] = Packet{}
				q.nextSeq++
			} else {
				q.window[pos] = packet // 存回剩余部分
			}
			return n, nil
		}

		if q.closed {
			return 0, io.EOF
		}
		q.cond.Wait()
	}
}

func (q *uploadQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.cond.Broadcast()
	for i := range q.window {
		if q.window[i].Buffer != nil {
			q.window[i].Buffer.Release()
			q.window[i] = Packet{}
		}
	}
	if q.reader != nil {
		return q.reader.Close()
	}
	return nil
}
