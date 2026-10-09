package localserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"goteams-client/internal/applog"
	"goteams-client/internal/i18n"
	"goteams-client/internal/taskruntime"
	"goteams-client/internal/workflow"
)

type wechatTask struct {
	UUID            string
	ShortID         int64
	Title           string
	Status          string
	Mode            string
	Tool            string
	CurrentStepUUID string
	SessionJSON     string
}

func (w *wechatRemote) ensureAliases(ctx context.Context) error {
	_, err := w.db.ExecContext(ctx, `INSERT OR IGNORE INTO gt_remote_task_aliases(task_uuid, created_at)
		SELECT t.uuid, ? FROM gt_tasks t
		WHERE NOT EXISTS (SELECT 1 FROM gt_remote_task_aliases a WHERE a.task_uuid=t.uuid)
		ORDER BY t.created_at, t.rowid`, time.Now().UnixMilli())
	return err
}

func (w *wechatRemote) taskByShortID(ctx context.Context, shortID int64) (wechatTask, error) {
	var task wechatTask
	err := w.db.QueryRowContext(ctx, `SELECT t.uuid, a.short_id, t.title, t.status, t.execution_mode,
		t.execution_tool, t.current_step_uuid, t.vibe_coding_session_json
		FROM gt_remote_task_aliases a JOIN gt_tasks t ON t.uuid=a.task_uuid WHERE a.short_id=?`, shortID).
		Scan(&task.UUID, &task.ShortID, &task.Title, &task.Status, &task.Mode, &task.Tool, &task.CurrentStepUUID, &task.SessionJSON)
	return task, err
}

func (w *wechatRemote) taskByUUID(ctx context.Context, taskUUID string) (wechatTask, error) {
	var task wechatTask
	err := w.db.QueryRowContext(ctx, `SELECT t.uuid, a.short_id, t.title, t.status, t.execution_mode,
		t.execution_tool, t.current_step_uuid, t.vibe_coding_session_json
		FROM gt_remote_task_aliases a JOIN gt_tasks t ON t.uuid=a.task_uuid WHERE t.uuid=?`, taskUUID).
		Scan(&task.UUID, &task.ShortID, &task.Title, &task.Status, &task.Mode, &task.Tool, &task.CurrentStepUUID, &task.SessionJSON)
	return task, err
}

func (w *wechatRemote) listTasks(ctx context.Context, status string) (string, error) {
	if err := w.ensureAliases(ctx); err != nil {
		return "", err
	}
	query := `SELECT a.short_id, t.title FROM gt_remote_task_aliases a JOIN gt_tasks t ON t.uuid=a.task_uuid`
	args := []any{}
	if status != "" {
		query += ` WHERE t.status=?`
		args = append(args, status)
	}
	query += ` ORDER BY a.short_id`
	rows, err := w.db.QueryContext(ctx, query, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var id int64
		var title string
		if err = rows.Scan(&id, &title); err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("#%d %s", id, title))
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	labelKey := map[string]string{"active": "remote_wechat_status_running", "done": "remote_wechat_status_done", "blocked": "remote_wechat_status_blocked", "": "remote_wechat_status_all"}[status]
	params := i18n.Params{"Count": len(lines), "Status": w.tr(ctx, labelKey, nil), "Tasks": strings.Join(lines, "\n"), "Help": w.tr(ctx, "remote_wechat_help", nil)}
	if len(lines) == 0 {
		return w.tr(ctx, "remote_wechat_empty_tasks", params), nil
	}
	return w.tr(ctx, "remote_wechat_list", params), nil
}

func (w *wechatRemote) latestFinal(ctx context.Context, task wechatTask) (string, error) {
	var result string
	var err error
	if task.Mode == taskruntime.ExecutionModeVibeCoding {
		err = w.db.QueryRowContext(ctx, `SELECT final_result FROM gt_task_progress WHERE task_uuid=?
			AND execution_mode='vibe_coding' AND record_type<>'user_question' AND final_result<>''
			AND activity_kind<>'assignment' AND uuid<>?
			ORDER BY created_at DESC, rowid DESC LIMIT 1`, task.UUID, vibeCodingAssignmentProgressUUID(task.UUID)).Scan(&result)
	} else {
		err = w.db.QueryRowContext(ctx, `SELECT final_result FROM gt_cli_sessions WHERE task_uuid=?
			AND status IN ('success','failed','stopped','interrupted') AND final_result<>''
			ORDER BY created_at DESC, rowid DESC LIMIT 1`, task.UUID).Scan(&result)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return w.tr(ctx, "remote_wechat_last_none", nil), nil
	}
	return strings.TrimSpace(result), err
}

