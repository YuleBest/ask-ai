package cli

import (
	"fmt"
	"io"
)

var Version = "v0.1.0-alpha.2"

func runVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stdout, Version)
		return 0
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		printVersionUsage(stdout)
		return 0
	}
	printErrorf(stderr, "version 不接受参数")
	printVersionUsage(stderr)
	return 2
}
