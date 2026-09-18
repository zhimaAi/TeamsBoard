package taskgit

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrUnavailable      = errors.New("git unavailable")
	ErrNotRepository    = errors.New("not a git repository")
	ErrInvalidBranch    = errors.New("invalid local branch")
	ErrSwitchFailed     = errors.New("git branch switch failed")
	ErrSwitchMerging    = errors.New("git branch switch blocked by merge")
	ErrSwitchRebasing   = errors.New("git branch switch blocked by rebase")
	ErrSwitchDirty      = errors.New("git branch switch blocked by local changes")
	ErrSwitchCheckedOut = errors.New("git branch already checked out")
)

type Snapshot struct {
	Current   string   `json:"current"`
	Branches  []string `json:"branches"`
	Available bool     `json:"available"`
	Detached  bool     `json:"detached"`
}

func run(ctx context.Context, directory string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", directory}, args...)...)
	// Git hook 的子进程可能继续持有输出管道，截止后仍要有界结束等待。
	command.WaitDelay = 250 * time.Millisecond
	return command.Output()
}

func commandErrorText(err error) string {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		if text := strings.TrimSpace(string(exitError.Stderr)); text != "" {
			return text
		}
	}
	return strings.TrimSpace(err.Error())
}

func containsAny(text string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(text, part) {
			return true
		}
	}
	return false
}

func classifySwitchError(err error) error {
	text := strings.ToLower(commandErrorText(err))
	switch {
	case containsAny(text, "while merging", "正在合并", "合并时"):
		return ErrSwitchMerging
	case containsAny(text, "while rebasing", "正在变基", "变基时"):
		return ErrSwitchRebasing
	case containsAny(text, "already checked out", "已经检出", "已检出"):
		return ErrSwitchCheckedOut
	case containsAny(text, "would be overwritten", "local changes", "uncommitted", "本地修改", "未提交"):
		return ErrSwitchDirty
	default:
		return ErrSwitchFailed
	}
}

// WorktreeRoot 返回目录所在 Git 工作树的真实根目录；linked worktree
// 使用自己的根目录，避免与主工作树互相阻塞。
func WorktreeRoot(ctx context.Context, directory string) (string, error) {
	root, err := run(ctx, directory, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrUnavailable
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", ErrNotRepository
	}
	path := strings.TrimSpace(string(root))
	if path == "" {
		return "", ErrNotRepository
	}
	if canonical, err := filepath.EvalSymlinks(path); err == nil {
		path = canonical
	}
	return filepath.Clean(path), nil
}

func List(ctx context.Context, directory string) (Snapshot, error) {
	snapshot := Snapshot{Branches: []string{}}
	inside, err := run(ctx, directory, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return snapshot, ErrUnavailable
		}
		if ctx.Err() != nil {
			return snapshot, ctx.Err()
		}
		return snapshot, nil
	}
	if strings.TrimSpace(string(inside)) != "true" {
		return snapshot, nil
	}
	// 同名 tag 会让 Git 的 :short 格式返回 heads/foo，因此读取完整 ref 后去前缀。
	branches, err := run(ctx, directory, "for-each-ref", "--format=%(refname)", "refs/heads/")
	if err != nil {
		return snapshot, err
	}
	current, err := run(ctx, directory, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) || exitError.ExitCode() != 1 {
			return snapshot, err
		}
		snapshot.Detached = true
	}
	snapshot.Available = true
	currentRef := strings.TrimSpace(string(current))
	if strings.HasPrefix(currentRef, "refs/heads/") {
		snapshot.Current = strings.TrimPrefix(currentRef, "refs/heads/")
	}
	for _, line := range strings.Split(string(branches), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "refs/heads/") {
			line = strings.TrimPrefix(line, "refs/heads/")
		}
		if line != "" {
			snapshot.Branches = append(snapshot.Branches, line)
		}
	}
	return snapshot, nil
}

func Switch(ctx context.Context, directory, branch string) error {
	if branch == "" || strings.TrimSpace(branch) != branch || strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, "\x00\r\n") {
		return ErrInvalidBranch
	}
	snapshot, err := List(ctx, directory)
	if err != nil {
		return err
	}
	if !snapshot.Available {
		return ErrNotRepository
	}
	found := false
	for _, localBranch := range snapshot.Branches {
		if localBranch == branch {
			found = true
			break
		}
	}
	if !found {
		return ErrInvalidBranch
	}
	if snapshot.Current == branch {
		return nil
	}
	// switch 只接受已存在的本地分支，不使用 checkout 的路径回退，也不强制覆盖或自动 stash。
	if _, err := run(ctx, directory, "switch", "--no-guess", branch); err != nil {
		return classifySwitchError(err)
	}
	return nil
}
