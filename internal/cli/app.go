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
	"sort"
	"strings"
	"time"

	"github.com/swsw1005/local-dev-launcher/internal/discovery"
	"github.com/swsw1005/local-dev-launcher/internal/domain"
	"github.com/swsw1005/local-dev-launcher/internal/execution"
	"github.com/swsw1005/local-dev-launcher/internal/gitignore"
	"github.com/swsw1005/local-dev-launcher/internal/process"
	"github.com/swsw1005/local-dev-launcher/internal/profile"
	"github.com/swsw1005/local-dev-launcher/internal/project"
	"github.com/swsw1005/local-dev-launcher/internal/runtimes"
	"github.com/swsw1005/local-dev-launcher/internal/search"
	"github.com/swsw1005/local-dev-launcher/internal/state"
	"github.com/swsw1005/local-dev-launcher/internal/tui"
)

const Version = "0.7.0"

const helpText = `Local Dev Runner (LDR)

Usage:
  ldr [command]

Commands:
  init [--yes, -y]  Create project-local .ldr state
  list [--json] [--recent] [--search query] List discovered runnable tasks
  refresh           Rebuild the discovery cache
  run <task-id>     Run a discovered task
  start <task-id>   Start a task in the background
  ps [--json]        List LDR-managed processes
  stop <process-id> Stop a managed process
  cleanup [--yes]    Find or terminate orphaned processes
  doctor [--json]    Diagnose the project and LDR environment
  alias [name target] List or create a task alias
  restart <process-id> Restart a managed process
  logs <process-id> Print process logs
  install ...       Install or update shared Java, Node, and Go runtimes
  runtime ...       Search, list, or activate managed runtimes
  init-shell        Configure shared Bash/Zsh PATH and optional banner support
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
	case args[0] == "install":
		return a.install(ctx, args[1:])
	case args[0] == "runtime":
		return a.runtime(ctx, args[1:])
	case args[0] == "use":
		return a.runtime(ctx, append([]string{"use"}, args[1:]...))
	case args[0] == "init-shell":
		if len(args) == 2 && isHelp(args[1]) {
			_, err := fmt.Fprint(a.out, initShellHelp)
			return err
		}
		if len(args) != 1 {
			return errors.New("usage: ldr init-shell")
		}
		return a.initShell()
	case args[0] == "list":
		jsonOutput, query, recent, err := validateListArgs(args[1:])
		if err != nil {
			return err
		}
		return a.list(ctx, directory, jsonOutput, query, recent, false)
	case len(args) == 1 && args[0] == "refresh":
		return a.list(ctx, directory, false, "", false, true)
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
	case args[0] == "ps":
		jsonOutput, err := validatePSArgs(args[1:])
		if err != nil {
			return err
		}
		return a.ps(ctx, directory, jsonOutput)
	case args[0] == "stop":
		if len(args) != 2 {
			return errors.New("usage: ldr stop <process-id>")
		}
		return a.stop(ctx, directory, args[1])
	case args[0] == "cleanup":
		confirm, err := validateCleanupArgs(args[1:])
		if err != nil {
			return err
		}
		return a.cleanup(ctx, directory, confirm)
	case args[0] == "doctor":
		jsonOutput, err := validateDoctorArgs(args[1:])
		if err != nil {
			return err
		}
		return a.doctor(ctx, directory, jsonOutput)
	case args[0] == "alias":
		return a.alias(ctx, directory, args[1:])
	case args[0] == "restart":
		if len(args) != 2 {
			return errors.New("usage: ldr restart <process-id>")
		}
		return a.restart(ctx, directory, args[1])
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

const installHelp = `Install and update shared runtimes

Usage:
  ldr install                         Choose a runtime and version interactively
  ldr install <java|node|go> <version> Install or update one runtime family
  ldr install <java|node> --lts        Install or update the five newest LTS families
  ldr install go                       Choose a Go family (for example 1.26)
  ldr install go latest                Install the newest stable Go release
  ldr install go <version>             Install a Go family or exact release
  ldr install java --list [query]      Search available Java releases
  ldr install node --list [query]      Search available Node releases
  ldr install go --list [query]        Legacy alias for runtime search go
  ldr install all                      Install latest Java, Node, and Go families
  ldr install all --lts                Install five Java/Node LTS families plus latest Go

