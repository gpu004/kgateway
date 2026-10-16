package statussync

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hegel.dev/go/hegel"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func TestWorkerPoolConcurrentPushProperties(t *testing.T) {
	hegel.Test(t, func(ht *hegel.T) {
		maxWorkers := uint(hegel.Draw(ht, hegel.Integers(1, 4)))
		resources := []Resource{
			{GroupVersionKind: schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"}, NamespacedName: types.NamespacedName{Namespace: "ns", Name: "a"}},
			{GroupVersionKind: schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"}, NamespacedName: types.NamespacedName{Namespace: "ns", Name: "b"}},
			{GroupVersionKind: schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"}, NamespacedName: types.NamespacedName{Namespace: "ns", Name: "c"}},
		}
		workDelays := make(map[Resource]time.Duration, len(resources))
		for _, resource := range resources {
			workDelays[resource] = time.Duration(hegel.Draw(ht, hegel.Integers(0, 250))) * time.Microsecond
		}
		pushes := append(append([]Resource(nil), resources...), resources...)
		for i := len(pushes) - 1; i > 0; i-- {
			j := hegel.Draw(ht, hegel.Integers(0, i))
			pushes[i], pushes[j] = pushes[j], pushes[i]
		}
		extraPushes := hegel.Draw(ht, hegel.Integers(0, 6))
		for range extraPushes {
			pushes = append(pushes, resources[hegel.Draw(ht, hegel.Integers(0, len(resources)-1))])
		}

		var eventSequence atomic.Int64
		var totalInFlight atomic.Int32
		var maxTotalInFlight atomic.Int32
		var mu sync.Mutex
		perResourceInFlight := make(map[Resource]int)
		maxPerResourceInFlight := make(map[Resource]int)
		starts := make(map[Resource][]int64)
		lastPushReturn := make(map[Resource]int64)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		pool := NewWorkerPool(ctx, func(_ context.Context, resource Resource) {
			startEvent := eventSequence.Add(1)
			current := totalInFlight.Add(1)
			for {
				previous := maxTotalInFlight.Load()
				if current <= previous || maxTotalInFlight.CompareAndSwap(previous, current) {
					break
				}
			}
			mu.Lock()
			perResourceInFlight[resource]++
			if perResourceInFlight[resource] > maxPerResourceInFlight[resource] {
				maxPerResourceInFlight[resource] = perResourceInFlight[resource]
			}
			starts[resource] = append(starts[resource], startEvent)
			mu.Unlock()

			time.Sleep(workDelays[resource])

			mu.Lock()
			perResourceInFlight[resource]--
			mu.Unlock()
			totalInFlight.Add(-1)
		}, maxWorkers)

		pushed := make(map[Resource]bool)
		for _, resource := range pushes {
			pool.Push(resource)
			returnEvent := eventSequence.Add(1)
			mu.Lock()
			lastPushReturn[resource] = returnEvent
			pushed[resource] = true
			mu.Unlock()
			switch hegel.Draw(ht, hegel.Integers(0, 2)) {
			case 0:
				runtime.Gosched()
			case 1:
				time.Sleep(time.Duration(hegel.Draw(ht, hegel.Integers(0, 100))) * time.Microsecond)
			}
		}

		deadline := time.Now().Add(30 * time.Millisecond)
		idle := false
		for time.Now().Before(deadline) {
			pool.lock.Lock()
			idle = pool.q.Length() == 0 && pool.workerCount == 0
			pool.lock.Unlock()
			if idle && totalInFlight.Load() == 0 {
				break
			}
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		if !idle || totalInFlight.Load() != 0 {
			ht.Fatal("worker pool did not become idle before timeout")
		}
		mu.Lock()
		defer mu.Unlock()
		if got := maxTotalInFlight.Load(); got > int32(maxWorkers) {
			ht.Fatalf("total concurrent work=%d exceeds maxWorkers=%d", got, maxWorkers)
		}
		for resource := range pushed {
			if maxPerResourceInFlight[resource] > 1 {
				ht.Fatalf("resource %v had %d concurrent work calls", resource, maxPerResourceInFlight[resource])
			}
			startedAfterLastPush := false
			for _, start := range starts[resource] {
				if start > lastPushReturn[resource] {
					startedAfterLastPush = true
					break
				}
			}
			if !startedAfterLastPush {
				ht.Fatalf("resource %v has no work start after last Push returned (starts=%v lastPushReturn=%d)",
					resource, starts[resource], lastPushReturn[resource])
			}
		}
	}, hegel.WithTestCases(20))
}
