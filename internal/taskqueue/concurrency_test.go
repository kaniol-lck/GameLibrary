package taskqueue

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestConcurrentWorkersRunTasksInParallel(t *testing.T) {
	release := make(chan struct{})
	var started sync.WaitGroup

	queue := New(func(ctx context.Context, _ *Task) {
		started.Done()
		select {
		case <-release:
		case <-ctx.Done():
		}
	})
	defer queue.Stop()

	queue.SetConcurrency(4)
	if got := queue.Concurrency(); got != 4 {
		t.Fatalf("expected a pool of 4, got %d", got)
	}
	started.Add(4)

	for i := 0; i < 4; i++ {
		queue.Submit(&Task{Type: TaskScrape, GameID: fmt.Sprintf("g%d", i), Title: fmt.Sprintf("Game %d", i)})
	}

	// All four workers must be busy at once; with a single worker this would block
	// on the third Done call.
	done := make(chan struct{})
	go func() {
		started.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tasks did not start in parallel")
	}

	status := queue.Status()
	if status.Running != 4 {
		t.Errorf("expected 4 running tasks, got %d", status.Running)
	}
	if len(status.RunningTitles) != 4 {
		t.Errorf("expected 4 running titles, got %v", status.RunningTitles)
	}
	if status.Concurrency != 4 {
		t.Errorf("expected the status to report the pool size, got %d", status.Concurrency)
	}

	close(release)
	waitFor(t, 5*time.Second, func() bool { return queue.Status().Completed == 4 }, "all tasks to finish")
}

func TestConcurrencyIsClamped(t *testing.T) {
	queue := New(func(context.Context, *Task) {})
	defer queue.Stop()

	queue.SetConcurrency(0)
	if got := queue.Concurrency(); got != 1 {
		t.Errorf("expected a minimum of 1, got %d", got)
	}
	queue.SetConcurrency(-5)
	if got := queue.Concurrency(); got != 1 {
		t.Errorf("expected a minimum of 1, got %d", got)
	}
	queue.SetConcurrency(999)
	if got := queue.Concurrency(); got != maxConcurrency {
		t.Errorf("expected the maximum of %d, got %d", maxConcurrency, got)
	}
}

// TestBatchProgressAccounting covers the numbers the task list displays.
func TestBatchProgressAccounting(t *testing.T) {
	queue := New(func(_ context.Context, task *Task) {
		if task.GameID == "bad" {
			task.Error = "no metadata source matched this game"
		}
	})
	defer queue.Stop()
	queue.SetConcurrency(2)

	for _, id := range []string{"a", "bad", "c"} {
		queue.Submit(&Task{Type: TaskScrape, GameID: id, Title: id})
	}

	waitFor(t, 5*time.Second, func() bool {
		status := queue.Status()
		return status.Pending == 0 && status.Running == 0
	}, "the batch to drain")

	status := queue.Status()
	if status.Completed != 2 {
		t.Errorf("expected 2 completed, got %d", status.Completed)
	}
	if status.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", status.Failed)
	}
	if status.Total != 3 {
		t.Errorf("expected a total of 3, got %d", status.Total)
	}
	if len(status.Failures) != 1 {
		t.Fatalf("expected one recorded failure, got %v", status.Failures)
	}
	if status.Failures[0].GameID != "bad" {
		t.Errorf("unexpected failure game id %q", status.Failures[0].GameID)
	}
	if status.Failures[0].Error != "no metadata source matched this game" {
		t.Errorf("unexpected failure error %q", status.Failures[0].Error)
	}
}

// TestProgressResetsForANewBatch keeps the counters meaningful across runs.
func TestProgressResetsForANewBatch(t *testing.T) {
	queue := New(func(context.Context, *Task) {})
	defer queue.Stop()

	queue.ResetProgress()
	queue.Submit(&Task{Type: TaskScrape, GameID: "a", Title: "a"})
	waitFor(t, 5*time.Second, func() bool { return queue.Status().Completed == 1 }, "the first batch")

	// A second batch starts from zero rather than inheriting the first one's count.
	queue.ResetProgress()
	if status := queue.Status(); status.Completed != 0 || status.Total != 0 {
		t.Fatalf("expected the counters to be cleared, got %+v", status)
	}

	queue.Submit(&Task{Type: TaskScrape, GameID: "b", Title: "b"})
	waitFor(t, 5*time.Second, func() bool { return queue.Status().Completed == 1 }, "the second batch to finish")

	status := queue.Status()
	if status.Completed != 1 {
		t.Errorf("expected one completion for the new batch, got %d", status.Completed)
	}
	if status.Total != 1 {
		t.Errorf("expected a total of 1 for the new batch, got %d", status.Total)
	}
}

// TestFailuresAreCapped keeps the payload handed to the UI bounded.
func TestFailuresAreCapped(t *testing.T) {
	queue := New(func(_ context.Context, task *Task) { task.Error = "boom" })
	defer queue.Stop()
	queue.SetConcurrency(4)
	queue.ResetProgress()

	for i := 0; i < maxFailures+5; i++ {
		queue.Submit(&Task{Type: TaskScrape, GameID: fmt.Sprintf("g%d", i), Title: fmt.Sprintf("g%d", i)})
	}
	waitFor(t, 10*time.Second, func() bool {
		status := queue.Status()
		return status.Pending == 0 && status.Running == 0
	}, "every task to fail")

	status := queue.Status()
	if len(status.Failures) != maxFailures {
		t.Errorf("expected the failure list to be capped at %d, got %d", maxFailures, len(status.Failures))
	}
	if status.Failed != maxFailures+5 {
		t.Errorf("expected every failure to be counted, got %d", status.Failed)
	}
}

// TestSetConcurrencyAfterStopIsIgnored keeps a stopped queue from growing its pool.
//
// Adding to a WaitGroup while another goroutine waits on it is a misuse that
// panics, and Stop waits on the same group, so a late reconfiguration must not
// start workers.
func TestSetConcurrencyAfterStopIsIgnored(t *testing.T) {
	queue := New(func(context.Context, *Task) {})
	queue.Stop()

	// Must not panic, and must not revive the queue.
	queue.SetConcurrency(8)
	queue.SetConcurrency(0)

	if queue.Submit(&Task{Type: TaskScrape, GameID: "g1", Title: "First"}) {
		t.Error("a stopped queue must not accept work after reconfiguration")
	}
	queue.Stop() // idempotent
}

// TestConcurrentQueuesAreRaceFree is only meaningful under -race: several workers
// mutating and reading the same bookkeeping at once.
func TestConcurrentQueuesAreRaceFree(t *testing.T) {
	queue := New(func(ctx context.Context, _ *Task) {})
	defer queue.Stop()
	queue.SetConcurrency(8)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = queue.Status()
				_ = queue.Concurrency()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			queue.Submit(&Task{Type: TaskScrape, GameID: fmt.Sprintf("g%d", i), Title: "x"})
		}
	}()

	waitFor(t, 10*time.Second, func() bool {
		s := queue.Status()
		return s.Pending == 0 && s.Running == 0
	}, "the queue to drain")

	close(stop)
	wg.Wait()
}
