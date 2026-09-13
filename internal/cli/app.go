// Package cli exposes the first, non-interactive LDR commands.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/swsw1005/local-dev-launcher/internal/discovery"
	"github.com/swsw1005/local-dev-launcher/internal/domain"
	"github.com/swsw1005/local-dev-launcher/internal/execution"
	"github.com/swsw1005/local-dev-launcher/internal/gitignore"
	"github.com/swsw1005/local-dev-launcher/internal/process"
	"github.com/swsw1005/local-dev-launcher/internal/profile"
	"github.com/swsw1005/local-dev-launcher/internal/project"
	"github.com/swsw1005/local-dev-launcher/internal/state"
	"github.com/swsw1005/local-dev-launcher/internal/tui"
)

const Version = "0.1.0"

const helpText = `Local Dev Runner (LDR)

Usage:
  ldr [command]

Commands:
  init [--yes, -y]  Create project-local .ldr state
  list [--json]     List discovered runnable tasks
  refresh           Rebuild the discovery cache
  run <task-id>     Run a discovered task
  start <task-id>   Start a task in the background
  ps                List LDR-managed processes
  stop <process-id> Stop a managed process
  logs <process-id> Print process logs
  profile ...       Create and inspect user execution profiles
  tui               Open the interactive task launcher
  help              Show this help
  version           Show the LDR version

Options:
  -h, --help        Show this help
  -v, --version     Show the LDR version
  -y, --yes         Accepted for script compatibility; initialization is non-blocking

On first use, LDR creates .ldr/ in the detected project root without blocking.
Add .ldr/ to your project's .gitignore; LDR warns but never changes it automatically.
`

type App struct {
	out      io.Writer
	errOut   io.Writer
	git      gitignore.Checker
	findRoot func(string) (string, error)
	version  string
	in       io.Reader
	terminal bool
}

func New(in io.Reader, out, errOut io.Writer) App {
	return App{
		in:       in,
		out:      out,
		errOut:   errOut,
		git:      gitignore.CommandChecker{},
		findRoot: project.FindRoot,
		version:  Version,
		terminal: isTerminal(in),
	}
}

func (a App) Run(ctx context.Context, args []string, directory string) error {
	switch {
	case len(args) == 0:
		return a.launchTUI(ctx, directory)
	case len(args) == 1 && isHelp(args[0]):
		fmt.Fprint(a.out, helpText)
		return nil
	case len(args) == 1 && isVersion(args[0]):
		fmt.Fprintf(a.out, "ldr %s\n", a.version)
		return nil
	case args[0] == "init":
		if err := validateInitArgs(args[1:]); err != nil {
			return err
		}
		return a.initialize(ctx, directory, false)
	case args[0] == "list":
		jsonOutput, err := validateListArgs(args[1:])
		if err != nil {
			return err
		}
		return a.list(ctx, directory, jsonOutput, false)
	case len(args) == 1 && args[0] == "refresh":
		return a.list(ctx, directory, false, true)
	case args[0] == "run":
		if len(args) != 2 {
			return errors.New("usage: ldr run <task-id>")
		}
		return a.run(ctx, directory, args[1])
	case args[0] == "start":
		if len(args) != 2 {
			return errors.New("usage: ldr start <task-id-or-profile>")
		}
		return a.start(ctx, directory, args[1])
	case len(args) == 1 && args[0] == "ps":
		return a.ps(ctx, directory)
	case args[0] == "stop":
		if len(args) != 2 {
			return errors.New("usage: ldr stop <process-id>")
		}
		return a.stop(ctx, directory, args[1])
	case args[0] == "logs":
		if len(args) != 2 {
			return errors.New("usage: ldr logs <process-id>")
		}
		return a.logs(ctx, directory, args[1])
	case args[0] == "profile":
		return a.profile(ctx, directory, args[1:])
	case len(args) == 1 && args[0] == "tui":
		return a.launchTUI(ctx, directory)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], helpText)
	}
}

func (a App) launchTUI(ctx context.Context, directory string) error {
	if err := a.initialize(ctx, directory, false); err != nil {
		return err
	}
	if !a.terminal {
		return nil
	}
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	result, err := discovery.LoadOrDiscover(root, state.NewLayout(root), false)
	if err != nil {
		return fmt.Errorf("discover tasks: %w", err)
	}
	if len(result.Tasks) == 0 {
		fmt.Fprintln(a.out, "No runnable tasks discovered.")
		return nil
	}
	browser := tui.State{}
	for {
		selection, err := tui.SelectWithState(a.in, a.out, result.Tasks, browser)
		if err != nil || selection.TaskID == "" {
			return err
		}
		browser = selection.State
		if err := a.runFromTUI(ctx, directory, selection.TaskID); err != nil {
			browser = browser.WithNotice(fmt.Sprintf("%s stopped or failed (%v). Enter reruns it.", selection.TaskID, err))
			continue
		}
		browser = browser.WithNotice(fmt.Sprintf("%s finished. Enter reruns it.", selection.TaskID))
	}
}