func (w *wechatRemote) processInbox(ctx context.Context) error {
	w.dispatchMu.RLock()
	defer w.dispatchMu.RUnlock()
	if w.pendingFinish != nil {
		return w.flushPendingFinish(ctx)
	}
	if _, err := w.readState(ctx); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	var id, content string
	err := w.db.QueryRowContext(ctx, `SELECT message_id, text_content FROM gt_remote_wechat_inbox
		WHERE status='pending' ORDER BY received_at, rowid LIMIT 1`).Scan(&id, &content)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// Mark first so a restart cannot dispatch the same remote command twice.
	result, err := w.db.ExecContext(ctx, `UPDATE gt_remote_wechat_inbox SET status='processing', updated_at=?
		WHERE message_id=? AND status='pending'`, time.Now().UnixMilli(), id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return err
	}
	if len([]rune(content)) > 16000 {
		w.pendingFinish = &wechatPendingFinish{id: id, status: "done", reply: w.tr(ctx, "remote_wechat_message_too_long", nil)}
		return w.flushPendingFinish(ctx)
	}
	reply, taskUUID, sessionUUID, dispatchErr := w.handleMessage(ctx, id, content)
	status := "done"
	if errors.Is(dispatchErr, errCodexOutcomeUnknown) {
		status = "uncertain"
		reply = w.tr(ctx, "remote_wechat_dispatch_uncertain", nil)
		dispatchErr = nil
	}
	if dispatchErr != nil {
		reply = w.tr(ctx, "remote_wechat_processing_failed", nil)
	}
	if sessionUUID != "" {
		status = "dispatched"
	}
	w.pendingFinish = &wechatPendingFinish{id: id, status: status, taskUUID: taskUUID, sessionUUID: sessionUUID, reply: reply}
	return w.flushPendingFinish(ctx)
}

func (w *wechatRemote) flushPendingFinish(ctx context.Context) error {
	pending := w.pendingFinish
	if err := w.finishProcessing(ctx, pending.id, pending.status, pending.taskUUID, pending.sessionUUID, pending.reply); err != nil {
		return err
	}
	w.pendingFinish = nil
	return nil
}

func (w *wechatRemote) finishProcessing(ctx context.Context, id, status, taskUUID, sessionUUID, reply string) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE gt_remote_wechat_inbox SET status=?, task_uuid=?, session_uuid=?, updated_at=?
		WHERE message_id=? AND status='processing'`, status, taskUUID, sessionUUID, time.Now().UnixMilli(), id)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		return nil
	}
	if reply != "" {
		if err := w.enqueueTx(ctx, tx, "reply:"+id, taskUUID, reply); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (w *wechatRemote) finishDispatched(ctx context.Context, id, taskUUID, reply string) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE gt_remote_wechat_inbox SET status='done', updated_at=?
		WHERE message_id=? AND status='dispatched'`, time.Now().UnixMilli(), id)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		return nil
	}
	if err := w.enqueueTx(ctx, tx, "result:"+id, taskUUID, reply); err != nil {
		return err
	}
	return tx.Commit()
}

