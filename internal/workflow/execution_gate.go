package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type taskExecutionGate struct {
	lock sync.RWMutex
	refs int
}

type taskExecutionGateRegistry struct {
	mu    sync.Mutex
	gates map[string]*taskExecutionGate
}

func newTaskExecutionGateRegistry() *taskExecutionGateRegistry {
	return &taskExecutionGateRegistry{gates: make(map[string]*taskExecutionGate)}
}

func (r *taskExecutionGateRegistry) acquire(taskUUID string, exclusive bool) func() {
	taskUUID = strings.TrimSpace(taskUUID)
	r.mu.Lock()
	gate := r.gates[taskUUID]
	if gate == nil {
		gate = &taskExecutionGate{}
		r.gates[taskUUID] = gate
	}
	gate.refs++
	r.mu.Unlock()
	if exclusive {
		gate.lock.Lock()
	} else {
		gate.lock.RLock()
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			if exclusive {
				gate.lock.Unlock()
			} else {
				gate.lock.RUnlock()
			}
			r.mu.Lock()
			gate.refs--
			if gate.refs == 0 && r.gates[taskUUID] == gate {
				delete(r.gates, taskUUID)
			}
			r.mu.Unlock()
		})
	}
}

// TaskExecutionSwitch owns the task's exclusive execution gate. Only this
// lease may stop the old sessions and create the first session of the new mode.
type TaskExecutionSwitch struct {
	orchestrator *Orchestrator
	taskUUID     string
	release      func()
}

// TaskExecution owns a shared task gate while a normal start operation first
// activates the step and then creates its Session.
type TaskExecution struct {
	orchestrator *Orchestrator
	taskUUID     string
	release      func()
}

func (o *Orchestrator) BeginTaskExecution(taskUUID string) (*TaskExecution, error) {
	taskUUID = strings.TrimSpace(taskUUID)
	if taskUUID == "" {
		return nil, fmt.Errorf("任务 UUID 不能为空")
	}
	return &TaskExecution{
		orchestrator: o,
		taskUUID:     taskUUID,
		release:      o.executionGates.acquire(taskUUID, false),
	}, nil
}

func (s *TaskExecution) Close() {
	if s == nil || s.release == nil {
		return
	}
	s.release()
	s.release = nil
}

func (s *TaskExecution) RunStep(ctx context.Context, opts RunStepOptions) (string, error) {
	if s == nil || s.orchestrator == nil || strings.TrimSpace(opts.TaskUUID) != s.taskUUID {
		return "", fmt.Errorf("任务执行上下文无效")
	}
	return s.orchestrator.runStep(ctx, opts)
}

func (s *TaskExecution) StopTaskSessions(ctx context.Context) error {
	if s == nil || s.orchestrator == nil {
		return fmt.Errorf("任务执行上下文无效")
	}
	return s.orchestrator.stopTaskSessions(ctx, s.taskUUID)
}

func (o *Orchestrator) BeginTaskExecutionSwitch(taskUUID string) (*TaskExecutionSwitch, error) {
	taskUUID = strings.TrimSpace(taskUUID)
	if taskUUID == "" {
		return nil, fmt.Errorf("任务 UUID 不能为空")
	}
	return &TaskExecutionSwitch{
		orchestrator: o,
		taskUUID:     taskUUID,
		release:      o.executionGates.acquire(taskUUID, true),
	}, nil
}

func (s *TaskExecutionSwitch) Close() {
	if s == nil || s.release == nil {
		return
	}
	s.release()
	s.release = nil
}

func (s *TaskExecutionSwitch) StopTaskSessions(ctx context.Context) error {
	if s == nil || s.orchestrator == nil {
		return fmt.Errorf("任务执行切换未初始化")
	}
	return s.orchestrator.stopTaskSessions(ctx, s.taskUUID)
}

func (s *TaskExecutionSwitch) RunStep(ctx context.Context, opts RunStepOptions) (string, error) {
	if s == nil || s.orchestrator == nil || strings.TrimSpace(opts.TaskUUID) != s.taskUUID {
		return "", fmt.Errorf("任务执行切换上下文无效")
	}
	return s.orchestrator.runStep(ctx, opts)
}
