package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type devRunner struct {
	args      []string
	stdout    io.Writer
	stderr    io.Writer
	fixedPort string
	child     *exec.Cmd
	cancel    context.CancelFunc
	done      chan error
}

type watchedFile struct {
	modTime time.Time
	size    int64
}

func runDev(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			fmt.Fprintf(stderr, "Usage: %s dev [serve flags] <media-file-or-directory>...\n\n", cliName)
			fmt.Fprintln(stderr, "Runs the web UI through `go run` and restarts it when Go or embedded web files change.")
			return nil
		}
	}

	runner := &devRunner{args: append([]string{}, args...), stdout: stdout, stderr: stderr}
	snapshot, err := sourceSnapshot()
	if err != nil {
		return err
	}
	if err := runner.start(); err != nil {
		return err
	}
	defer runner.stop()

	fmt.Fprintln(stderr, "Dev watcher: restart on changes under cmd/, internal/, go.mod, go.sum, .env, and .env.local.")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		nextSnapshot, err := sourceSnapshot()
		if err != nil {
			fmt.Fprintf(stderr, "Dev watcher warning: %v\n", err)
			continue
		}
		if !snapshotChanged(snapshot, nextSnapshot) {
			if runner.exited() {
				fmt.Fprintln(stderr, "Dev server exited; waiting for the next source change.")
			}
			snapshot = nextSnapshot
			continue
		}
		fmt.Fprintln(stderr, "Source change detected; restarting web UI.")
		runner.stop()
		if err := runner.start(); err != nil {
			fmt.Fprintf(stderr, "Dev restart failed: %v\n", err)
		}
		snapshot = nextSnapshot
	}
	return nil
}

func (r *devRunner) start() error {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	cmdArgs := append([]string{"run", "./cmd/clip-indexer", "serve"}, r.serveArgs()...)
	cmd := exec.CommandContext(ctx, "go", cmdArgs...)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}
	r.child = cmd
	r.done = make(chan error, 1)
	go r.forwardOutput(stdoutPipe, r.stdout)
	go r.forwardOutput(stderrPipe, r.stderr)
	go func() {
		r.done <- cmd.Wait()
	}()
	return nil
}

func (r *devRunner) serveArgs() []string {
	args := append([]string{}, r.args...)
	if r.fixedPort == "" || !hasPortZero(args) {
		return args
	}
	for index := 0; index < len(args); index++ {
		if args[index] == "--port" && index+1 < len(args) && args[index+1] == "0" {
			args[index+1] = r.fixedPort
			return args
		}
		if args[index] == "-port" && index+1 < len(args) && args[index+1] == "0" {
			args[index+1] = r.fixedPort
			return args
		}
		if args[index] == "--port=0" {
			args[index] = "--port=" + r.fixedPort
			return args
		}
		if args[index] == "-port=0" {
			args[index] = "-port=" + r.fixedPort
			return args
		}
	}
	return args
}

func (r *devRunner) forwardOutput(reader io.Reader, writer io.Writer) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		r.capturePort(line)
		fmt.Fprintln(writer, line)
	}
}

func (r *devRunner) capturePort(line string) {
	if r.fixedPort != "" || !hasPortZero(r.args) || !strings.Contains(line, "Clip Atlas web UI:") {
		return
	}
	_, rawURL, ok := strings.Cut(line, "Clip Atlas web UI:")
	if !ok {
		return
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return
	}
	if port := parsed.Port(); port != "" {
		r.fixedPort = port
	}
}

func (r *devRunner) stop() {
	if r.cancel == nil {
		return
	}
	if r.child != nil && r.child.Process != nil {
		_ = r.child.Process.Signal(os.Interrupt)
	}
	r.cancel()
	if r.done == nil {
		r.cancel = nil
		r.child = nil
		return
	}
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
		if r.child != nil && r.child.Process != nil {
			_ = r.child.Process.Kill()
		}
		<-r.done
	}
	r.cancel = nil
	r.child = nil
}

func (r *devRunner) exited() bool {
	if r.done == nil {
		return true
	}
	select {
	case err := <-r.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(r.stderr, "Dev server exited: %v\n", err)
		}
		r.done = nil
		r.cancel = nil
		r.child = nil
		return true
	default:
		return false
	}
}

func hasPortZero(args []string) bool {
	for index := 0; index < len(args); index++ {
		if (args[index] == "--port" || args[index] == "-port") && index+1 < len(args) && args[index+1] == "0" {
			return true
		}
		if args[index] == "--port=0" || args[index] == "-port=0" {
			return true
		}
	}
	return false
}

func sourceSnapshot() (map[string]watchedFile, error) {
	paths := map[string]watchedFile{}
	for _, root := range []string{"cmd", "internal"} {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if !isWatchedSource(path) {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			paths[path] = watchedFile{modTime: info.ModTime(), size: info.Size()}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	for _, path := range []string{"go.mod", "go.sum", ".env", ".env.local"} {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		paths[path] = watchedFile{modTime: info.ModTime(), size: info.Size()}
	}
	return paths, nil
}

func isWatchedSource(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".html", ".css", ".js", ".svg":
		return true
	default:
		return false
	}
}

func snapshotChanged(previous map[string]watchedFile, next map[string]watchedFile) bool {
	if len(previous) != len(next) {
		return true
	}
	for path, oldFile := range previous {
		newFile, ok := next[path]
		if !ok || !oldFile.modTime.Equal(newFile.modTime) || oldFile.size != newFile.size {
			return true
		}
	}
	return false
}
