package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type DockerRunner struct {
	image string
}

func NewDockerRunner(image string) *DockerRunner {
	return &DockerRunner{image: image}
}

func dockerContainerName(userID int64, conversationID string) string {
	return "guchat-sbx-u" + strconv.FormatInt(userID, 10) + "-" + conversationID
}

func (r *DockerRunner) Open(ctx context.Context, input OpenInput) error {
	name := dockerContainerName(input.UserID, input.ConversationID)

	// 检查容器是否已经存在且运行
	inspect := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", name)
	output, err := inspect.Output()
	if err == nil && strings.TrimSpace(string(output)) == "true" {
		return nil
	}

	// 如果存在但没运行
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", name).Run()

	args := []string{
		"run", "-d",
		"--name", name,
		"--memory", "512m",
		"--cpus", "1",
		"--pids-limit", "128",
		"--cap-drop", "NET_RAW",
		"--cap-drop", "MKNOD",
		"--security-opt", "no-new-privileges",
		"-v", input.WorkspacePath + ":/workspace",
		"-w", "/workspace",
		r.image,
		"sleep", "infinity",
	}

	return exec.CommandContext(ctx, "docker", args...).Run()
}

func (r *DockerRunner) Exec(
	ctx context.Context,
	userID int64,
	conversationID string,
	input ExecInput,
) (*ExecResult, error) {
	name := dockerContainerName(userID, conversationID)

	inspect := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", name)
	output, err := inspect.Output()
	if err != nil || strings.TrimSpace(string(output)) != "true" {
		return nil, ErrTerminalNotOpen
	}

	execCtx, cancel := context.WithTimeout(ctx, input.Timeout)
	defer cancel()

	startedAt := time.Now()

	cmd := exec.CommandContext(execCtx, "docker", "exec", name, "sh", "-c", input.Command)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	duration := time.Since(startedAt)

	result := &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
		TimedOut: errors.Is(execCtx.Err(), context.DeadlineExceeded),
	}

	if err == nil {
		result.ExitCode = 0
		return result, nil
	}

	if result.TimedOut {
		result.ExitCode = -1
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}

	return nil, err
}

func (r *DockerRunner) Destroy(ctx context.Context, userID int64, conversationID string) error {
	name := dockerContainerName(userID, conversationID)
	output, err := exec.CommandContext(ctx, "docker", "rm", "-f", name).CombinedOutput()
	if err == nil {
		return nil
	}

	text := strings.TrimSpace(string(output))
	if strings.Contains(text, "No such container") {
		return nil
	}

	return fmt.Errorf("destroy docker container %s: %w: %s", name, err, text)
}

func (r *DockerRunner) Check(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "info")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker is not available: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
