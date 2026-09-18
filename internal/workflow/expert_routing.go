package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"goteams-client/internal/applog"
	"goteams-client/internal/taskruntime"
)

const expertChainMaxRounds = 20

const (
	expertDispatchCodeRoutingPending         = "expert_routing.routing_pending"
	expertDispatchCodeContextLoadFailed      = "expert_routing.context_load_failed"
	expertDispatchCodeLeaderFailed           = "expert_routing.leader_failed"
	expertDispatchCodeNoDelegation           = "expert_routing.no_delegation"
	expertDispatchCodeMultipleDelegations    = "expert_routing.multiple_delegations"
	expertDispatchCodeInvalidMember          = "expert_routing.invalid_member"
	expertDispatchCodeMemberActivationFailed = "expert_routing.member_activation_failed"
	expertDispatchCodeMemberStartFailed      = "expert_routing.member_start_failed"
	expertDispatchCodeDispatched             = "expert_routing.dispatched"
	expertDispatchCodeLeaderMissing          = "expert_routing.leader_missing"
	expertDispatchCodeLeaderActivationFailed = "expert_routing.leader_activation_failed"
	expertDispatchCodeLeaderStartFailed      = "expert_routing.leader_start_failed"
	expertDispatchCodeReturned               = "expert_routing.returned"
	expertDispatchCodeRoundLimit             = "expert_routing.round_limit"
)

var expertMentionPattern = regexp.MustCompile(`\[@[^\]]+\]\(mention://expert/([0-9a-fA-F-]{36})\)`)

