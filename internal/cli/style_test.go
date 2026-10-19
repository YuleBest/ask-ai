package cli

import "testing"

func TestOutputStyle(t *testing.T) {
	enabled := outputStyle{enabled: true}
	if got := enabled.boldCyan("[user]"); got != ansiBoldCyan+"[user]"+ansiReset {
		t.Fatalf("boldCyan output = %q", got)
	}
	if got := enabled.boldGreen("[assistant]"); got != ansiBoldGreen+"[assistant]"+ansiReset {
		t.Fatalf("boldGreen output = %q", got)
	}
	if got := enabled.gray("思考"); got != ansiGray+"思考"+ansiReset {
		t.Fatalf("gray output = %q", got)
	}

	disabled := outputStyle{}
	if got := disabled.cyan("text"); got != "text" {
		t.Fatalf("disabled output = %q, want plain text", got)
	}
}