Examples:
  ldr install java 21
  ldr install node 24
  ldr install node --lts
  ldr install go 1.26
  ldr install go --list 1.26

Runtimes are installed below the LDR user runtime store. Installation currently
supports macOS arm64 and Intel Macs.
`

func (a App) install(ctx context.Context, args []string) error {
	if len(args) == 1 && isHelp(args[0]) {
		_, err := fmt.Fprint(a.out, installHelp)
		return err
	}
	if len(args) >= 2 && (strings.EqualFold(args[0], "java") || strings.EqualFold(args[0], "node") || strings.EqualFold(args[0], "go") || strings.EqualFold(args[0], "golang")) && (args[1] == "--list" || args[1] == "--search") {
		if len(args) > 3 {
			return errors.New("usage: ldr install go --list [query]")
		}
		query := ""
		if len(args) == 3 {
			query = args[2]
		}
		return a.listRuntimeVersions(ctx, strings.ToLower(args[0]), query)
	}
	request, choose, err := parseInstallArgs(args)
	if err != nil {
		return err
	}
	if choose {
		if !a.terminal {
			return errors.New("usage: ldr install <java|node|go> <version> or `ldr install <java|node> --lts`")
		}
		request, err = promptInstall(a.in, a.out)
		if err != nil {
			return err
		}
	}
	installed, err := runtimes.NewInstaller().Install(ctx, request)
	if err != nil {
		return err
	}
	selected := map[string]runtimes.InstalledRuntime{}
	for _, item := range installed {
		fmt.Fprintf(a.out, "Installed %s %s (%s)\n%s\n", item.Runtime, item.Family, item.Version, item.Path)
		selected[item.Runtime] = item
	}
	for _, runtimeName := range []string{"java", "node", "go"} {
		item, ok := selected[runtimeName]
		if !ok {
			continue
		}
		activation, err := runtimes.Activate(item.Runtime, item.Family)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "Activated %s %s for this shell via %s\n", activation.Runtime, activation.Family, strings.Join(activation.Links, ", "))
	}
	return nil
}

func (a App) listRuntimeVersions(ctx context.Context, runtimeName, query string) error {
	if runtimeName == "golang" {
		runtimeName = "go"
	}
	if runtimeName == "go" {
		versions, err := runtimes.NewInstaller().AvailableGoVersions(ctx, query)
		if err != nil {
			return err
		}
		if len(versions) == 0 {
			return fmt.Errorf("no stable Go releases match %q", query)
		}
		fmt.Fprintln(a.out, "RUNTIME\tVERSION\tLTS")
		for _, version := range versions {
			fmt.Fprintf(a.out, "go\t%s\t-\n", version)
		}
		return nil
	}
	versions, err := runtimes.NewInstaller().AvailableVersions(ctx, runtimeName, query)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		return fmt.Errorf("no %s releases match %q", runtimeName, query)
	}
	fmt.Fprintln(a.out, "RUNTIME\tVERSION\tLTS")
	for _, version := range versions {
		lts := ""
		if version.LTS {
			lts = "*"
		}
		fmt.Fprintf(a.out, "%s\t%s\t%s\n", version.Runtime, version.Version, lts)
	}
	return nil
}

const runtimeHelp = `Manage shell-active runtimes

Usage:
  ldr runtime list              List installed Java, Node, and Go families
  ldr runtime search <runtime> [query]
                                Search available Java, Node, or Go releases
  ldr runtime use <runtime> <version>
                                Activate one installed family via ~/bin links
  ldr runtime remove <runtime> <version>
                                Remove one installed family and its active links
  ldr use <runtime> <version>   Alias for ldr runtime use

