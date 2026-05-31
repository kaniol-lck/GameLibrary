package taskqueue

import (
	"sync"
	"time"

	"GameLibrary/internal/logger"
)

type TaskType string

const (
	TaskScrape TaskType = "scrape"
)

type TaskStatus string

const (
	StatusPending TaskStatus = "pending"
	StatusRunning TaskStatus = "running"
	StatusDone    TaskStatus = "done"
	StatusError   TaskStatus = "error"
)

type Task struct {
	Type     TaskType   `json:"type"`
	GameID   string     `json:"gameId"`
	Title    string     `json:"title,omitempty"`
	Status   TaskStatus `json:"status"`
	Error    string     `json:"error,omitempty"`
	Priority int        `json:"-"`
}

type ProgressCallback func(current, total int, task *Task)

type Queue struct {
	mu       sync.Mutex
	tasks    []*Task
	notEmpty chan struct{}
	stopped  chan struct{}
	paused   bool
	callback ProgressCallback
	worker   func(*Task)
	wg       sync.WaitGroup
}

func New(worker func(*Task), cb ProgressCallback) *Queue {
	q := &Queue{
		tasks:    make([]*Task, 0),
		notEmpty: make(chan struct{}, 1),
		stopped:  make(chan struct{}),
		callback: cb,
		worker:   worker,
	}
	q.wg.Add(1)
	go q.run()
	return q
}

func (q *Queue) Pause() {
	q.mu.Lock()
	q.paused = true
	q.mu.Unlock()
}

func (q *Queue) Resume() {
	q.mu.Lock()
	q.paused = false
	q.mu.Unlock()
	select {
	case q.notEmpty <- struct{}{}:
	default:
	}
}

func (q *Queue) Clear() {
	q.mu.Lock()
	q.tasks = q.tasks[:0]
	q.mu.Unlock()
	if q.callback != nil {
		q.callback(0, 0, nil)
	}
}

func (q *Queue) Submit(task *Task) {
	q.mu.Lock()
	task.Status = StatusPending
	q.tasks = append(q.tasks, task)
	total := len(q.tasks)
	q.mu.Unlock()

	if q.callback != nil {
		q.callback(len(q.tasks), total, nil)
	}

	select {
	case q.notEmpty <- struct{}{}:
	default:
	}
}

func (q *Queue) Stop() {
	close(q.stopped)
	q.wg.Wait()
}

func (q *Queue) Status() (pending, running int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, t := range q.tasks {
		switch t.Status {
		case StatusPending:
			pending++
		case StatusRunning:
			running++
		}
	}
	return
}

func (q *Queue) run() {
	defer q.wg.Done()
	for {
		select {
		case <-q.stopped:
			return
		case <-q.notEmpty:
			q.processNext()
		}
	}
}

func (q *Queue) processNext() {
	q.mu.Lock()
	if q.paused || len(q.tasks) == 0 {
		q.mu.Unlock()
		return
	}
	task := q.tasks[0]
	q.tasks = q.tasks[1:]
	task.Status = StatusRunning
	total := len(q.tasks)
	q.mu.Unlock()

	logger.Info("queue: task started", "gameId", task.GameID, "type", task.Type)

	start := time.Now()

	if q.worker != nil {
		q.worker(task)
	}

	if task.Error != "" {
		task.Status = StatusError
		logger.Warn("queue: task failed", "gameId", task.GameID, "error", task.Error, "duration", time.Since(start))
	} else {
		task.Status = StatusDone
		logger.Info("queue: task completed", "gameId", task.GameID, "duration", time.Since(start))
	}

	if q.callback != nil {
		q.callback(0, total+1, task)
	}

	select {
	case q.notEmpty <- struct{}{}:
	default:
	}
}