// runFromTUI keeps Ctrl+C scoped to the foreground child task. Terminals send
// that signal to both the Gradle process and LDR's foreground process group;
// registering it here prevents LDR itself from exiting before it can reopen
// the selected task pane.
func (a App) runFromTUI(ctx context.Context, directory, taskID string) error {
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	return a.run(ctx, directory, taskID)
}

func (a App) run(ctx context.Context, directory, taskID string) error {
	if err := a.initialize(ctx, directory, false); err != nil {
		return err
	}
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	task, options, err := a.resolveRunnable(root, taskID)
	if err != nil {
		return err
	}
	return execution.RunWithOptions(ctx, root, task, a.out, a.errOut, options)
}

func (a App) start(ctx context.Context, directory, taskID string) error {
	if err := a.initialize(ctx, directory, false); err != nil {
		return err
	}
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	task, options, err := a.resolveRunnable(root, taskID)
	if err != nil {
		return err
	}
	record, err := process.New(state.NewLayout(root)).Start(ctx, task, options)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Started %s\nPID: %d\nProcess: %s\nLogs: %s\n", record.TaskID, record.PID, record.ID, record.LogPath)
	return nil
}

func (a App) ps(ctx context.Context, directory string) error {
	if err := a.initialize(ctx, directory, false); err != nil {
		return err
	}
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	records := process.New(state.NewLayout(root)).List()
	if len(records) == 0 {
		fmt.Fprintln(a.out, "No managed processes.")
		return nil
	}
	fmt.Fprintln(a.out, "PROCESS\tPID\tSTATUS\tTASK")
	for _, record := range records {
		fmt.Fprintf(a.out, "%s\t%d\t%s\t%s\n", record.ID, record.PID, record.Status, record.TaskID)
	}
	return nil
}

func (a App) stop(ctx context.Context, directory, processID string) error {
	if err := a.initialize(ctx, directory, false); err != nil {
		return err
	}
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	record, err := process.New(state.NewLayout(root)).Stop(processID)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Stopped %s\n", record.ID)
	return nil
}

func (a App) logs(ctx context.Context, directory, processID string) error {
	if err := a.initialize(ctx, directory, false); err != nil {
		return err
	}
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	contents, err := process.New(state.NewLayout(root)).Logs(processID)
	if err != nil {
		return err
	}
	_, err = a.out.Write(contents)
	return err
}

func (a App) resolveRunnable(root, taskID string) (domain.Task, execution.Options, error) {
	layout := state.NewLayout(root)
	result, err := discovery.LoadOrDiscover(root, layout, false)
	if err != nil {
		return domain.Task{}, execution.Options{}, fmt.Errorf("discover tasks: %w", err)
	}
	for _, task := range result.Tasks {
		if task.ID == taskID {
			return task, execution.Options{}, nil
		}
	}
	loaded, err := profile.LoadByName(layout.Profiles, taskID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.Task{}, execution.Options{}, fmt.Errorf("task or profile %q was not found; run `ldr list` or `ldr profile list`", taskID)
		}
		return domain.Task{}, execution.Options{}, err
	}
	for _, task := range result.Tasks {
		if task.ID == loaded.Extends {
			return task, execution.Options{Env: loaded.Env, PrependArgs: loaded.PrependArgs, AppendArgs: loaded.AppendArgs}, nil
		}
	}
	return domain.Task{}, execution.Options{}, fmt.Errorf("profile %q is BROKEN: base task %q was not found", loaded.Name, loaded.Extends)
}