Examples:
  ldr runtime list
  ldr runtime search go
  ldr runtime search go 1.27
  ldr runtime search java 21
  ldr runtime search node 24
  ldr runtime use java 21
  ldr runtime remove node 20
  ldr use node 24

LDR updates only symlinks in ~/bin for the selected runtime. Ensure ~/bin is
on PATH (it is already present in this environment).
`

func (a App) runtime(ctx context.Context, args []string) error {
	if len(args) == 0 || (len(args) == 1 && args[0] == "list") {
		installed, err := runtimes.ListInstalled()
		if err != nil {
			return err
		}
		if len(installed) == 0 {
			fmt.Fprintln(a.out, "No LDR-managed runtimes are installed. Run `ldr install --help`.")
			return nil
		}
		fmt.Fprintln(a.out, "RUNTIME\tFAMILY\tACTIVE\tPATH")
		for _, item := range installed {
			active := ""
			if item.Active {
				active = "*"
			}
			fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\n", item.Runtime, item.Family, active, item.Path)
		}
		return nil
	}
	if len(args) == 1 && isHelp(args[0]) {
		_, err := fmt.Fprint(a.out, runtimeHelp)
		return err
	}
	if len(args) >= 2 && (args[0] == "search" || args[0] == "available") {
		if len(args) > 3 || (args[1] != "java" && args[1] != "node" && args[1] != "go" && args[1] != "golang") {
			return errors.New("usage: ldr runtime search <java|node|go> [query]")
		}
		query := ""
		if len(args) == 3 {
			query = args[2]
		}
		return a.listRuntimeVersions(ctx, args[1], query)
	}
	if len(args) == 3 && args[0] == "remove" {
		if err := runtimes.Remove(args[1], args[2]); err != nil {
			return err
		}
		fmt.Fprintf(a.out, "Removed %s %s\n", args[1], args[2])
		return nil
	}
	if len(args) != 3 || args[0] != "use" {
		return errors.New("usage: ldr runtime <list|search|use|remove> [java|node|go] [version-or-query]")
	}
	activation, err := runtimes.Activate(args[1], args[2])
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Activated %s %s\n%s\n", activation.Runtime, activation.Family, strings.Join(activation.Links, "\n"))
	return nil
}

func (a App) initShell() error {
	result, err := runtimes.InitShell()
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Configured shared shell paths: %s\n", result.ShellPaths)
	fmt.Fprintf(a.out, "Optional banner location: %s/banner.sh\n", result.ShellHome)
	fmt.Fprintln(a.out, "Open a new shell or run `exec zsh -l` to apply it.")
	return nil
}

const initShellHelp = `Configure shared shell paths

Usage:
  ldr init-shell

Creates or extends ~/.shell_paths while retaining its existing content, then
makes ~/.bashrc and ~/.zshrc source it. The shared file keeps ~/bin,
/opt/homebrew/bin, and ~/.local/bin on PATH without duplicate entries.

