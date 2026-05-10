package api

import (
	"sync"

	"github.com/aslanchik/go-phish/internal/pipeline"
)

const (
	bufSize    = 32 // max buffered events kept per investigation for Last-Event-ID replay
	subChanBuf = 32 // per-subscriber channel buffer
)

type seqEvent struct {
	Seq   int64
	Event pipeline.Event
}

type sub struct {
	ch chan seqEvent
}

// Broker fans pipeline events out to connected SSE clients.
// It keeps a ring buffer per investigation for Last-Event-ID reconnect replay.
type Broker struct {
	mu   sync.Mutex
	subs map[string][]*sub
	bufs map[string][]seqEvent
	seqs map[string]int64
}

func newBroker() *Broker {
	return &Broker{
		subs: make(map[string][]*sub),
		bufs: make(map[string][]seqEvent),
		seqs: make(map[string]int64),
	}
}

// Publish assigns a sequence number to ev, appends it to the ring buffer, and
// fans it out to all current subscribers for the investigation. Slow subscribers
// whose channel is full are silently skipped (their event is dropped, not queued).
func (b *Broker) Publish(ev pipeline.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := ev.InvestigationID
	b.seqs[id]++
	se := seqEvent{Seq: b.seqs[id], Event: ev}

	buf := append(b.bufs[id], se)
	if len(buf) > bufSize {
		buf = buf[len(buf)-bufSize:]
	}
	b.bufs[id] = buf

	for _, s := range b.subs[id] {
		select {
		case s.ch <- se:
		default:
		}
	}
}

// Subscribe registers a new client for invID. Any buffered events with
// Seq > afterSeq are immediately queued into the returned channel so the
// caller can replay missed events after a reconnect. The returned cancel
// function must be called when the client disconnects.
func (b *Broker) Subscribe(invID string, afterSeq int64) (<-chan seqEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s := &sub{ch: make(chan seqEvent, subChanBuf)}

	for _, se := range b.bufs[invID] {
		if se.Seq > afterSeq {
			select {
			case s.ch <- se:
			default:
			}
		}
	}

	b.subs[invID] = append(b.subs[invID], s)

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.subs[invID]
		for i, existing := range subs {
			if existing == s {
				b.subs[invID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}

	return s.ch, cancel
}