func (w *wechatRemote) handleMessage(ctx context.Context, messageID, raw string) (reply, taskUUID, sessionUUID string, err error) {
	content := strings.TrimSpace(raw)
	command, params := content, ""
	if split := strings.IndexByte(content, ' '); split >= 0 {
		command, params = content[:split], strings.TrimSpace(content[split+1:])
	}
	if command == "/task" || command == "/t" || command == "/T" {
		status := "active"
		switch params {
		case "", "-r":
		case "-a":
			status = ""
		case "-f":
			status = "done"
		case "-e":
			status = "blocked"
		default:
			return w.tr(ctx, "remote_wechat_params_invalid", i18n.Params{"Help": w.tr(ctx, "remote_wechat_help", nil)}), "", "", nil
		}
		reply, err = w.listTasks(ctx, status)
		return reply, "", "", err
	}
	if err = w.ensureAliases(ctx); err != nil {
		return
	}
	var task wechatTask
	if strings.HasPrefix(content, "@") {
		payload := content[1:]
		idText, instruction := payload, ""
		if split := strings.IndexFunc(payload, unicode.IsSpace); split >= 0 {
			idText, instruction = payload[:split], strings.TrimSpace(payload[split:])
		}
		shortID, parseErr := strconv.ParseInt(idText, 10, 64)
		if parseErr != nil || shortID < 1 {
			return w.tr(ctx, "remote_wechat_id_invalid", i18n.Params{"Help": w.tr(ctx, "remote_wechat_help", nil)}), "", "", nil
		}
		task, err = w.taskByShortID(ctx, shortID)
		if errors.Is(err, sql.ErrNoRows) {
			return w.tr(ctx, "remote_wechat_task_missing", i18n.Params{"Help": w.tr(ctx, "remote_wechat_help", nil)}), "", "", nil
		}
		if err != nil {
			return
		}
		if _, err = w.db.ExecContext(ctx, `UPDATE gt_remote_wechat_state SET bound_task_uuid=?, updated_at=? WHERE id=1`, task.UUID, time.Now().UnixMilli()); err != nil {
			return
		}
		if instruction == "" {
			last, lastErr := w.latestFinal(ctx, task)
			if lastErr != nil {
				err = lastErr
				return
			}
			return w.tr(ctx, "remote_wechat_switch", i18n.Params{"Task": task.Title, "Result": last}), task.UUID, "", nil
		}
		content = instruction
	} else {
		state, readErr := w.readState(ctx)
		if readErr != nil {
			err = readErr
			return
		}
		if state.BoundTaskUUID == "" {
			return w.tr(ctx, "remote_wechat_no_binding", i18n.Params{"Help": w.tr(ctx, "remote_wechat_help", nil)}), "", "", nil
		}
		task, err = w.taskByUUID(ctx, state.BoundTaskUUID)
		if errors.Is(err, sql.ErrNoRows) {
			_, _ = w.db.ExecContext(ctx, `UPDATE gt_remote_wechat_state SET bound_task_uuid='' WHERE id=1`)
			return w.tr(ctx, "remote_wechat_task_missing", i18n.Params{"Help": w.tr(ctx, "remote_wechat_help", nil)}), "", "", nil
		}
		if err != nil {
			return
		}
	}
	if block, blockErr := remoteExecutionBlock(ctx, w.db, task.UUID); blockErr != nil {
		err = blockErr
		return
	} else if block == "uncertain" {
		return w.tr(ctx, "remote_wechat_uncertain_blocked", nil), task.UUID, "", nil
	}
	busy, busyErr := w.taskBusy(ctx, task.UUID)
	if busyErr != nil {
		err = busyErr
		return
	}
	if busy {
		return w.tr(ctx, "remote_wechat_busy", i18n.Params{"Task": task.Title}), task.UUID, "", nil
	}
	if err = w.markProcessingTask(ctx, messageID, task.UUID); err != nil {
		return "", task.UUID, "", err
	}
	sessionUUID, err = w.dispatchTask(ctx, task, messageID, content)
	if errors.Is(err, errCodexOutcomeUnknown) {
		applog.Warn("微信指令下发结果不确定", "task_uuid", task.UUID, "category", "uncertain", "stage", "codex_turn_start")
		return "", task.UUID, "", err
	}
	if err != nil {
		return w.dispatchErrorReply(ctx, task, err), task.UUID, "", nil
	}
	return w.tr(ctx, "remote_wechat_send_ack", i18n.Params{"Task": task.Title, "Message": content}), task.UUID, sessionUUID, nil
}

func (w *wechatRemote) dispatchErrorReply(ctx context.Context, task wechatTask, err error) string {
	if errors.Is(err, errCodexBusy) {
		applog.Warn("微信指令下发被活动执行拦截", "task_uuid", task.UUID, "category", "busy", "stage", "execution")
		return w.tr(ctx, "remote_wechat_busy", i18n.Params{"Task": task.Title})
	}
	key, category, stage := "remote_wechat_dispatch_failed", "runtime", "dispatch"
	var failure *wechatDispatchFailure
	if errors.As(err, &failure) {
		key, category, stage = failure.key, failure.category, failure.stage
	}
	applog.Warn("微信指令下发失败", "task_uuid", task.UUID, "category", category, "stage", stage)
	return w.tr(ctx, key, nil)
}