func (o *Orchestrator) routeExpertCompletion(sessionUUID, taskUUID, status, finalResult string) {
	release := o.executionGates.acquire(taskUUID, false)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var mode, role, stepUUID, chainUUID string
	var chainRound int
	err := o.db.QueryRowContext(ctx, `SELECT execution_mode, COALESCE((SELECT member_role FROM gt_task_steps WHERE uuid=s.step_uuid),''),
		s.step_uuid, s.expert_chain_uuid, s.expert_chain_round FROM gt_cli_sessions s WHERE s.uuid=?`, sessionUUID).Scan(
		&mode, &role, &stepUUID, &chainUUID, &chainRound)
	if err == sql.ErrNoRows {
		// 执行方式切换会在独占门闩内删除旧 Session；等待门闩后的旧路由
		// 回调必须直接结束，不能再修改新执行方式的任务状态。
		return
	}
	if err != nil {
		applog.Warn("[ExpertGroup] 读取协作上下文失败", "session", sessionUUID, "error", err)
		o.recordExpertDispatch(ctx, sessionUUID, taskUUID, "failed", expertDispatchCodeContextLoadFailed)
		return
	}
	if mode != "expert_group" {
		return
	}
	if role == "leader" {
		if status != SessionStatusSuccess {
			o.recordExpertDispatch(ctx, sessionUUID, taskUUID, "blocked", expertDispatchCodeLeaderFailed)
			return
		}
		if !o.allowNextExpertRound(ctx, sessionUUID, taskUUID, chainRound) {
			return
		}
		matches := expertMentionPattern.FindAllStringSubmatch(finalResult, -1)
		if len(matches) == 0 {
			o.recordExpertDispatch(ctx, sessionUUID, taskUUID, "idle", expertDispatchCodeNoDelegation)
			return
		}
		if len(matches) != 1 {
			o.recordExpertDispatch(ctx, sessionUUID, taskUUID, "failed", expertDispatchCodeMultipleDelegations)
			return
		}
		targetStepUUID := normalizeExpertMentionUUID(matches[0][1])
		var targetRole string
		if err := o.db.QueryRowContext(ctx, `SELECT member_role FROM gt_task_steps WHERE task_uuid=? AND uuid=? AND execution_mode='expert_group'`,
			taskUUID, targetStepUUID).Scan(&targetRole); err != nil || targetRole != "member" {
			o.recordExpertDispatch(ctx, sessionUUID, taskUUID, "failed", expertDispatchCodeInvalidMember)
			return
		}
		if err := taskruntime.NewService(o.db, "").ActivateExpertMember(ctx, taskUUID, targetStepUUID); err != nil {
			applog.Warn("[ExpertGroup] 激活委派成员失败", "session", sessionUUID, "error", err)
			o.recordExpertDispatch(ctx, sessionUUID, taskUUID, "failed", expertDispatchCodeMemberActivationFailed)
			return
		}
		if _, err := o.runExpertTarget(ctx, taskUUID, targetStepUUID, finalResult, finalResult, false,
			chainUUID, chainRound+1, sessionUUID, ""); err != nil {
			applog.Warn("[ExpertGroup] 启动委派成员失败", "session", sessionUUID, "error", err)
			o.recordExpertDispatch(ctx, sessionUUID, taskUUID, "failed", expertDispatchCodeMemberStartFailed)
			return
		}
		o.recordExpertDispatch(ctx, sessionUUID, taskUUID, "dispatched", expertDispatchCodeDispatched)
		return
	}

	leaderStepUUID, err := taskruntime.NewService(o.db, "").ExpertLeaderStep(ctx, taskUUID)
	if err != nil || leaderStepUUID == "" {
		if err != nil {
			applog.Warn("[ExpertGroup] 读取团长快照失败", "session", sessionUUID, "error", err)
		}
		o.recordExpertDispatch(ctx, sessionUUID, taskUUID, "failed", expertDispatchCodeLeaderMissing)
		return
	}
	handoff := fmt.Sprintf("成员执行状态：%s\n\n成员结果：\n%s", status, strings.TrimSpace(finalResult))
	if err := taskruntime.NewService(o.db, "").ActivateExpertMember(ctx, taskUUID, leaderStepUUID); err != nil {
		applog.Warn("[ExpertGroup] 激活团长失败", "session", sessionUUID, "error", err)
		o.recordExpertDispatch(ctx, sessionUUID, taskUUID, "failed", expertDispatchCodeLeaderActivationFailed)
		return
	}
	if _, err := o.runExpertTarget(ctx, taskUUID, leaderStepUUID, handoff, handoff, false,
		chainUUID, chainRound+1, sessionUUID, ""); err != nil {
		applog.Warn("[ExpertGroup] 重新唤醒团长失败", "session", sessionUUID, "error", err)
		o.recordExpertDispatch(ctx, sessionUUID, taskUUID, "failed", expertDispatchCodeLeaderStartFailed)
		return
	}
	o.recordExpertDispatch(ctx, sessionUUID, taskUUID, "returned", expertDispatchCodeReturned)
}

func normalizeExpertMentionUUID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (o *Orchestrator) allowNextExpertRound(ctx context.Context, sessionUUID, taskUUID string, chainRound int) bool {
	if chainRound+1 <= expertChainMaxRounds {
		return true
	}
	o.recordExpertDispatch(ctx, sessionUUID, taskUUID, "paused", expertDispatchCodeRoundLimit)
	return false
}

