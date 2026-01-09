package scheduler

import (
	"context"
	"errors"
	"fmt"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"

	"telegram-auto-checkin/internal/client"
	"telegram-auto-checkin/internal/config"
	"telegram-auto-checkin/internal/executor"
)

type Scheduler struct {
	cron *cron.Cron
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		cron: cron.New(),
	}
}

func (s *Scheduler) AddTask(schedule string, task func()) error {
	_, err := s.cron.AddFunc(schedule, task)
	return err
}

func (s *Scheduler) Start() {
	s.cron.Start()
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}

type taskClient interface {
	CheckInMessage(ctx context.Context, target string, message string) error
	CheckInButton(ctx context.Context, target string, buttonText string) error
	Auth(ctx context.Context, phone, password string) error
	Run(ctx context.Context, fn func(ctx context.Context) error) error
	AuthInRun(ctx context.Context, phone, password string) error
	CheckInMessageInRun(ctx context.Context, target string, message string) error
	CheckInButtonInRun(ctx context.Context, target string, buttonText string) error
	CheckInMessageInRunWithLogger(ctx context.Context, target string, message string, taskLogger zerolog.Logger) error
	CheckInButtonInRunWithLogger(ctx context.Context, target string, buttonText string, taskLogger zerolog.Logger) error
}

type clientFactory func(appID int, appHash string, sessionName string, log zerolog.Logger, replyWaitSeconds, replyHistoryLimit int) (taskClient, error)

func isTaskEnabled(task config.TaskConfig) bool {
	if task.Enabled == nil {
		return true
	}
	return *task.Enabled
}

func formatAccountLabel(acc config.AccountConfig, sessionName string) string {
	if acc.Name != "" && acc.Phone != "" {
		return fmt.Sprintf("%s(%s)", acc.Name, acc.Phone)
	}
	if acc.Name != "" {
		return acc.Name
	}
	if acc.Phone != "" {
		return acc.Phone
	}
	if sessionName != "" {
		return sessionName
	}
	return "unknown_account"
}

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

func RunTasksOnce(ctx context.Context, cfg *config.Config, log zerolog.Logger) error {
	factory := func(appID int, appHash string, sessionFile string, log zerolog.Logger, replyWaitSeconds, replyHistoryLimit int) (taskClient, error) {
		return client.NewClient(appID, appHash, sessionFile, cfg.Proxy, log, replyWaitSeconds, replyHistoryLimit)
	}
	return runTasksOnce(ctx, cfg, log, factory)
}

func runTasksOnce(ctx context.Context, cfg *config.Config, log zerolog.Logger, factory clientFactory) error {
	var allErrs []error

	for _, acc := range cfg.Accounts {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		sessionName := acc.Phone
		if sessionName == "" {
			sessionName = fmt.Sprintf("session_%d", acc.AppID)
		}

		// Session file name
		sessionFile := sessionName + ".session"

		accountLabel := formatAccountLabel(acc, sessionName)
		accLog := log.With().Str("account", accountLabel).Str("session", sessionName).Logger()

		// Count enabled tasks
		enabledTaskCount := 0
		for _, task := range acc.Tasks {
			if isTaskEnabled(task) {
				enabledTaskCount++
			}
		}

		if enabledTaskCount == 0 {
			accLog.Info().Msg("No enabled tasks, skipping")
			continue
		}

		accLog.Info().Int("task_count", enabledTaskCount).Msg("Starting tasks")
		appID, appHash, err := resolveAppConfig(cfg, acc)
		if err != nil {
			accLog.Error().Err(err).Msg("Account configuration incomplete")
			allErrs = append(allErrs, err)
			continue
		}

		replyWaitSeconds, replyHistoryLimit := resolveReplyConfig(cfg, acc, config.TaskConfig{})

		client, err := factory(appID, appHash, sessionFile, accLog, replyWaitSeconds, replyHistoryLimit)
		if err != nil {
			accLog.Error().Err(err).Msg("Failed to create client")
			allErrs = append(allErrs, err)
			continue
		}

		// Execute all tasks within long-running Run session
		err = client.Run(ctx, func(ctx context.Context) error {
			if err := client.AuthInRun(ctx, acc.Phone, acc.Password); err != nil {
				accLog.Error().Err(err).Msg("Account authentication failed")
				return err
			}

			// Create task executor
			workerCount := acc.WorkerCount
			if workerCount <= 0 {
				workerCount = 4
			}
			queueSize := acc.TaskQueueSize
			if queueSize <= 0 {
				queueSize = 100
			}

			exec := executor.NewTaskExecutor(client, workerCount, queueSize, accLog, cfg.Log.Dir, cfg.Log.Format, accountLabel)
			exec.Start(ctx)
			defer exec.Stop()

			// Submit all tasks to executor
			taskErrors := make([]error, 0)
			for _, task := range acc.Tasks {
				if !isTaskEnabled(task) {
					continue
				}

				// Block and submit task
				if !exec.SubmitTaskBlocking(ctx, task, accLog, "once") {
					taskErrors = append(taskErrors, fmt.Errorf("failed to submit task: %s", task.Name))
				}
			}

			if len(taskErrors) > 0 {
				allErrs = append(allErrs, taskErrors...)
				accLog.Warn().Int("failed_count", len(taskErrors)).Int("total_count", enabledTaskCount).Msg("Some tasks failed")
			} else {
				accLog.Info().Int("total_count", enabledTaskCount).Msg("All tasks completed")
			}

			return nil
		})
		if err != nil {
			allErrs = append(allErrs, err)
		}
	}

	return errors.Join(allErrs...)
}

