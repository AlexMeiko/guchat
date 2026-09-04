package sandbox

import "time"

type terminalExpireItem struct {
	key       terminalKey
	expiresAt time.Time
}

type terminalExpireHeap []terminalExpireItem

func (h terminalExpireHeap) Len() int {
	return len(h)
}

func (h terminalExpireHeap) Less(i, j int) bool {
	return h[i].expiresAt.Before(h[j].expiresAt)
}

func (h terminalExpireHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *terminalExpireHeap) Push(x any) {
	*h = append(*h, x.(terminalExpireItem))
}

func (h *terminalExpireHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func (h terminalExpireHeap) Peek() (terminalExpireItem, bool) {
	if len(h) == 0 {
		return terminalExpireItem{}, false
	}
	return h[0], true
}
