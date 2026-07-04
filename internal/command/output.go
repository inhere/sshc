package command

import (
	"io"
	"os"

	"github.com/gookit/gcli/v3"
)

var commandOutput io.Writer = os.Stdout

func cmdOutput(_ *gcli.Command) io.Writer {
	if commandOutput == nil {
		return os.Stdout
	}
	return commandOutput
}

func setCommandOutputForTest(out io.Writer) func() {
	old := commandOutput
	commandOutput = out
	return func() { commandOutput = old }
}
