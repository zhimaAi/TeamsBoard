package bootstrap

import (
	"context"
	"database/sql"
	"time"

	"goteams-client/internal/applog"
	"goteams-client/internal/cloud"
	"goteams-client/internal/executor"
	"goteams-client/internal/executor/claude"
	"goteams-client/internal/executor/codebuddy"
	"goteams-client/internal/executor/codex"
	"goteams-client/internal/executor/copilot"
	"goteams-client/internal/executor/cursor"
	"goteams-client/internal/executor/grok"
	"goteams-client/internal/executor/hermes"
	"goteams-client/internal/executor/kimi"
	"goteams-client/internal/executor/openclaw"
	"goteams-client/internal/executor/opencode"
	"goteams-client/internal/executor/pi"
	"goteams-client/internal/executor/qoder"
	"goteams-client/internal/executor/qwen"
	"goteams-client/internal/localserver"
	"goteams-client/internal/pipeline"
	"goteams-client/internal/storage"
	storagelog "goteams-client/internal/storage/log"
	"goteams-client/internal/workflow"
)

// LocalRuntime owns all local task components for the lifetime of the process.
// It is intentionally independent from the optional cloud account session.
type LocalRuntime struct {
	manager      *storage.Manager
	DB           *sql.DB
	EventStore   *storagelog.EventStore
	Orchestrator *workflow.Orchestrator
	WSHub        *localserver.WSHub
}

func NewLocalRuntime(ctx context.Context, dataDir string, cloudClient *cloud.Client) (*LocalRuntime, error) {
	manager, err := storage.NewManagerForRole(dataDir, storage.DatabaseGoTeams)
	if err != nil {
		return nil, err
	}
	if err := manager.Migrate(ctx); err != nil {
		manager.Close()
		return nil, err
	}

	db := manager.DB()
	// 旧版本曾把云端流水线缓存进 gt_pipelines；现在云端流水线仅存内存，启动时清掉历史残留。
	// 清理失败不阻塞启动：List/Get 只读本地来源，残留行不会对外暴露。
	if err := pipeline.CleanupLegacyCloudPipelines(ctx, db); err != nil {
		applog.Warn("清理历史云端流水线本地缓存失败", "error", err)
	}
	eventStore := storagelog.NewEventStore(db)
	registerExecutorFactories()
	orchestrator := workflow.NewOrchestrator(db, db, eventStore)
	wsHub := localserver.NewWSHub(db)
	orchestrator.SetEventBroadcaster(func(sessionUUID string, sequence int, event executor.ExecutorEvent) {
		wsHub.BroadcastEvent(sessionUUID, sequence, event)
	})
	orchestrator.SetStateBroadcaster(func(taskUUID, stepKey, sessionUUID, status string) {
		wsHub.BroadcastStateChanged(taskUUID, stepKey, sessionUUID, status)
	})
	orchestrator.SetActivityBroadcaster(func(taskUUID, sessionUUID, eventType, content string, at int64) {
		wsHub.BroadcastActivity(taskUUID, sessionUUID, eventType, content, at)
	})
	wsHub.RegisterPushFunc(func() (string, interface{}) {
		count, err := orchestrator.GlobalRunningSessionCount()
		if err != nil {
			return "", nil
		}
		return "app.cli_count", map[string]interface{}{"count": count}
	})
	wsHub.RegisterPushFunc(func() (string, interface{}) {
		var unread int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gt_task_notifications WHERE is_archived = 0 AND is_read = 0`).Scan(&unread); err != nil {
			return "", nil
		}
		return "app.notifications.unread", map[string]interface{}{"count": unread}
	})
	wsHub.StartPushLoop(5 * time.Second)

	runtime := &LocalRuntime{
		manager: manager, DB: db, EventStore: eventStore,
		Orchestrator: orchestrator, WSHub: wsHub,
	}
	go func() {
		if err := orchestrator.HandleCrashRecovery(); err != nil {
			applog.Error("崩溃恢复失败", "error", err)
		}
	}()
	return runtime, nil
}

func (r *LocalRuntime) Close() {
	if r == nil {
		return
	}
	if r.WSHub != nil {
		r.WSHub.Close()
	}
	if r.manager != nil {
		r.manager.Close()
	}
}

func registerExecutorFactories() {
	workflow.SetCodexAdapterFactory(func() executor.Adapter { return codex.NewAdapter() })
	workflow.SetClaudeAdapterFactory(func() executor.Adapter { return claude.NewAdapter() })
	workflow.SetCodeBuddyAdapterFactory(func() executor.Adapter { return codebuddy.NewAdapter() })
	workflow.SetOpenCodeAdapterFactory(func() executor.Adapter { return opencode.NewAdapter() })
	workflow.SetCursorAdapterFactory(func() executor.Adapter { return cursor.NewAdapter() })
	workflow.SetCopilotAdapterFactory(func() executor.Adapter { return copilot.NewAdapter() })
	workflow.SetGrokAdapterFactory(func() executor.Adapter { return grok.NewAdapter() })
	workflow.SetHermesAdapterFactory(func() executor.Adapter { return hermes.NewAdapter() })
	workflow.SetKimiAdapterFactory(func() executor.Adapter { return kimi.NewAdapter() })
	workflow.SetQoderAdapterFactory(func() executor.Adapter { return qoder.NewAdapter() })
	workflow.SetQwenAdapterFactory(func() executor.Adapter { return qwen.NewAdapter() })
	workflow.SetOpenClawAdapterFactory(func() executor.Adapter { return openclaw.NewAdapter() })
	workflow.SetPiAdapterFactory(func() executor.Adapter { return pi.NewAdapter() })
}
