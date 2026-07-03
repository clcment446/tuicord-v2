package store

// ring is a fixed-capacity FIFO buffer of messages. When full, pushing evicts
// the oldest message. It keeps message history bounded per channel.
type ring struct {
	buf   []Message
	start int // index of the oldest element
	size  int
}

func newRing(capacity int) *ring {
	if capacity < 1 {
		capacity = 1
	}
	return &ring{buf: make([]Message, capacity)}
}

func (r *ring) push(m Message) {
	if r.size < len(r.buf) {
		r.buf[(r.start+r.size)%len(r.buf)] = m
		r.size++
		return
	}
	// Full: overwrite the oldest and advance start.
	r.buf[r.start] = m
	r.start = (r.start + 1) % len(r.buf)
}

// slice returns the messages oldest-first as a fresh copy.
func (r *ring) slice() []Message {
	out := make([]Message, r.size)
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(r.start+i)%len(r.buf)]
	}
	return out
}

func (r *ring) replaceByNonce(nonce string, confirmed Message) bool {
	if i, ok := r.indexByNonce(nonce); ok {
		r.buf[i] = confirmed
		return true
	}
	return false
}

func (r *ring) markFailed(nonce string) bool {
	if i, ok := r.indexByNonce(nonce); ok {
		r.buf[i].Failed = true
		r.buf[i].Pending = false
		return true
	}
	return false
}

func (r *ring) indexByNonce(nonce string) (int, bool) {
	for i := 0; i < r.size; i++ {
		idx := (r.start + i) % len(r.buf)
		if r.buf[idx].Nonce == nonce {
			return idx, true
		}
	}
	return 0, false
}