If ~/Library/Application Support/local-dev-runner/shell/banner.sh exists, it is
sourced only in interactive shells. Missing banner.sh files are ignored.
`

func parseInstallArgs(args []string) (runtimes.InstallRequest, bool, error) {
	if len(args) == 0 {
		return runtimes.InstallRequest{}, true, nil
	}
	request := runtimes.InstallRequest{Runtime: strings.ToLower(args[0])}
	explicitLatest := false
	if request.Runtime == "all" {
		request.All = true
		request.Runtime = ""
	}
	if request.Runtime != "" && request.Runtime != "java" && request.Runtime != "node" && request.Runtime != "go" && request.Runtime != "golang" {
		return runtimes.InstallRequest{}, false, fmt.Errorf("unsupported runtime %q; choose java, node, go, or all", args[0])
	}
	for _, argument := range args[1:] {
		switch argument {
		case "--lts":
			request.LTS = true
		case "--latest":
			if request.Version != "" || request.LTS {
				return runtimes.InstallRequest{}, false, errors.New("--latest cannot be combined with a version or --lts")
			}
			explicitLatest = true
		case "--all":
			if request.Runtime != "" {
				return runtimes.InstallRequest{}, false, errors.New("--all cannot be combined with a named runtime")
			}
			request.All = true
		default:
			if request.Version != "" || strings.HasPrefix(argument, "-") {
				return runtimes.InstallRequest{}, false, errors.New("usage: ldr install <java|node|go> <version> [--lts]")
			}
			if strings.EqualFold(argument, "latest") {
				explicitLatest = true
			} else {
				request.Version = argument
			}
		}
	}
	if request.All && request.Version != "" {
		return runtimes.InstallRequest{}, false, errors.New("an all-runtime install does not accept a version")
	}
	if request.Runtime == "" && !request.All {
		return runtimes.InstallRequest{}, false, errors.New("usage: ldr install all [--lts]")
	}
	if explicitLatest {
		if request.LTS {
			return runtimes.InstallRequest{}, false, errors.New("latest cannot be combined with --lts")
		}
		return request, false, nil
	}
	if request.Runtime != "" && request.Version == "" && !request.LTS {
		return request, true, nil
	}
	return request, false, nil
}

func promptInstall(in io.Reader, out io.Writer) (runtimes.InstallRequest, error) {
	fmt.Fprint(out, "Runtime [java/node/go/all]: ")
	var runtimeName string
	if _, err := fmt.Fscan(in, &runtimeName); err != nil {
		return runtimes.InstallRequest{}, err
	}
	if strings.EqualFold(runtimeName, "all") {
		fmt.Fprint(out, "Mode [latest/lts]: ")
		var mode string
		if _, err := fmt.Fscan(in, &mode); err != nil {
			return runtimes.InstallRequest{}, err
		}
		if !strings.EqualFold(mode, "latest") && !strings.EqualFold(mode, "lts") {
			return runtimes.InstallRequest{}, errors.New("mode must be latest or lts")
		}
		return runtimes.InstallRequest{All: true, LTS: strings.EqualFold(mode, "lts")}, nil
	}
	fmt.Fprint(out, "Version [latest/lts/major or Go family]: ")
	var version string
	if _, err := fmt.Fscan(in, &version); err != nil {
		return runtimes.InstallRequest{}, err
	}
	if strings.EqualFold(version, "lts") {
		return runtimes.InstallRequest{Runtime: strings.ToLower(runtimeName), LTS: true}, nil
	}
	if strings.EqualFold(version, "latest") {
		return runtimes.InstallRequest{Runtime: strings.ToLower(runtimeName)}, nil
	}
	return runtimes.InstallRequest{Runtime: strings.ToLower(runtimeName), Version: version}, nil
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
		for {
			runResult, err := a.runFromTUI(ctx, directory, selection.TaskID)
			if err != nil {
				return err
			}
			action, err := tui.ShowRunResult(a.in, a.out, runResult)
			if err != nil {
				return err
			}
			switch action {
			case tui.ResultRerun:
				continue
			case tui.ResultBack:
				break
			default:
				return nil
			}
			break
		}
	}
}

// runFromTUI keeps Ctrl+C scoped to the foreground child task. Terminals send
// that signal to both the Gradle process and LDR's foreground process group;
// registering it here prevents LDR itself from exiting before it can reopen
// the selected task pane.
func (a App) runFromTUI(ctx context.Context, directory, taskID string) (tui.RunResult, error) {
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	if err := a.initialize(ctx, directory, false); err != nil {
		return tui.RunResult{}, err
	}
	root, err := a.findRoot(directory)
	if err != nil {
		return tui.RunResult{}, fmt.Errorf("find project root: %w", err)
	}
	task, options, err := a.resolveRunnable(root, taskID)
	if err != nil {
		return tui.RunResult{}, err
	}
	logFile, logPath, err := newForegroundLog(state.NewLayout(root), task.ID)
	if err != nil {
		return tui.RunResult{}, err
	}
	defer logFile.Close()
	fmt.Fprintf(logFile, "[LDR] Started %s at %s\n\n", task.ID, time.Now().Format(time.RFC3339))
	runErr := execution.RunWithOptions(ctx, root, task, io.MultiWriter(a.out, logFile), io.MultiWriter(a.errOut, logFile), options)
	if runErr != nil {
		fmt.Fprintf(logFile, "\n[LDR] Stopped or failed: %v\n", runErr)
	} else {
		fmt.Fprintf(logFile, "\n[LDR] Finished successfully\n")
	}
	return tui.RunResult{Task: task, LogPath: logPath, Err: runErr}, nil
}

func newForegroundLog(layout state.Layout, taskID string) (*os.File, string, error) {
	directory := filepath.Join(layout.State, "logs", "foreground")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, "", err
	}
	name := strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(taskID)
	path := filepath.Join(directory, fmt.Sprintf("%s-%d.log", name, time.Now().UnixNano()))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, "", err
	}
	return file, path, nil
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
	err = execution.RunWithOptions(ctx, root, task, a.out, a.errOut, options)
	if err == nil {
		_ = recordRecent(root, task.ID)
	}
	return err
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
	_ = recordRecent(root, task.ID)
	fmt.Fprintf(a.out, "Started %s\nPID: %d\nProcess: %s\nLogs: %s\n", record.TaskID, record.PID, record.ID, record.LogPath)
	return nil
}

func (a App) ps(ctx context.Context, directory string, jsonOutput bool) error {
	if err := a.initialize(ctx, directory, false); err != nil {
		return err
	}
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	records := process.New(state.NewLayout(root)).Reconcile()
	if jsonOutput {
		if records == nil {
			records = []process.Record{}
		}
		return json.NewEncoder(a.out).Encode(records)
	}
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

func (a App) cleanup(ctx context.Context, directory string, confirm bool) error {
	if err := a.initialize(ctx, directory, false); err != nil {
		return err
	}
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	manager := process.New(state.NewLayout(root))
	records := manager.Reconcile()
	orphaned := make([]process.Record, 0)
	for _, record := range records {
		if record.Status == "ORPHANED" {
			orphaned = append(orphaned, record)
		}
	}
	if len(orphaned) == 0 {
		fmt.Fprintln(a.out, "No orphaned processes.")
		return nil
	}
	if !confirm {
		fmt.Fprintln(a.out, "Orphaned processes (nothing terminated):")
		fmt.Fprintln(a.out, "PROCESS\tPID\tPGID\tTASK")
		for _, record := range orphaned {
			fmt.Fprintf(a.out, "%s\t%d\t%d\t%s\n", record.ID, record.PID, record.PGID, record.TaskID)
		}
		fmt.Fprintln(a.out, "Run `ldr cleanup --yes` to terminate these LDR-managed process groups.")
		return nil
	}
	for _, record := range orphaned {
		if _, err := manager.Stop(record.ID); err != nil {
			return fmt.Errorf("cleanup %s: %w", record.ID, err)
		}
		fmt.Fprintf(a.out, "Stopped %s (PID %d, PGID %d)\n", record.ID, record.PID, record.PGID)
	}
	return nil
}

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

func (a App) doctor(ctx context.Context, directory string, jsonOutput bool) error {
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	layout := state.NewLayout(root)
	checks := make([]doctorCheck, 0, 8)
	add := func(name, status, message, hint string) {
		checks = append(checks, doctorCheck{Name: name, Status: status, Message: message, Hint: hint})
	}
	if config, err := project.LoadConfig(root); err != nil {
		add("project-config", "ERROR", err.Error(), "Fix .ldr/project.toml and run ldr doctor again.")
	} else if config.DefaultProfile != "" {
		add("project-config", "OK", fmt.Sprintf(".ldr/project.toml loaded; default profile %s", config.DefaultProfile), "")
	} else {
		add("project-config", "OK", "No project overrides configured", "")
	}
	if status, err := gitignore.Check(ctx, a.git, root); err != nil {
		add("gitignore", "ERROR", err.Error(), "Ensure Git is available and .ldr/ can be checked.")
	} else if !status.InRepository {
		add("gitignore", "WARN", "Project is not inside a Git work tree", "Add .ldr/ to .gitignore when using Git.")
	} else if !status.Ignored {
		add("gitignore", "WARN", ".ldr/ is not ignored by Git", "Add .ldr/ to .gitignore.")
	} else {
		add("gitignore", "OK", ".ldr/ is ignored by Git", "")
	}
	missing := make([]string, 0)
	for name, path := range map[string]string{"cache": layout.Cache, "profiles": layout.Profiles, "state": layout.State} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		add("state", "WARN", "Missing .ldr directories: "+strings.Join(missing, ", "), "Run ldr init to create local state.")
	} else {
		add("state", "OK", ".ldr cache, profiles, and state directories are present", "")
	}
	if records := process.New(layout).List(); len(records) == 0 {
		add("processes", "OK", "No managed process metadata found", "")
	} else {
		orphaned := 0
		for _, record := range records {
			if record.Status == "ORPHANED" {
				orphaned++
			}
		}
		if orphaned > 0 {
			add("processes", "WARN", fmt.Sprintf("%d orphaned managed process record(s)", orphaned), "Run ldr cleanup to review them.")
		} else {
			add("processes", "OK", fmt.Sprintf("%d managed process record(s)", len(records)), "")
		}
	}
	if installed, err := runtimes.ListInstalled(); err != nil {
		add("runtimes", "WARN", err.Error(), "Run ldr runtime list or install a required runtime.")
	} else {
		add("runtimes", "OK", fmt.Sprintf("%d installed runtime family(ies)", len(installed)), "")
	}
	if jsonOutput {
		return json.NewEncoder(a.out).Encode(checks)
	}
	fmt.Fprintln(a.out, "CHECK\tSTATUS\tMESSAGE")
	for _, check := range checks {
		fmt.Fprintf(a.out, "%s\t%s\t%s\n", check.Name, check.Status, check.Message)
		if check.Hint != "" {
			fmt.Fprintf(a.out, "\t\tHint: %s\n", check.Hint)
		}
	}
	return nil
}

func (a App) alias(ctx context.Context, directory string, args []string) error {
	if len(args) != 0 && len(args) != 2 {
		return errors.New("usage: ldr alias [name target]")
	}
	if err := a.initialize(ctx, directory, len(args) == 0); err != nil {
		return err
	}
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	path := filepath.Join(state.NewLayout(root).State, "aliases.json")
	aliases, err := state.ReadJSON[map[string]string](path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read aliases: %w", err)
	}
	if aliases == nil {
		aliases = map[string]string{}
	}
	if len(args) == 0 {
		if len(aliases) == 0 {
			fmt.Fprintln(a.out, "No aliases found.")
			return nil
		}
		keys := make([]string, 0, len(aliases))
		for key := range aliases {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		fmt.Fprintln(a.out, "NAME\tTARGET")
		for _, key := range keys {
			fmt.Fprintf(a.out, "%s\t%s\n", key, aliases[key])
		}
		return nil
	}
	if _, err := profile.Filename(args[0]); err != nil {
		return fmt.Errorf("invalid alias name: %w", err)
	}
	aliases[args[0]] = args[1]
	if err := state.WriteJSON(path, aliases); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Created alias %q -> %s\n", args[0], args[1])
	return nil
}

func (a App) restart(ctx context.Context, directory, processID string) error {
	if err := a.initialize(ctx, directory, false); err != nil {
		return err
	}
	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	layout := state.NewLayout(root)
	manager := process.New(layout)
	var existing process.Record
	for _, record := range manager.List() {
		if record.ID == processID {
			existing = record
			break
		}
	}
	if existing.ID == "" {
		return fmt.Errorf("process %q was not found", processID)
	}
	task, options, err := a.resolveRunnable(root, existing.TaskID)
	if err != nil {
		return err
	}
	restarted, err := manager.Restart(ctx, processID, task, options)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Restarted %s\nPID: %d\nProcess: %s\nLogs: %s\n", restarted.TaskID, restarted.PID, restarted.ID, restarted.LogPath)
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
	aliases := map[string]string{}
	if loaded, err := state.ReadJSON[map[string]string](filepath.Join(layout.State, "aliases.json")); err == nil {
		aliases = loaded
		if target, ok := aliases[taskID]; ok {
			taskID = target
		}
	}
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
			env, err := profile.ResolveEnv(root, loaded.EnvFrom, loaded.Env)
			if err != nil {
				return domain.Task{}, execution.Options{}, err
			}
			return task, execution.Options{Env: env, PrependArgs: loaded.PrependArgs, AppendArgs: loaded.AppendArgs}, nil
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
		if len(loaded.EnvFrom) > 0 {
			fmt.Fprintf(a.out, "Environment files: %s\n", strings.Join(loaded.EnvFrom, ", "))
		}
		if len(loaded.Env) > 0 {
			fmt.Fprintln(a.out, "Environment:")
			for key, value := range loaded.Env {
				masked := "********"
				if value == "" {
					masked = ""
				}
				fmt.Fprintf(a.out, "  %s=%s\n", key, masked)
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

func (a App) list(ctx context.Context, directory string, jsonOutput bool, query string, recent, force bool) error {
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
	if query != "" {
		matches := search.Tasks(result.Tasks, query)
		if jsonOutput {
			return json.NewEncoder(a.out).Encode(matches)
		}
		filtered := make([]domain.Task, 0, len(matches))
		for _, match := range matches {
			filtered = append(filtered, match.Task)
		}
		printTasks(a.out, filtered)
		return nil
	}
	if recent {
		byID := make(map[string]domain.Task, len(result.Tasks))
		for _, task := range result.Tasks {
			byID[task.ID] = task
		}
		ordered := make([]domain.Task, 0)
		for _, id := range recentTasks(root) {
			if task, ok := byID[id]; ok {
				ordered = append(ordered, task)
			}
		}
		if jsonOutput {
			return json.NewEncoder(a.out).Encode(ordered)
		}
		printTasks(a.out, ordered)
		return nil
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

func recordRecent(root, taskID string) error {
	path := filepath.Join(state.NewLayout(root).State, "recent.json")
	recent, err := state.ReadJSON[[]string](path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ordered := []string{taskID}
	for _, existing := range recent {
		if existing != taskID && len(ordered) < 20 {
			ordered = append(ordered, existing)
		}
	}
	return state.WriteJSON(path, ordered)
}

func recentTasks(root string) []string {
	recent, err := state.ReadJSON[[]string](filepath.Join(state.NewLayout(root).State, "recent.json"))
	if err != nil {
		return nil
	}
	return recent
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

func validateListArgs(args []string) (bool, string, bool, error) {
	if len(args) == 0 {
		return false, "", false, nil
	}
	jsonOutput := false
	query := ""
	recent := false
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			jsonOutput = true
		case "--search":
			if index+1 >= len(args) || args[index+1] == "" {
				return false, "", false, fmt.Errorf("usage: ldr list [--json] [--recent] [--search query]")
			}
			query = args[index+1]
			index++
		case "--recent":
			recent = true
		default:
			return false, "", false, fmt.Errorf("usage: ldr list [--json] [--recent] [--search query]")
		}
	}
	return jsonOutput, query, recent, nil
}

func validatePSArgs(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--json" {
		return true, nil
	}
	return false, fmt.Errorf("usage: ldr ps [--json]")
}

func validateCleanupArgs(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && (args[0] == "--yes" || args[0] == "-y") {
		return true, nil
	}
	return false, fmt.Errorf("usage: ldr cleanup [--yes, -y]")
}

func validateDoctorArgs(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--json" {
		return true, nil
	}
	return false, fmt.Errorf("usage: ldr doctor [--json]")
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
