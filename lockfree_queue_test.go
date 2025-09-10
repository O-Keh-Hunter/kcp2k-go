package kcp2k

import (
	"sync"
	"testing"
	"time"
)

// Test basic queue operations
func TestLockFreeQueueBasic(t *testing.T) {
	q := NewLockFreeQueue[int]()

	// Test empty queue
	if !q.IsEmpty() {
		t.Error("New queue should be empty")
	}

	if _, ok := q.Dequeue(); ok {
		t.Error("Dequeue from empty queue should return false")
	}

	// Test enqueue and dequeue
	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)

	if q.IsEmpty() {
		t.Error("Queue should not be empty after enqueue")
	}

	val, ok := q.Dequeue()
	if !ok || val != 1 {
		t.Errorf("Expected 1, got %d", val)
	}

	val, ok = q.Dequeue()
	if !ok || val != 2 {
		t.Errorf("Expected 2, got %d", val)
	}

	val, ok = q.Dequeue()
	if !ok || val != 3 {
		t.Errorf("Expected 3, got %d", val)
	}

	if !q.IsEmpty() {
		t.Error("Queue should be empty after dequeuing all items")
	}
}

// Test concurrent operations
func TestLockFreeQueueConcurrent(t *testing.T) {
	q := NewLockFreeQueue[int]()
	const numProducers = 4
	const numConsumers = 4
	const itemsPerProducer = 1000

	var wg sync.WaitGroup
	produced := make([][]int, numProducers)
	consumed := make([][]int, numConsumers)

	// Start producers
	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		go func(producerId int) {
			defer wg.Done()
			produced[producerId] = make([]int, itemsPerProducer)
			for j := 0; j < itemsPerProducer; j++ {
				value := producerId*itemsPerProducer + j
				produced[producerId][j] = value
				q.Enqueue(value)
			}
		}(i)
	}

	// Start consumers
	for i := 0; i < numConsumers; i++ {
		wg.Add(1)
		go func(consumerId int) {
			defer wg.Done()
			consumed[consumerId] = make([]int, 0)
			for {
				if val, ok := q.Dequeue(); ok {
					consumed[consumerId] = append(consumed[consumerId], val)
				} else {
					// Check if all producers are done
					time.Sleep(1 * time.Millisecond)
					if val, ok := q.Dequeue(); !ok {
						// Double check - if still empty, we're probably done
						break
					} else {
						consumed[consumerId] = append(consumed[consumerId], val)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all items were consumed
	allProduced := make(map[int]bool)
	for _, items := range produced {
		for _, item := range items {
			allProduced[item] = true
		}
	}

	allConsumed := make(map[int]bool)
	for _, items := range consumed {
		for _, item := range items {
			if allConsumed[item] {
				t.Errorf("Item %d consumed multiple times", item)
			}
			allConsumed[item] = true
		}
	}

	if len(allProduced) != len(allConsumed) {
		t.Errorf("Produced %d items, consumed %d items", len(allProduced), len(allConsumed))
	}

	for item := range allProduced {
		if !allConsumed[item] {
			t.Errorf("Item %d was produced but not consumed", item)
		}
	}
}

// Benchmark lock-free queue vs channel
func BenchmarkLockFreeQueue(b *testing.B) {
	q := NewLockFreeQueue[int]()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.Enqueue(1)
			q.Dequeue()
		}
	})
}

func BenchmarkChannel(b *testing.B) {
	ch := make(chan int, 1000)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			select {
			case ch <- 1:
			default:
			}
			select {
			case <-ch:
			default:
			}
		}
	})
}