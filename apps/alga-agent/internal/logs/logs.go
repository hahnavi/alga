// Package logs implements the `alga-agent logs` subcommand, which prints and
// optionally follows the agent's log file, inspired by kubectl logs and
// docker logs.
package logs

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"alga-agent/internal/config"
	"alga-agent/internal/logging"
)

// Run executes the logs subcommand.
func Run(w io.Writer, args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	follow := fs.Bool("f", false, "follow log output")
	tail := fs.Int("n", 0, "number of lines to show from the end (0 = all)")
	fs.IntVar(tail, "tail", 0, "alias for -n")
	file := fs.String("file", "", "log file path (default: resolved from config)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	path := *file
	if path == "" {
		path = resolveLogFile()
	}
	if path == logging.FileLoggingDisabled {
		return fmt.Errorf("file logging is disabled (logging.file = %q); logs go to stderr/journal", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	if *tail > 0 {
		if err := printTail(w, f, *tail); err != nil {
			return err
		}
	} else {
		if _, err := io.Copy(w, f); err != nil {
			return fmt.Errorf("read log file: %w", err)
		}
	}

	if !*follow {
		return nil
	}
	return followLog(w, path, f)
}

func resolveLogFile() string {
	if cfg, err := config.Load(""); err == nil && cfg.Logging.File != "" {
		return cfg.Logging.File
	}
	return logging.DefaultLogFile()
}

func printTail(w io.Writer, f *os.File, n int) error {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lines := make([]string, 0, min(n, 1024))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan log file: %w", err)
	}
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
	return nil
}

func followLog(w io.Writer, path string, f *os.File) error {
	pos, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("seek: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		newInfo, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat %s: %w", path, err)
		}

		if !os.SameFile(info, newInfo) || newInfo.Size() < pos {
			f.Close()
			f, err = os.Open(path)
			if err != nil {
				return fmt.Errorf("reopen %s: %w", path, err)
			}
			pos = 0
			info = newInfo
		}

		if newInfo.Size() > pos {
			if _, err := f.Seek(pos, io.SeekStart); err != nil {
				return fmt.Errorf("seek: %w", err)
			}
			n, err := io.Copy(w, f)
			if err != nil {
				return fmt.Errorf("read: %w", err)
			}
			pos += n
		}
	}
	return nil
}