func (o *Orchestrator) recordExpertDispatch(ctx context.Context, sessionUUID, taskUUID, dispatchStatus, messageCode string) {
	_, err := o.db.ExecContext(ctx, `UPDATE gt_task_progress SET dispatch_status=?, dispatch_message=? WHERE session_uuid=? AND record_type<>'user_question'`,
		dispatchStatus, messageCode, sessionUUID)
	if err != nil {
		applog.Warn("[ExpertGroup] 更新协作路由状态失败", "session", sessionUUID, "error", err)
	}
	now := time.Now().UnixMilli()
	if dispatchStatus == "blocked" {
		_, _ = o.db.ExecContext(ctx, `UPDATE gt_tasks SET status=CASE WHEN status='done' THEN status ELSE 'blocked' END,
			execution_status='failed', updated_at=? WHERE uuid=?`, now, taskUUID)
	} else if dispatchStatus == "paused" {
		_, _ = o.db.ExecContext(ctx, `UPDATE gt_tasks SET status=CASE WHEN status='done' THEN status ELSE 'blocked' END,
			execution_status='idle', updated_at=? WHERE uuid=?`, now, taskUUID)
	} else if dispatchStatus == "idle" || dispatchStatus == "failed" {
		_, _ = o.db.ExecContext(ctx, `UPDATE gt_tasks SET status=CASE WHEN status='done' THEN status ELSE 'active' END,
			execution_status='idle', updated_at=? WHERE uuid=?`, now, taskUUID)
	}
	if o.stateBroadcaster != nil {
		o.stateBroadcaster(taskUUID, "", sessionUUID, dispatchStatus)
	}
	for _, listener := range o.stateListeners {
		listener(taskUUID, "", sessionUUID, dispatchStatus)
	}
}

func (o *Orchestrator) StartExpertMessage(ctx context.Context, taskUUID, targetStepUUID, prompt, displayPrompt, requestID string) (string, error) {
	release := o.executionGates.acquire(taskUUID, false)
	defer release()
	return o.startExpertMessage(ctx, taskUUID, targetStepUUID, prompt, displayPrompt, requestID)
}

func (o *Orchestrator) startExpertMessage(ctx context.Context, taskUUID, targetStepUUID, prompt, displayPrompt, requestID string) (string, error) {
	if strings.TrimSpace(targetStepUUID) == "" {
		var err error
		targetStepUUID, err = taskruntime.NewService(o.db, "").ExpertLeaderStep(ctx, taskUUID)
		if err != nil {
			return "", err
		}
	}
	var role string
	if err := o.db.QueryRowContext(ctx, `SELECT member_role FROM gt_task_steps WHERE task_uuid=? AND uuid=? AND execution_mode='expert_group'`,
		taskUUID, targetStepUUID).Scan(&role); err == sql.ErrNoRows {
		return "", fmt.Errorf("所选 Agent 不在当前专家团快照中")
	} else if err != nil {
		return "", err
	}
	if err := taskruntime.NewService(o.db, "").ActivateExpertMemberForUser(ctx, taskUUID, targetStepUUID); err != nil {
		return "", err
	}
	return o.runExpertTarget(ctx, taskUUID, targetStepUUID, prompt, displayPrompt, true, "", 1, "", requestID)
}

func (o *Orchestrator) runExpertTarget(ctx context.Context, taskUUID, targetStepUUID, prompt, displayPrompt string,
	recordUser bool, chainUUID string, chainRound int, triggerSessionUUID, requestID string) (string, error) {
	var parentSessionUUID string
	err := o.db.QueryRowContext(ctx, `SELECT uuid FROM gt_cli_sessions WHERE task_uuid=? AND step_uuid=?
		AND status IN ('success','failed','stopped','interrupted') ORDER BY created_at DESC, rowid DESC LIMIT 1`,
		taskUUID, targetStepUUID).Scan(&parentSessionUUID)
	if err == nil {
		return o.continueConversation(ctx, ContinueConversationOptions{ParentSessionUUID: parentSessionUUID,
			Prompt: prompt, DisplayPrompt: displayPrompt, RequestID: requestID, InternalHandoff: !recordUser,
			ExpertChainUUID: chainUUID, ExpertChainRound: chainRound, TriggerSessionUUID: triggerSessionUUID})
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	recordType := "initial_run"
	if recordUser {
		recordType = "user_question"
	}
	return o.runStep(ctx, RunStepOptions{TaskUUID: taskUUID, StepUUID: targetStepUUID,
		UserPrompt: prompt, DisplayPrompt: displayPrompt, RequestID: requestID, RecordType: recordType,
		ExpertChainUUID: chainUUID, ExpertChainRound: chainRound, TriggerSessionUUID: triggerSessionUUID})
}
