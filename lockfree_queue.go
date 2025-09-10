package kcp2k

import (
	"sync/atomic"
	"unsafe"
)

// LockFreeQueue is a lock-free queue implementation using atomic operations
// Based on Michael & Scott algorithm for lock-free queues
type LockFreeQueue[T any] struct {
	head unsafe.Pointer // *node[T]
	tail unsafe.Pointer // *node[T]
}

// node represents a queue node
type node[T any] struct {
	data T
	next unsafe.Pointer // *node[T]
}

// NewLockFreeQueue creates a new lock-free queue
func NewLockFreeQueue[T any]() *LockFreeQueue[T] {
	// Create a dummy node
	dummy := &node[T]{}
	q := &LockFreeQueue[T]{
		head: unsafe.Pointer(dummy),
		tail: unsafe.Pointer(dummy),
	}
	return q
}

// Enqueue adds an item to the queue
func (q *LockFreeQueue[T]) Enqueue(item T) {
	newNode := &node[T]{data: item}
	newNodePtr := unsafe.Pointer(newNode)

	for {
		tail := (*node[T])(atomic.LoadPointer(&q.tail))
		next := (*node[T])(atomic.LoadPointer(&tail.next))

		// Check if tail is still the last node
		if tail == (*node[T])(atomic.LoadPointer(&q.tail)) {
			if next == nil {
				// Try to link new node at the end of the list
				if atomic.CompareAndSwapPointer(&tail.next, nil, newNodePtr) {
					break // Enqueue is done
				}
			} else {
				// Tail was not pointing to the last node, try to swing tail to the next node
				atomic.CompareAndSwapPointer(&q.tail, unsafe.Pointer(tail), unsafe.Pointer(next))
			}
		}
	}

	// Try to swing tail to the new node
	atomic.CompareAndSwapPointer(&q.tail, unsafe.Pointer((*node[T])(atomic.LoadPointer(&q.tail))), newNodePtr)
}

// Dequeue removes and returns an item from the queue
// Returns the item and true if successful, zero value and false if queue is empty
func (q *LockFreeQueue[T]) Dequeue() (T, bool) {
	var zero T

	for {
		head := (*node[T])(atomic.LoadPointer(&q.head))
		tail := (*node[T])(atomic.LoadPointer(&q.tail))
		next := (*node[T])(atomic.LoadPointer(&head.next))

		// Check if head is still the first node
		if head == (*node[T])(atomic.LoadPointer(&q.head)) {
			if head == tail {
				if next == nil {
					// Queue is empty
					return zero, false
				}
				// Tail is falling behind, try to advance it
				atomic.CompareAndSwapPointer(&q.tail, unsafe.Pointer(tail), unsafe.Pointer(next))
			} else {
				if next == nil {
					// This should not happen in a correct implementation
					continue
				}

				// Read data before CAS, otherwise another dequeue might free the next node
				data := next.data

				// Try to swing head to the next node
				if atomic.CompareAndSwapPointer(&q.head, unsafe.Pointer(head), unsafe.Pointer(next)) {
					return data, true
				}
			}
		}
	}
}

// IsEmpty checks if the queue is empty
func (q *LockFreeQueue[T]) IsEmpty() bool {
	head := (*node[T])(atomic.LoadPointer(&q.head))
	tail := (*node[T])(atomic.LoadPointer(&q.tail))
	next := (*node[T])(atomic.LoadPointer(&head.next))
	return head == tail && next == nil
}

// Size returns an approximate size of the queue
// Note: This is not atomic and should be used for monitoring purposes only
func (q *LockFreeQueue[T]) Size() int {
	count := 0
	current := (*node[T])(atomic.LoadPointer(&q.head))

	for current != nil {
		next := (*node[T])(atomic.LoadPointer(&current.next))
		if next != nil {
			count++
		}
		current = next
	}

	return count
}