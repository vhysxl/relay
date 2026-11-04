package worker

import "sync"

// QueueNotifier allows goroutines waiting for tasks on specific queues
// to be notified immediately when a new task becomes runnable on that queue.
type QueueNotifier interface {
	Notify(queue string)
	Subscribe(queue string) (ch <-chan struct{}, cancel func())
}

type queueNotifier struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan struct{}]struct{}
}

// NewQueueNotifier creates a new in-memory QueueNotifier.
func NewQueueNotifier() QueueNotifier {
	return &queueNotifier{
		subscribers: make(map[string]map[chan struct{}]struct{}),
	}
}

func (n *queueNotifier) Notify(queue string) {
	if queue == "" {
		queue = "legacy"
	}
	n.mu.RLock()
	defer n.mu.RUnlock()

	for ch := range n.subscribers[queue] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (n *queueNotifier) Subscribe(queue string) (<-chan struct{}, func()) {
	if queue == "" {
		queue = "legacy"
	}
	ch := make(chan struct{}, 1)

	n.mu.Lock()
	if n.subscribers[queue] == nil {
		n.subscribers[queue] = make(map[chan struct{}]struct{})
	}
	n.subscribers[queue][ch] = struct{}{}
	n.mu.Unlock()

	cancel := func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		if subs, ok := n.subscribers[queue]; ok {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(n.subscribers, queue)
			}
		}
	}

	return ch, cancel
}