func RunTasks(ctx context.Context, cfg *config.Config, log zerolog.Logger) error {
	s := NewScheduler()
	hasAnyScheduled := false
	factory := func(appID int, appHash string, sessionFile string, log zerolog.Logger, replyWaitSeconds, replyHistoryLimit int) (taskClient, error) {
		return client.NewClient(appID, appHash, sessionFile, cfg.Proxy, log, replyWaitSeconds, replyHistoryLimit)
	}

	for _, acc := range cfg.Accounts {
		sessionName := acc.Phone
		if sessionName == "" {
			sessionName = fmt.Sprintf("session_%d", acc.AppID)
		}

		// Session file name
		sessionFile := sessionName + ".session"

		accountLabel := formatAccountLabel(acc, sessionName)
		accLog := log.With().Str("account", accountLabel).Str("session", sessionName).Logger()

		hasImmediateTasks := false
		hasScheduledTasks := false
		for _, task := range acc.Tasks {
			if !isTaskEnabled(task) {
				continue
			}
			if task.RunOnStart {
				hasImmediateTasks = true
			}
			if task.Schedule != "" {
				hasScheduledTasks = true
			}
		}

		if !hasImmediateTasks && !hasScheduledTasks {
			accLog.Info().Msg("No runnable tasks configured, skipping account")
			continue
		}

		appID, appHash, err := resolveAppConfig(cfg, acc)
		if err != nil {
			accLog.Error().Err(err).Msg("Account configuration incomplete")
			continue
		}

		replyWaitSeconds, replyHistoryLimit := resolveReplyConfig(cfg, acc, config.TaskConfig{})

		client, err := factory(appID, appHash, sessionFile, accLog, replyWaitSeconds, replyHistoryLimit)
		if err != nil {
			accLog.Error().Err(err).Msg("Failed to create client")
			continue
		}

		// Mark if there are scheduled tasks (before starting goroutine)
		if hasScheduledTasks {
			hasAnyScheduled = true
		}

		// Start long-running client.Run() session
		go client.Run(ctx, func(ctx context.Context) error {
			// Login authentication
			if err := client.AuthInRun(ctx, acc.Phone, acc.Password); err != nil {
				accLog.Error().Err(err).Msg("Account authentication failed")
				return err
			}

			// Create task executor
			workerCount := acc.WorkerCount
			if workerCount <= 0 {
				workerCount = 4
			}
			queueSize := acc.TaskQueueSize
			if queueSize <= 0 {
				queueSize = 100
			}

			exec := executor.NewTaskExecutor(client, workerCount, queueSize, accLog, cfg.Log.Dir, cfg.Log.Format, accountLabel)
			exec.Start(ctx)
			defer exec.Stop()

			// Execute run_on_start tasks
			if hasImmediateTasks {
				for _, task := range acc.Tasks {
					if isTaskEnabled(task) && task.RunOnStart {
						exec.SubmitTask(task, accLog, "run_on_start")
					}
				}
			}

			// Add scheduled tasks to scheduler
			if hasScheduledTasks {
				for _, task := range acc.Tasks {
					if !isTaskEnabled(task) || task.Schedule == "" {
						continue
					}

					t := task // copy
					taskName := t.Name
					if taskName == "" {
						taskName = t.Target
					}

					err := s.AddTask(t.Schedule, func() {
						select {
						case <-ctx.Done():
							return
						default:
						}
						// Submit to executor queue
						exec.SubmitTask(t, accLog, "scheduled")
					})

					if err != nil {
						accLog.Error().Err(err).Str("schedule", t.Schedule).Msg("Failed to add scheduled task")
						return err
					} else {
						accLog.Debug().Str("schedule", t.Schedule).Str("task", taskName).Str("target", t.Target).Msg("📅 Scheduled task added")
					}
				}
			}

			// Keep session running
			<-ctx.Done()
			return nil
		})
	}

	if !hasAnyScheduled {
		log.Info().Msg("No scheduled tasks, scheduler not started")
		return nil
	}

	s.Start()
	log.Info().Msg("Scheduler started")
	return nil
}

func resolveAppConfig(cfg *config.Config, acc config.AccountConfig) (int, string, error) {
	appID := acc.AppID
	appHash := acc.AppHash
	if appID == 0 {
		appID = cfg.AppID
	}
	if appHash == "" {
		appHash = cfg.AppHash
	}
	if appID == 0 || appHash == "" {
		return 0, "", fmt.Errorf("missing app_id or app_hash")
	}
	return appID, appHash, nil
}

// resolveReplyConfig resolves reply config parameters, priority: task > account > global > default
func resolveReplyConfig(cfg *config.Config, acc config.AccountConfig, task config.TaskConfig) (replyWaitSeconds, replyHistoryLimit int) {
	// Default values
	replyWaitSeconds = 3
	replyHistoryLimit = 10

	// Global config
	if cfg.ReplyWaitSeconds > 0 {
		replyWaitSeconds = cfg.ReplyWaitSeconds
	}
	if cfg.ReplyHistoryLimit > 0 {
		replyHistoryLimit = cfg.ReplyHistoryLimit
	}

	// Account level config
	if acc.ReplyWaitSeconds > 0 {
		replyWaitSeconds = acc.ReplyWaitSeconds
	}
	if acc.ReplyHistoryLimit > 0 {
		replyHistoryLimit = acc.ReplyHistoryLimit
	}

	// Task level config
	if task.ReplyWaitSeconds > 0 {
		replyWaitSeconds = task.ReplyWaitSeconds
	}
	if task.ReplyHistoryLimit > 0 {
		replyHistoryLimit = task.ReplyHistoryLimit
	}

	return replyWaitSeconds, replyHistoryLimit
}
