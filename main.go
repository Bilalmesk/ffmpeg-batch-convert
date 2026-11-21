// Command ffbatch is a fast, friendly FFmpeg batch converter with a CLI and a
// local web dashboard.
package main

import (
	"os"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
