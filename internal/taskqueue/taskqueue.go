// Package taskqueue runs background work (currently metadata scraping).
//
// Work is executed by a pool of workers. The pool size is 1 unless the caller
// raises it, because serial execution is the safe default for code that has not
// thought about rate limits. The scraper is rate limited per host, so a
// library-wide scrape can safely run several games at once.
//
// The queue also owns the numbers the UI shows: what is running, what is waiting,
// how far the current batch has got, and which games failed and why. Reporting
// that from one place keeps the UI from having to reconstruct it from scattered
// events.
package taskqueue

import (
	"context"
	"errors"
	"sync"
	"time"

	"GameLibrary/internal/logger"
)

// TaskType identifies the kind of work a task represents.
type TaskType string

// TaskScrape fetches metadata for one game.
const TaskScrape TaskType = "scrape"

// TaskStatus is a task's lifecycle state.
type TaskStatus string

const (
	StatusPending TaskStatus = "pending"
	StatusRunning TaskStatus = "running"
	StatusDone    TaskStatus = "done"
	StatusError   TaskStatus = "error"
)

const (
	// maxFailures bounds the failure list handed to the UI. A scrape of an
	// unmatched collection can fail on many entries; only recent ones matter.
	maxFailures = 20
	// maxConcurrency bounds the pool regardless of configuration.
	maxConcurrency = 16
)

// Task is one unit of queued work.
type Task struct {
	Type   TaskType   `json:"type"`
	GameID string     `json:"gameId"`
	Title  string     `json:"title,omitempty"`
	Status TaskStatus `json:"status"`
	Error  string     `json:"error,omitempty"`

	// StartedAt and FinishedAt are set by the queue, for the elapsed time the task
	// list shows.
	StartedAt  time.Time `json:"-"`
	FinishedAt time.Time `json:"-"`
}

