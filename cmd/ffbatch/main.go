// Command ffbatch is the `go install` entry point. Release builds use the
// repository-root main package; both call the same CLI.
package main

import (
	"os"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
