package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// runExternal executes an optional tool with inherited stdio. It never invokes
// a shell, so paths and arguments cannot be re-interpreted by cmd.exe.
func runExternal(ctx context.Context, binary string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("%s exited with code %d", filepath.Base(binary), exitErr.ExitCode())
		}
		return fmt.Errorf("run %s: %w", filepath.Base(binary), err)
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
