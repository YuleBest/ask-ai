package i18n

import "testing"

func TestDetectLanguageFromEnvironment(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Language
	}{
		{
			name: "Chinese locale",
			env:  map[string]string{"LANG": "zh_CN.UTF-8"},
			want: ChineseSimplified,
		},
		{
			name: "English locale",
			env:  map[string]string{"LANG": "en_US.UTF-8"},
			want: English,
		},
		{
			name: "POSIX falls back to English",
			env:  map[string]string{"LANG": "C.UTF-8"},
			want: English,
		},
		{
			name: "LC_ALL has higher priority",
			env: map[string]string{
				"LANG":        "zh_CN.UTF-8",
				"LC_ALL":      "en_US.UTF-8",
				"LC_MESSAGES": "zh_CN.UTF-8",
			},
			want: English,
		},
		{
			name: "LC_MESSAGES falls back before LANG",
			env: map[string]string{
				"LANG":        "en_US.UTF-8",
				"LC_MESSAGES": "zh_CN.UTF-8",
			},
			want: ChineseSimplified,
		},
		{
			name: "LANGUAGE has highest priority",
			env: map[string]string{
				"LANG":     "en_US.UTF-8",
				"LANGUAGE": "zh_CN:en_US",
			},
			want: ChineseSimplified,
		},
		{
			name: "LANGUAGE skips unsupported entries",
			env: map[string]string{
				"LANGUAGE": "fr_FR:zh_CN:en_US",
			},
			want: ChineseSimplified,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Detect(func(key string) string {
				return test.env[key]
			})
			if got != test.want {
				t.Fatalf("Detect() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolveLanguageSettings(t *testing.T) {
	getenv := func(key string) string {
		return map[string]string{"LC_ALL": "zh_CN.UTF-8"}[key]
	}

	settings := Resolve(getenv, English, "")
	if settings.Output != English {
		t.Fatalf("output = %q, want explicit English", settings.Output)
	}
	if settings.Prompt != ChineseSimplified {
		t.Fatalf("prompt = %q, want inherited Chinese", settings.Prompt)
	}
}

func TestTranslateForLanguage(t *testing.T) {
	if got, want := TFor(English, "已思考 %d 秒...", 5), "Thinking for 5 seconds..."; got != want {
		t.Fatalf("TFor() = %q, want %q", got, want)
	}
	if got, want := TFor(ChineseSimplified, "已思考 %d 秒...", 5), "已思考 5 秒..."; got != want {
		t.Fatalf("TFor() = %q, want %q", got, want)
	}
}

func TestCurrentOutputLanguage(t *testing.T) {
	previous := OutputLanguage()
	t.Cleanup(func() { SetOutputLanguage(previous) })

	SetOutputLanguage(English)
	if got, want := T("暂无任务"), "No tasks."; got != want {
		t.Fatalf("T() = %q, want %q", got, want)
	}
}

func TestCurrentPromptLanguage(t *testing.T) {
	previous := PromptLanguage()
	t.Cleanup(func() { SetPromptLanguage(previous) })

	SetPromptLanguage(English)
	if got := PromptLanguage(); got != English {
		t.Fatalf("PromptLanguage() = %q, want English", got)
	}
}

func TestDynamicErrorUsesCurrentLanguage(t *testing.T) {
	previous := OutputLanguage()
	t.Cleanup(func() { SetOutputLanguage(previous) })

	err := NewDynamic("没有可追问的上一次对话")
	SetOutputLanguage(ChineseSimplified)
	if got, want := err.Error(), "没有可追问的上一次对话"; got != want {
		t.Fatalf("Chinese error = %q, want %q", got, want)
	}
	SetOutputLanguage(English)
	if got, want := err.Error(), "There is no previous conversation to follow"; got != want {
		t.Fatalf("English error = %q, want %q", got, want)
	}
}