func (w *wechatRemote) taskBusy(ctx context.Context, taskUUID string) (bool, error) {
	var active int
	err := w.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM gt_cli_sessions WHERE task_uuid=? AND status IN ('created','running','waiting_input','stop_requested'))
		+ (SELECT COUNT(*) FROM gt_task_progress WHERE task_uuid=? AND dispatch_status='routing_pending')
		+ (SELECT COUNT(*) FROM gt_remote_wechat_inbox WHERE task_uuid=? AND status IN ('dispatched','uncertain'))`, taskUUID, taskUUID, taskUUID).Scan(&active)
	return active > 0, err
}

func (w *wechatRemote) markProcessingTask(ctx context.Context, messageID, taskUUID string) error {
	result, err := w.db.ExecContext(ctx, `UPDATE gt_remote_wechat_inbox SET task_uuid=?, updated_at=?
		WHERE message_id=? AND status='processing'`, taskUUID, time.Now().UnixMilli(), messageID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("微信指令已取消")
	}
	return nil
}

func (w *wechatRemote) dispatchTask(ctx context.Context, task wechatTask, messageID, content string) (string, error) {
	if task.Mode == taskruntime.ExecutionModeVibeCoding {
		if task.Tool != taskruntime.ExecutionToolCodex {
			return "", wechatConfigFailure("execution_mode", "remote_wechat_config_mode")
		}
		return w.dispatchCodex(ctx, task, messageID, content)
	}
	if task.Mode != taskruntime.ExecutionModeCLI && task.Mode != taskruntime.ExecutionModePipeline &&
		task.Mode != taskruntime.ExecutionModeExpertGroup {
		return "", wechatConfigFailure("execution_mode", "remote_wechat_config_mode")
	}
	if err := w.validateDispatchWorkspace(ctx, task.UUID); err != nil {
		return "", err
	}
	if task.Mode == taskruntime.ExecutionModeExpertGroup {
		var stepUUID string
		err := w.db.QueryRowContext(ctx, `SELECT leader_task_step_uuid FROM gt_task_expert_group_snapshots WHERE task_uuid=?`,
			task.UUID).Scan(&stepUUID)
		if errors.Is(err, sql.ErrNoRows) || err == nil && stepUUID == "" {
			return "", wechatConfigFailure("expert_group", "remote_wechat_config_expert")
		}
		if err != nil {
			return "", err
		}
		if err := w.validateDispatchStep(ctx, task.UUID, stepUUID); err != nil {
			return "", err
		}
		if w.orchestrator == nil {
			return "", errors.New("任务执行服务不可用")
		}
		return w.orchestrator.StartExpertMessage(ctx, task.UUID, "", content, content, messageID)
	}
	if task.CurrentStepUUID == "" {
		if task.Mode != taskruntime.ExecutionModePipeline {
			return "", wechatConfigFailure("step", "remote_wechat_config_step")
		}
		stepUUID, err := w.preparePipelineMessage(ctx, task)
		if err != nil {
			return "", err
		}
		task.CurrentStepUUID = stepUUID
	}
	if err := w.validateDispatchStep(ctx, task.UUID, task.CurrentStepUUID); err != nil {
		return "", err
	}
	if w.orchestrator == nil {
		return "", errors.New("任务执行服务不可用")
	}
	var parentUUID, parentStatus string
	err := w.db.QueryRowContext(ctx, `SELECT uuid, status FROM gt_cli_sessions WHERE task_uuid=? AND step_uuid=?
		ORDER BY created_at DESC, rowid DESC LIMIT 1`, task.UUID, task.CurrentStepUUID).Scan(&parentUUID, &parentStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return w.orchestrator.RunStep(ctx, workflow.RunStepOptions{TaskUUID: task.UUID, StepUUID: task.CurrentStepUUID,
			UserPrompt: content, DisplayPrompt: content, RecordType: "user_question", RequestID: messageID})
	}
	if err != nil {
		return "", err
	}
	if !workflow.IsTerminalStatus(parentStatus) {
		return "", errors.New("任务仍在执行")
	}
	return w.orchestrator.ContinueConversation(ctx, workflow.ContinueConversationOptions{
		ParentSessionUUID: parentUUID, Prompt: content, DisplayPrompt: content, RequestID: messageID})
}

func (w *wechatRemote) collectResults(ctx context.Context) error {
	rows, err := w.db.QueryContext(ctx, `SELECT message_id, task_uuid, session_uuid FROM gt_remote_wechat_inbox
		WHERE status='dispatched' AND session_uuid NOT LIKE 'codex:%' AND next_recovery_at<=?
		ORDER BY next_recovery_at, received_at LIMIT 16`, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	defer rows.Close()
	type item struct{ id, task, session string }
	var pending []item
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.id, &v.task, &v.session); err != nil {
			return err
		}
		pending = append(pending, v)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, v := range pending {
		var status, final string
		err = w.db.QueryRowContext(ctx, `SELECT status, final_result FROM gt_cli_sessions WHERE uuid=?`, v.session).Scan(&status, &final)
		if errors.Is(err, sql.ErrNoRows) {
			if err := w.finishDispatched(ctx, v.id, v.task, w.tr(ctx, "remote_wechat_result_unavailable", nil)); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !workflow.IsTerminalStatus(status) {
			if err := w.deferCLIResult(ctx, v.id); err != nil {
				return err
			}
			continue
		}
		var mode, title string
		err = w.db.QueryRowContext(ctx, `SELECT execution_mode,title FROM gt_tasks WHERE uuid=?`, v.task).Scan(&mode, &title)
		if errors.Is(err, sql.ErrNoRows) {
			title = w.tr(ctx, "remote_wechat_task_deleted_title", nil)
		} else if err != nil {
			return err
		}
		if mode == taskruntime.ExecutionModeExpertGroup {
			var chain string
			if err = w.db.QueryRowContext(ctx, `SELECT expert_chain_uuid FROM gt_cli_sessions WHERE uuid=?`, v.session).Scan(&chain); err != nil {
				return err
			}
			busy, busyErr := w.expertChainBusy(ctx, v.task, chain)
			if busyErr != nil {
				return busyErr
			}
			if busy {
				if err := w.deferCLIResult(ctx, v.id); err != nil {
					return err
				}
				continue
			}
			err = w.db.QueryRowContext(ctx, `SELECT final_result FROM gt_cli_sessions WHERE task_uuid=?
				AND expert_chain_uuid=? AND status IN ('success','failed','stopped','interrupted')
				ORDER BY expert_chain_round DESC,created_at DESC,rowid DESC LIMIT 1`, v.task, chain).Scan(&final)
			if err != nil {
				return err
			}
		}
		var shortID int64
		_ = w.db.QueryRowContext(ctx, `SELECT short_id FROM gt_remote_task_aliases WHERE task_uuid=?`, v.task).Scan(&shortID)
		body := w.tr(ctx, "remote_wechat_result", i18n.Params{"ID": shortID, "Task": title, "Result": strings.TrimSpace(final)})
		if err = w.finishDispatched(ctx, v.id, v.task, body); err != nil {
			return err
		}
	}
	return nil
}

func (w *wechatRemote) deferCLIResult(ctx context.Context, messageID string) error {
	_, err := w.db.ExecContext(ctx, `UPDATE gt_remote_wechat_inbox SET next_recovery_at=?
		WHERE message_id=? AND status='dispatched'`, time.Now().Add(5*time.Second).UnixMilli(), messageID)
	return err
}

func (w *wechatRemote) expertChainBusy(ctx context.Context, taskUUID, chain string) (bool, error) {
	var active int
	err := w.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM gt_cli_sessions WHERE task_uuid=? AND expert_chain_uuid=?
			AND status IN ('created','running','waiting_input','stop_requested'))
		+ (SELECT COUNT(*) FROM gt_task_progress p JOIN gt_cli_sessions s ON s.uuid=p.session_uuid
			WHERE s.task_uuid=? AND s.expert_chain_uuid=? AND p.dispatch_status='routing_pending')`, taskUUID, chain, taskUUID, chain).Scan(&active)
	return active > 0, err
}
