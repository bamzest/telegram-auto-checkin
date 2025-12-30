package executor

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog"

	"telegram-auto-checkin/internal/config"
	"telegram-auto-checkin/internal/logger"
)

// taskClient defines the client interface
type taskClient interface {
	CheckInMessageInRun(ctx context.Context, target string, message string) error
	CheckInButtonInRun(ctx context.Context, target string, buttonText string) error
	// Add methods with logger parameter
	CheckInMessageInRunWithLogger(ctx context.Context, target string, message string, taskLogger zerolog.Logger) error
	CheckInButtonInRunWithLogger(ctx context.Context, target string, buttonText string, taskLogger zerolog.Logger) error
}

// TaskRequest Task request
type TaskRequest struct {
	Task        config.TaskConfig
	Logger      zerolog.Logger
	TriggerType string // "run_on_start" or "scheduled"
	WorkerID    int
}

// TaskExecutor manages concurrent worker pool
type TaskExecutor struct {
	client      taskClient
	taskQueue   chan TaskRequest
	workerCount int
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	log         zerolog.Logger
	logDir      string // Log directory
	logFormat   string // Log format
	accountName string // Account name
}

// NewTaskExecutor creates task executor
func NewTaskExecutor(client taskClient, workerCount, queueSize int, log zerolog.Logger, logDir, logFormat, accountName string) *TaskExecutor {
	if workerCount <= 0 {
		workerCount = 4 // default 4 workers
	}
	if queueSize <= 0 {
		queueSize = 100 // default queue size 100
	}
	if logFormat == "" {
		logFormat = "text" // default text format
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TaskExecutor{
		client:      client,
		taskQueue:   make(chan TaskRequest, queueSize),
		workerCount: workerCount,
		ctx:         ctx,
		cancel:      cancel,
		log:         log,
		logDir:      logDir,
		logFormat:   logFormat,
		accountName: accountName,
	}
}

// Start starts the worker pool (called within client.Run session)
func (e *TaskExecutor) Start(ctx context.Context) {
	e.log.Debug().Int("worker_count", e.workerCount).Msg("Starting task executor")

	for i := 0; i < e.workerCount; i++ {
		e.wg.Add(1)
		go e.worker(ctx, i)
	}
}

// worker goroutine, executes tasks concurrently
func (e *TaskExecutor) worker(ctx context.Context, id int) {
	defer e.wg.Done()

	workerLog := e.log.With().Int("worker_id", id).Logger()
	workerLog.Debug().Msg("Worker started")

	for {
		select {
		case <-ctx.Done():
			workerLog.Debug().Msg("Worker exiting")
			return
		case <-e.ctx.Done():
			workerLog.Debug().Msg("Worker exiting")
			return
		case req, ok := <-e.taskQueue:
			if !ok {
				workerLog.Debug().Msg("Worker exiting")
				return
			}
			// Concurrent task execution is safe within the same client.Run() session
			req.WorkerID = id
			e.executeTask(ctx, req)
		}
	}
}

// executeTask executes a single task
func (e *TaskExecutor) executeTask(ctx context.Context, req TaskRequest) {
	taskName := req.Task.Name
	if taskName == "" {
		taskName = req.Task.Target
	}

	// Create separate log file for task
	taskLogger, logFile, err := logger.CreateTaskLogger(e.logDir, e.accountName, taskName, req.TriggerType, e.logFormat)
	if err != nil {
		e.log.Error().Err(err).Str("task", taskName).Msg("Failed to create task log file, using main log")
		taskLogger = req.Logger
	} else {
		defer logFile.Close()
	}

	taskLog := taskLogger.With().
		Int("thread_id", req.WorkerID).
		Str("thread_name", taskName).
		Str("task", taskName).
		Str("target", req.Task.Target).
		Logger()

	// Display different logs based on trigger type
	if req.TriggerType == "run_on_start" {
		taskLog.Info().Msg("Executing startup task...")
		req.Logger.Debug().
			Int("thread_id", req.WorkerID).
			Str("thread_name", taskName).
			Str("task", taskName).
			Msg("Executing startup task...")
	} else if req.TriggerType == "scheduled" {
		taskLog.Info().Msg("Executing scheduled task...")
		req.Logger.Debug().
			Int("thread_id", req.WorkerID).
			Str("thread_name", taskName).
			Str("task", taskName).
			Msg("Executing scheduled task...")
	} else {
		taskLog.Info().Msg("Executing task...")
		req.Logger.Debug().
			Int("thread_id", req.WorkerID).
			Str("thread_name", taskName).
			Str("task", taskName).
			Msg("Executing task...")
	}

	// Execute task directly, gotd library handles concurrency safety internally
	if err := executeTaskWithLogger(ctx, e.client, req.Task, taskLog); err != nil {
		if req.TriggerType == "run_on_start" {
			taskLog.Error().Err(err).Str("payload", req.Task.Payload).Msg("Startup task failed")
			req.Logger.Error().
				Err(err).
				Int("thread_id", req.WorkerID).
				Str("thread_name", taskName).
				Str("task", taskName).
				Str("payload", req.Task.Payload).
				Msg("Startup task failed")
		} else if req.TriggerType == "scheduled" {
			taskLog.Error().Err(err).Str("payload", req.Task.Payload).Msg("Scheduled task failed")
			req.Logger.Error().
				Err(err).
				Int("thread_id", req.WorkerID).
				Str("thread_name", taskName).
				Str("task", taskName).
				Str("payload", req.Task.Payload).
				Msg("Scheduled task failed")
		} else {
			taskLog.Error().Err(err).Str("payload", req.Task.Payload).Msg("Task failed")
			req.Logger.Error().
				Err(err).
				Int("thread_id", req.WorkerID).
				Str("thread_name", taskName).
				Str("task", taskName).
				Str("payload", req.Task.Payload).
				Msg("Task failed")
		}
	} else {
		taskLog.Info().Msg("Task completed successfully")
		req.Logger.Info().
			Int("thread_id", req.WorkerID).
			Str("thread_name", taskName).
			Str("task", taskName).
			Msg("Task completed successfully")
	}
}

// executeTask executes a single task
func executeTask(ctx context.Context, client taskClient, task config.TaskConfig) error {
	switch task.Method {
	case "message":
		return client.CheckInMessageInRun(ctx, task.Target, task.Payload)
	case "button":
		return client.CheckInButtonInRun(ctx, task.Target, task.Payload)
	default:
		return fmt.Errorf("unknown method %q", task.Method)
	}
}

// executeTaskWithLogger executes a single task (with task logger)
func executeTaskWithLogger(ctx context.Context, client taskClient, task config.TaskConfig, taskLogger zerolog.Logger) error {
	switch task.Method {
	case "message":
		return client.CheckInMessageInRunWithLogger(ctx, task.Target, task.Payload, taskLogger)
	case "button":
		return client.CheckInButtonInRunWithLogger(ctx, task.Target, task.Payload, taskLogger)
	default:
		return fmt.Errorf("unknown method %q", task.Method)
	}
}

// SubmitTask submits task to execution queue (non-blocking)
func (e *TaskExecutor) SubmitTask(task config.TaskConfig, logger zerolog.Logger, triggerType string) bool {
	select {
	case e.taskQueue <- TaskRequest{Task: task, Logger: logger, TriggerType: triggerType}:
		return true
	default:
		logger.Warn().Str("task", task.Name).Msg("⚠️ Task queue is full, dropping task")
		return false
	}
}

// SubmitTaskBlocking submits task to execution queue (blocking)
func (e *TaskExecutor) SubmitTaskBlocking(ctx context.Context, task config.TaskConfig, logger zerolog.Logger, triggerType string) bool {
	select {
	case <-ctx.Done():
		return false
	case e.taskQueue <- TaskRequest{Task: task, Logger: logger, TriggerType: triggerType}:
		return true
	}
}

// Stop stops the executor
func (e *TaskExecutor) Stop() {
	e.cancel()
	close(e.taskQueue)
	e.wg.Wait()
	e.log.Debug().Msg("Task executor stopped")
}

// QueueLen returns the queue length
func (e *TaskExecutor) QueueLen() int {
	return len(e.taskQueue)
}