// Failure describes a task that ended in an error.
type Failure struct {
	GameID string `json:"gameId"`
	Title  string `json:"title,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Worker performs a task. It receives a context that is cancelled when the queue
// stops, so in-flight HTTP work is aborted rather than left hanging.
type Worker func(ctx context.Context, task *Task)

// Status is an immutable snapshot of the queue, suitable for handing to the UI.
type Status struct {
	Pending     int  `json:"pending"`
	Running     int  `json:"running"`
	Paused      bool `json:"paused"`
	Concurrency int  `json:"concurrency"`

	// CurrentTitle/CurrentGameID name one running task, for the compact indicator.
	CurrentTitle  string `json:"currentTitle,omitempty"`
	CurrentGameID string `json:"currentGameId,omitempty"`
	// RunningTitles lists every task in flight so the task list can show them all.
	RunningTitles []string `json:"runningTitles"`
	PendingTitles []string `json:"pendingTitles"`

	// Batch progress. Completed and Failed count finished tasks since the queue
	// last became idle; Total is their sum plus everything still outstanding.
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Total     int `json:"total"`

	// Failures lists recent errors, newest first.
	Failures []Failure `json:"failures"`
}

// runningEntry is a task in flight.
//
// The queue keeps its own copy of the display fields rather than reading them off
// the Task: the worker goroutine rewrites Task.Title and Task.Error when it
// finishes, and Status() must not read a field another goroutine is writing.
type runningEntry struct {
	task   *Task
	title  string
	gameID string
}

// Queue runs tasks on a worker pool.
type Queue struct {
	worker Worker

	mu          sync.Mutex
	tasks       []*Task
	running     []runningEntry
	paused      bool
	stopped     bool
	concurrency int
	started     int

	completed int
	failed    int
	failures  []Failure

	notify   chan struct{}
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	stopOnce sync.Once
	wg       sync.WaitGroup

	// onChange is called whenever the queue's observable state changes, and
	// onTaskDone when a task finishes (successfully or not).
	onChange   func(Status)
	onTaskDone func(*Task)
}

// New starts a queue with a single worker.
func New(worker Worker) *Queue {
	ctx, cancel := context.WithCancel(context.Background())
	q := &Queue{
		worker:      worker,
		notify:      make(chan struct{}, 256),
		done:        make(chan struct{}),
		ctx:         ctx,
		cancel:      cancel,
		concurrency: 1,
		failures:    []Failure{},
	}
	q.ensureWorkers()

	// A cheap ticker keeps the reported elapsed time fresh while work is in
	// flight, so a long task does not look stalled.
	q.wg.Add(1)
	go q.watchLoop()

	return q
}

// SetConcurrency grows the worker pool. Shrinking is deliberately unsupported:
// surplus workers idle on the wake channel and cost nothing, whereas cancelling
// one mid-flight would abandon a task.
func (q *Queue) SetConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	if n > maxConcurrency {
		n = maxConcurrency
	}
	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return
	}
	q.concurrency = n
	q.mu.Unlock()

	q.ensureWorkers()
	q.signalAll()
	q.emit()
}

// Concurrency reports the configured pool size.
func (q *Queue) Concurrency() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.concurrency
}

// ensureWorkers starts workers until the configured pool size is reached.
func (q *Queue) ensureWorkers() {
	q.mu.Lock()
	defer q.mu.Unlock()
	// Adding to the WaitGroup once Stop is waiting on it is a misuse that panics,
	// so a stopped queue never grows its pool.
	if q.stopped {
		return
	}
	for q.started < q.concurrency {
		q.started++
		q.wg.Add(1)
		go q.run()
	}
}

// SetObserver installs the state-change callbacks. Either may be nil.
func (q *Queue) SetObserver(onChange func(Status), onTaskDone func(*Task)) {
	q.mu.Lock()
	q.onChange = onChange
	q.onTaskDone = onTaskDone
	q.mu.Unlock()
	q.emit()
}

// Submit enqueues a task.
//
// A duplicate is rejected: submitting the same game twice while it is still
// pending or running would queue two identical scrapes, doubling the API traffic
// and listing the title twice.
func (q *Queue) Submit(task *Task) bool {
	if task == nil || task.GameID == "" {
		return false
	}

	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return false
	}
	for _, existing := range q.tasks {
		if existing.GameID == task.GameID && existing.Type == task.Type {
			q.mu.Unlock()
			logger.Debug("queue: duplicate task ignored", "gameId", task.GameID, "type", task.Type)
			return false
		}
	}
	for _, running := range q.running {
		if running.gameID == task.GameID && running.task.Type == task.Type {
			q.mu.Unlock()
			return false
		}
	}

	// The progress counters are deliberately not reset here. With a worker pool the
	// queue transiently empties between submissions, so inferring a "new batch"
	// from an empty queue reset the counters in the middle of a run. The caller
	// declares a batch explicitly with ResetProgress.
	task.Status = StatusPending
	if task.Title == "" {
		task.Title = task.GameID
	}
	q.tasks = append(q.tasks, task)
	pool := q.concurrency
	q.mu.Unlock()

	// Wake the whole pool so a batch fans out immediately instead of trickling
	// through one worker.
	q.signalN(pool)
	q.emit()
	return true
}

// ResetProgress clears the completed/failed counters and the failure list. It is
// called when a new batch is about to be enqueued, so the UI's progress figures
// describe that batch rather than everything since the application started.
func (q *Queue) ResetProgress() {
	q.mu.Lock()
	q.completed = 0
	q.failed = 0
	q.failures = q.failures[:0]
	q.mu.Unlock()
	q.emit()
}

// Pause stops new tasks from starting. Tasks already in flight always finish,
// because interrupting a scrape halfway would leave a partially written record.
func (q *Queue) Pause() {
	q.mu.Lock()
	q.paused = true
	q.mu.Unlock()
	q.emit()
}

// Resume allows queued tasks to start again.
func (q *Queue) Resume() {
	q.mu.Lock()
	q.paused = false
	q.mu.Unlock()
	q.signalAll()
	q.emit()
}

// Clear drops every pending task. Running tasks are unaffected.
func (q *Queue) Clear() {
	q.mu.Lock()
	kept := q.tasks[:0]
	for _, task := range q.tasks {
		if task.Status == StatusPending {
			continue
		}
		kept = append(kept, task)
	}
	q.tasks = kept
	q.mu.Unlock()
	q.emit()
}

// Status returns a snapshot of the queue.
func (q *Queue) Status() Status {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.statusLocked()
}

func (q *Queue) statusLocked() Status {
	status := Status{
		Paused:        q.paused,
		Concurrency:   q.concurrency,
		RunningTitles: []string{},
		PendingTitles: []string{},
		Completed:     q.completed,
		Failed:        q.failed,
		Failures:      append([]Failure{}, q.failures...),
	}

	for _, entry := range q.running {
		status.Running++
		status.RunningTitles = append(status.RunningTitles, entry.title)
	}
	for _, task := range q.tasks {
		if task.Status == StatusPending {
			status.Pending++
			status.PendingTitles = append(status.PendingTitles, task.Title)
		}
	}
	if len(q.running) > 0 {
		status.CurrentTitle = q.running[0].title
		status.CurrentGameID = q.running[0].gameID
	}
	status.Total = status.Pending + status.Running + status.Completed + status.Failed
	return status
}

// Pending reports whether any work remains.
func (q *Queue) Pending() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, task := range q.tasks {
		if task.Status == StatusPending {
			return true
		}
	}
	return false
}

// Stop cancels in-flight work and waits for every worker to exit. It is safe to
// call more than once.
func (q *Queue) Stop() {
	q.stopOnce.Do(func() {
		q.mu.Lock()
		q.stopped = true
		q.mu.Unlock()

		q.cancel()
		close(q.done)
		q.wg.Wait()
	})
}

// signal wakes one worker.
func (q *Queue) signal() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// signalN wakes up to n workers.
func (q *Queue) signalN(n int) {
	for i := 0; i < n; i++ {
		select {
		case q.notify <- struct{}{}:
		default:
			return
		}
	}
}

// signalAll wakes every worker in the pool.
func (q *Queue) signalAll() {
	q.mu.Lock()
	pool := q.concurrency
	q.mu.Unlock()
	q.signalN(pool)
}

// emit publishes the current status to the observer.
func (q *Queue) emit() {
	q.mu.Lock()
	onChange := q.onChange
	status := q.statusLocked()
	q.mu.Unlock()

	if onChange != nil {
		onChange(status)
	}
}

func (q *Queue) watchLoop() {
	defer q.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-q.done:
			return
		case <-ticker.C:
			q.mu.Lock()
			active := len(q.running) > 0
			q.mu.Unlock()
			if active {
				q.emit()
			}
		}
	}
}

func (q *Queue) run() {
	defer q.wg.Done()
	for {
		if q.processNext() {
			continue
		}
		select {
		case <-q.done:
			return
		case <-q.notify:
		}
	}
}

// processNext claims and runs the next pending task. It reports whether a task
// was handled.
func (q *Queue) processNext() bool {
	q.mu.Lock()
	if q.paused || q.stopped {
		q.mu.Unlock()
		return false
	}
	var task *Task
	for _, candidate := range q.tasks {
		if candidate.Status == StatusPending {
			candidate.Status = StatusRunning
			candidate.StartedAt = time.Now()
			task = candidate
			break
		}
	}
	if task == nil {
		q.mu.Unlock()
		return false
	}
	q.running = append(q.running, runningEntry{task: task, title: task.Title, gameID: task.GameID})
	q.mu.Unlock()

	q.emit()

	logger.QueueTaskStarted(task.GameID, string(task.Type))
	start := time.Now()

	if q.worker != nil {
		q.worker(q.ctx, task)
	}

	elapsed := time.Since(start)

	// The finish state is written under the lock. Status() walks the queue and
	// reads task.Status, so setting it here in the open would race with another
	// worker's snapshot — which is exactly what the race detector reported.
	q.mu.Lock()
	task.FinishedAt = time.Now()
	if task.Error != "" {
		task.Status = StatusError
	} else {
		task.Status = StatusDone
	}

	// Retire the task: keeping finished tasks in the slice made Status() depend on
	// how long the application had been running.
	for i, candidate := range q.tasks {
		if candidate == task {
			q.tasks = append(q.tasks[:i], q.tasks[i+1:]...)
			break
		}
	}
	for i, entry := range q.running {
		if entry.task == task {
			q.running = append(q.running[:i], q.running[i+1:]...)
			break
		}
	}
	if task.Status == StatusError {
		q.failed++
		q.failures = append([]Failure{{
			GameID: task.GameID,
			Title:  task.Title,
			Error:  task.Error,
		}}, q.failures...)
		if len(q.failures) > maxFailures {
			q.failures = q.failures[:maxFailures]
		}
	} else {
		q.completed++
	}
	onTaskDone := q.onTaskDone
	taskErr := errorOf(task)
	q.mu.Unlock()

	// Logging and callbacks run outside the lock, on a task no longer reachable
	// from the queue.
	logger.QueueTaskFinished(task.GameID, string(task.Type), taskErr, elapsed)

	if onTaskDone != nil {
		onTaskDone(task)
	}
	q.emit()

	// Let another worker pick up the next task.
	q.signal()
	return true
}

func errorOf(task *Task) error {
	if task.Error == "" {
		return nil
	}
	return errors.New(task.Error)
}
