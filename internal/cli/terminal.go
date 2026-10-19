package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strconv"
)

type terminalIDProvider func(stdout, stderr io.Writer) (string, error)

func detectTerminalSession(stdout, stderr io.Writer) (string, error) {
	var terminal *os.File
	for _, writer := range []io.Writer{stdout, stderr} {
		file, ok := writer.(*os.File)
		if ok && isTerminal(file) {
			terminal = file
			break
		}
	}
	if terminal == nil {
		return "", nil
	}

	return platformTerminalID(terminal)
}

func terminalKey(terminalPath string, sessionID int) string {
	sum := sha256.Sum256([]byte(terminalPath + "|" + strconv.Itoa(sessionID)))
	return hex.EncodeToString(sum[:16])
}
