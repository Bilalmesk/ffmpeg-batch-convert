// Package hashutil provides streaming file hashes for chain-of-custody
// manifests.
package hashutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// SHA256 returns the lower-case hex SHA-256 of a file, honouring cancellation
// between chunks.
func SHA256(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open for hashing: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	buffer := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			if _, err := hasher.Write(buffer[:n]); err != nil {
				return "", err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("read for hashing: %w", readErr)
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
