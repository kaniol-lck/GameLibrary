package taskqueue

import (
	"context"
	"sync"
	"testing"
	"time"
)

// recorder is a worker that records what it ran and can be held open so the
// queue's observable state is deterministic while a test asserts on it.
type recorder struct {
	mu    sync.Mutex
	ran   []string
	hold  chan struct{}
	block func(ctx context.Context, task *Task)
}

func newRecorder() *recorder {
	return &recorder{hold: make(chan struct{})}
}

func (r *recorder) worker(ctx context.Context, task *Task) {
	if r.block != nil {
		r.block(ctx, task)
	}
	select {
	case <-r.hold:
	case <-ctx.Done():
	}
	r.mu.Lock()
	r.ran = append(r.ran, task.GameID)
	r.mu.Unlock()
}

func (r *recorder) release() { close(r.hold) }

func (r *recorder) ranIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ran...)
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", message)
}

func TestSubmitsAndRunsTask(t *testing.T) {
	rec := newRecorder()
	queue := New(rec.worker)
	defer queue.Stop()

	done := make(chan *Task, 1)
	queue.SetObserver(nil, func(task *Task) { done <- task })

	if !queue.Submit(&Task{Type: TaskScrape, GameID: "g1", Title: "Game One"}) {
		t.Fatal("expected the task to be accepted")
	}
	rec.release()

	select {
	case task := <-done:
		if task.Status != StatusDone {
			t.Errorf("expected the task to be done, got %s", task.Status)
		}
		if task.GameID != "g1" {
			t.Errorf("unexpected game id %q", task.GameID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("task never completed")
	}
}

// TestDeduplicatesSubmittedTasks pins down a fix: the same game submitted twice
// while still pending or running used to queue two identical scrapes, which
// doubled the API traffic and listed the title twice in the UI.
func TestDeduplicatesSubmittedTasks(t *testing.T) {
	rec := newRecorder()
	queue := New(rec.worker)
	defer queue.Stop()

	if !queue.Submit(&Task{Type: TaskScrape, GameID: "g1", Title: "Game One"}) {
		t.Fatal("the first submission should be accepted")
	}
	if queue.Submit(&Task{Type: TaskScrape, GameID: "g1", Title: "Game One"}) {
		t.Fatal("a duplicate submission must be rejected")
	}
	// A different type for the same game is genuinely different work.
	if !queue.Submit(&Task{Type: "other", GameID: "g1", Title: "Game One"}) {
		t.Fatal("a task with a different type should be accepted")
	}

	rec.release()
	waitFor(t, 5*time.Second, func() bool { return len(rec.ranIDs()) == 2 }, "both tasks to run")

	// Once finished, the same task may be submitted again.
	waitFor(t, 5*time.Second, func() bool {
		return queue.Status().Pending == 0 && queue.Status().Running == 0
	}, "the queue to drain")

	if !queue.Submit(&Task{Type: TaskScrape, GameID: "g1", Title: "Game One"}) {
		t.Fatal("a resubmission after completion should be accepted")
	}
}

func TestSubmitRejectsInvalidTasks(t *testing.T) {
	queue := New(func(context.Context, *Task) {})
	defer queue.Stop()

	if queue.Submit(nil) {
		t.Error("a nil task must be rejected")
	}
	if queue.Submit(&Task{Type: TaskScrape}) {
		t.Error("a task without a game id must be rejected")
	}
}

func TestRunsInFIFOOrder(t *testing.T) {
	rec := newRecorder()
	queue := New(rec.worker)
	defer queue.Stop()

	for _, id := range []string{"a", "b", "c"} {
		queue.Submit(&Task{Type: TaskScrape, GameID: id, Title: id})
	}
	rec.release()

	waitFor(t, 5*time.Second, func() bool { return len(rec.ranIDs()) == 3 }, "all three tasks to run")
	got := rec.ranIDs()
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("expected FIFO order, got %v", got)
	}
}

func TestStatusReportsProgress(t *testing.T) {
	started := make(chan struct{}, 1)
	rec := newRecorder()
	rec.block = func(ctx context.Context, task *Task) {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	queue := New(rec.worker)
	defer queue.Stop()

	queue.Submit(&Task{Type: TaskScrape, GameID: "g1", Title: "First"})
	queue.Submit(&Task{Type: TaskScrape, GameID: "g2", Title: "Second"})
	queue.Submit(&Task{Type: TaskScrape, GameID: "g3", Title: "Third"})

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the first task never started")
	}

	// Let the queue settle with one task running and two pending.
	waitFor(t, 5*time.Second, func() bool {
		status := queue.Status()
		return status.Running == 1 && status.Pending == 2
	}, "one running and two pending tasks")

	status := queue.Status()
	if status.CurrentGameID != "g1" || status.CurrentTitle != "First" {
		t.Errorf("unexpected current task: %+v", status)
	}
	if len(status.PendingTitles) != 2 || status.PendingTitles[0] != "Second" {
		t.Errorf("unexpected pending titles: %v", status.PendingTitles)
	}
}

// TestFinishedTasksAreRetired pins down a fix: completed tasks used to stay in
// the queue's slice, so the reported counts depended on how long the app had been
// running rather than on what was outstanding.
func TestFinishedTasksAreRetired(t *testing.T) {
	rec := newRecorder()
	queue := New(rec.worker)
	defer queue.Stop()

	for _, id := range []string{"a", "b", "c"} {
		queue.Submit(&Task{Type: TaskScrape, GameID: id, Title: id})
	}
	rec.release()

	waitFor(t, 5*time.Second, func() bool { return len(rec.ranIDs()) == 3 }, "all tasks to run")
	waitFor(t, 5*time.Second, func() bool {
		status := queue.Status()
		return status.Pending == 0 && status.Running == 0 && len(status.PendingTitles) == 0
	}, "the queue to report nothing outstanding")

	status := queue.Status()
	if status.CurrentTitle != "" || status.CurrentGameID != "" {
		t.Errorf("expected no current task, got %+v", status)
	}
}

func TestPauseAndResume(t *testing.T) {
	rec := newRecorder()
	queue := New(rec.worker)
	defer queue.Stop()

	queue.Pause()
	queue.Submit(&Task{Type: TaskScrape, GameID: "g1", Title: "First"})

	// Give the worker a chance to (incorrectly) pick it up.
	time.Sleep(100 * time.Millisecond)
	if len(rec.ranIDs()) != 0 {
		t.Fatal("a paused queue must not start tasks")
	}
	if status := queue.Status(); !status.Paused || status.Pending != 1 {
		t.Fatalf("unexpected status while paused: %+v", status)
	}

	queue.Resume()
	rec.release()
	waitFor(t, 5*time.Second, func() bool { return len(rec.ranIDs()) == 1 }, "the task to run after resume")
}

func TestPauseLetsRunningTaskFinish(t *testing.T) {
	started := make(chan struct{}, 1)
	rec := newRecorder()
	rec.block = func(ctx context.Context, task *Task) {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	queue := New(rec.worker)
	defer queue.Stop()

	queue.Submit(&Task{Type: TaskScrape, GameID: "g1", Title: "First"})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the task never started")
	}

	queue.Pause()
	rec.release()
	// Interrupting a scrape halfway would leave a partially written record, so
	// the running task must be allowed to complete.
	waitFor(t, 5*time.Second, func() bool { return len(rec.ranIDs()) == 1 }, "the running task to finish")
}

func TestClearDropsOnlyPendingTasks(t *testing.T) {
	started := make(chan struct{}, 1)
	rec := newRecorder()
	rec.block = func(ctx context.Context, task *Task) {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	queue := New(rec.worker)
	defer queue.Stop()

	queue.Submit(&Task{Type: TaskScrape, GameID: "g1", Title: "First"})
	queue.Submit(&Task{Type: TaskScrape, GameID: "g2", Title: "Second"})
	queue.Submit(&Task{Type: TaskScrape, GameID: "g3", Title: "Third"})

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the first task never started")
	}
	waitFor(t, 5*time.Second, func() bool { return queue.Status().Running == 1 }, "a task to be running")

	queue.Clear()
	if status := queue.Status(); status.Pending != 0 {
		t.Fatalf("expected no pending tasks, got %+v", status)
	}

	rec.release()
	waitFor(t, 5*time.Second, func() bool { return len(rec.ranIDs()) == 1 }, "only the running task to finish")
}

func TestWorkerErrorIsReported(t *testing.T) {
	queue := New(func(_ context.Context, task *Task) {
		task.Error = "boom"
	})
	defer queue.Stop()

	done := make(chan *Task, 1)
	queue.SetObserver(nil, func(task *Task) { done <- task })
	queue.Submit(&Task{Type: TaskScrape, GameID: "g1", Title: "First"})

	select {
	case task := <-done:
		if task.Status != StatusError {
			t.Errorf("expected StatusError, got %s", task.Status)
		}
		if task.Error != "boom" {
			t.Errorf("expected the error to be preserved, got %q", task.Error)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the task never reported completion")
	}
}

// TestStopIsIdempotent pins down a fix: Stop used to close a channel on every
// call, so a second shutdown request panicked.
func TestStopIsIdempotent(t *testing.T) {
	queue := New(func(context.Context, *Task) {})

	queue.Submit(&Task{Type: TaskScrape, GameID: "g1", Title: "First"})
	queue.Stop()
	queue.Stop()
	queue.Stop()

	if queue.Submit(&Task{Type: TaskScrape, GameID: "g2", Title: "Second"}) {
		t.Fatal("a stopped queue must not accept new tasks")
	}
}

func TestStopCancelsWorkerContext(t *testing.T) {
	started := make(chan struct{}, 1)
	observed := make(chan struct{})
	queue := New(func(ctx context.Context, _ *Task) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		close(observed)
	})

	queue.Submit(&Task{Type: TaskScrape, GameID: "g1", Title: "First"})

	// The task has to be running before Stop is meaningful. A task that never
	// started is simply skipped, which is correct behaviour but is not what this
	// test is about.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the task never started")
	}

	done := make(chan struct{})
	go func() {
		queue.Stop()
		close(done)
	}()

	select {
	case <-observed:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker context was not cancelled")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return")
	}
}

func TestObserverReceivesStatusChanges(t *testing.T) {
	rec := newRecorder()
	queue := New(rec.worker)
	defer queue.Stop()

	var mu sync.Mutex
	var seen []Status
	queue.SetObserver(func(status Status) {
		mu.Lock()
		seen = append(seen, status)
		mu.Unlock()
	}, nil)

	queue.Submit(&Task{Type: TaskScrape, GameID: "g1", Title: "First"})
	rec.release()

	waitFor(t, 5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(seen) > 0
	}, "a status notification")

	// SetObserver emits the current state immediately, so the first notification
	// can legitimately be the initial empty one. What matters is that submitting a
	// task produced a notification reporting it as pending.
	waitFor(t, 5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, status := range seen {
			if status.Pending == 1 && status.PendingTitles[0] == "First" {
				return true
			}
		}
		return false
	}, "a notification reporting the task as pending")
}