func (a App) profile(ctx context.Context, directory string, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: ldr profile <clone|list|show>")
	}
	if err := a.initialize(ctx, directory, false); err != nil {
		return err
	}
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	layout := state.NewLayout(root)
	switch args[0] {
	case "clone":
		if len(args) != 3 {
			return errors.New("usage: ldr profile clone <task-id> <profile-name>")
		}
		result, err := discovery.LoadOrDiscover(root, layout, false)
		if err != nil {
			return err
		}
		if !hasTask(result.Tasks, args[1]) {
			return fmt.Errorf("task %q was not found; run `ldr list` to see available tasks", args[1])
		}
		created, err := profile.Create(layout.Profiles, args[2], args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "Created profile %q extending %s\n", created.Name, created.Extends)
		return nil
	case "list":
		if len(args) != 1 {
			return errors.New("usage: ldr profile list")
		}
		result, err := discovery.LoadOrDiscover(root, layout, false)
		if err != nil {
			return err
		}
		profiles, err := profile.LoadAll(layout.Profiles)
		if err != nil {
			return err
		}
		if len(profiles) == 0 {
			fmt.Fprintln(a.out, "No profiles found.")
			return nil
		}
		fmt.Fprintln(a.out, "NAME\tEXTENDS\tSTATUS")
		for _, loaded := range profiles {
			status := "READY"
			if !hasTask(result.Tasks, loaded.Extends) {
				status = "BROKEN"
			}
			fmt.Fprintf(a.out, "%s\t%s\t%s\n", loaded.Name, loaded.Extends, status)
		}
		return nil
	case "show":
		if len(args) != 2 {
			return errors.New("usage: ldr profile show <profile-name>")
		}
		loaded, err := profile.LoadByName(layout.Profiles, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "Name: %s\nExtends: %s\n", loaded.Name, loaded.Extends)
		if len(loaded.Env) > 0 {
			fmt.Fprintln(a.out, "Environment:")
			for key, value := range loaded.Env {
				fmt.Fprintf(a.out, "  %s=%s\n", key, value)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown profile command %q", args[0])
	}
}

func hasTask(tasks []domain.Task, taskID string) bool {
	for _, task := range tasks {
		if task.ID == taskID {
			return true
		}
	}
	return false
}

func (a App) list(ctx context.Context, directory string, jsonOutput, force bool) error {
	if err := a.initialize(ctx, directory, jsonOutput); err != nil {
		return err
	}
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	result, err := discovery.LoadOrDiscover(root, state.NewLayout(root), force)
	if err != nil {
		return fmt.Errorf("discover tasks: %w", err)
	}
	if force {
		fmt.Fprintf(a.out, "Refreshed %d task(s).\n", len(result.Tasks))
	}
	if jsonOutput {
		return json.NewEncoder(a.out).Encode(result.Tasks)
	}
	printTasks(a.out, result.Tasks)
	return nil
}

func printTasks(out io.Writer, tasks []domain.Task) {
	if len(tasks) == 0 {
		fmt.Fprintln(out, "No runnable tasks discovered.")
		return
	}
	fmt.Fprintln(out, "ID\tADAPTER\tMODULE\tCOMMAND")
	for _, task := range tasks {
		command := task.Command
		for _, arg := range task.Args {
			command += " " + arg
		}
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", task.ID, task.Adapter, task.Module, command)
	}
}

func (a App) initialize(ctx context.Context, directory string, quiet bool) error {
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	layout := state.NewLayout(root)
	_, err = os.Lstat(layout.Base)
	newLayout := errors.Is(err, os.ErrNotExist)
	if err != nil && !newLayout {
		return fmt.Errorf("inspect %s: %w", layout.Base, err)
	}
	if err := layout.Ensure(); err != nil {
		return err
	}
	if newLayout && !quiet {
		fmt.Fprintf(a.out, "Initialized %s\n", filepath.Join(root, ".ldr"))
	}

	status, err := gitignore.Check(ctx, a.git, root)
	if err != nil {
		return fmt.Errorf("check Git ignore status: %w", err)
	}
	if status.InRepository && !status.Ignored {
		fmt.Fprintln(a.errOut, "WARNING: .ldr/ is not ignored by Git.\n경고: .ldr/ 디렉터리가 Git에서 무시되지 않습니다.\n\nLDR stores generated cache, local process state, and user execution profiles inside .ldr/. Committing this directory is not recommended.\nLDR은 생성 캐시, 로컬 프로세스 상태, 사용자 실행 프로필을 .ldr/에 저장합니다. 이 디렉터리를 커밋하지 않는 것을 권장합니다.\n\nAdd the following entry to .gitignore:\n.gitignore에 다음 항목을 추가하세요:\n\n    .ldr/")
	}
	return nil
}

func validateInitArgs(args []string) error {
	if len(args) == 0 {
		return nil
	}
	if len(args) == 1 && (args[0] == "--yes" || args[0] == "-y") {
		return nil
	}
	return fmt.Errorf("usage: ldr init [--yes, -y]")
}

func validateListArgs(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--json" {
		return true, nil
	}
	return false, fmt.Errorf("usage: ldr list [--json]")
}

func isHelp(arg string) bool {
	return arg == "help" || arg == "-h" || arg == "--help"
}

func isVersion(arg string) bool {
	return arg == "version" || arg == "-v" || arg == "--version"
}

func isTerminal(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
