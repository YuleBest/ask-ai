package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/YuleBest/ask-ai/internal/chat"
	"github.com/YuleBest/ask-ai/internal/config"
	"github.com/YuleBest/ask-ai/internal/conversation"
	"github.com/YuleBest/ask-ai/internal/i18n"
	"github.com/YuleBest/ask-ai/internal/render"
	"github.com/YuleBest/ask-ai/internal/terminaltext"
	"github.com/YuleBest/ask-ai/internal/websearch"
	"golang.org/x/term"
)

var thinkingFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type configPathFunc func() (string, error)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWithTerminalInput(ctx, args, os.Stdin, stdout, stderr, config.DefaultPath, detectTerminalSession)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, configPath configPathFunc) int {
	return runWithTerminal(ctx, args, stdout, stderr, configPath, detectTerminalSession)
}

func runWithTerminal(ctx context.Context, args []string, stdout, stderr io.Writer, configPath configPathFunc, terminalID terminalIDProvider) int {
	return runWithTerminalInput(ctx, args, os.Stdin, stdout, stderr, configPath, terminalID)
}

func runWithTerminalInput(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, configPath configPathFunc, terminalID terminalIDProvider) int {
	allowLanguageFallback := languageFallbackAllowed(args)
	if err := configureLanguage(configPath); err != nil && !allowLanguageFallback {
		printErrorf(stderr, "%v", err)
		return 1
	}
	if len(args) > 0 && args[0] == "help" {
		return runHelp(args[1:], stdout, stderr)
	}
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version" || args[0] == "-v") {
		return runVersion(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "model" {
		return runModel(ctx, args[1:], stdout, stderr, configPath, terminalID)
	}
	if len(args) > 0 && args[0] == "mu" {
		return runModel(ctx, append([]string{"use"}, args[1:]...), stdout, stderr, configPath, terminalID)
	}
	if len(args) > 0 && args[0] == "md" {
		return runModel(ctx, append([]string{"default"}, args[1:]...), stdout, stderr, configPath, terminalID)
	}
	if len(args) > 0 && args[0] == "ml" {
		return runModel(ctx, append([]string{"list"}, args[1:]...), stdout, stderr, configPath, terminalID)
	}
	if len(args) > 0 && args[0] == "config" {
		return runConfig(ctx, args[1:], stdout, stderr, configPath)
	}
	if len(args) > 0 && args[0] == "tool" {
		return runTool(args[1:], stdout, stderr, configPath)
	}
	if len(args) > 0 && args[0] == "task" {
		return runTaskContext(ctx, args[1:], stdout, stderr)
	}
	if len(args) > 0 {
		switch args[0] {
		case "tl":
			return runTaskContext(ctx, append([]string{"list"}, args[1:]...), stdout, stderr)
		case "tv":
			return runTaskContext(ctx, append([]string{"view"}, args[1:]...), stdout, stderr)
		case "ts":
			return runTaskContext(ctx, append([]string{"switch"}, args[1:]...), stdout, stderr)
		case "tr":
			return runTaskContext(ctx, append([]string{"remove"}, args[1:]...), stdout, stderr)
		}
	}

	opts, err := parseArgs(args)
	if errors.Is(err, errVersionRequested) {
		return runVersion(nil, stdout, stderr)
	}
	if errors.Is(err, errHelpRequested) {
		printUsage(stdout)
		return 0
	}
	if err != nil {
		printErrorf(stderr, "%v", err)
		printUsage(stderr)
		return 2
	}
	pictures, err := loadImageAttachments(opts.pictures)
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}

	terminalSessionID := ""
	if opts.bind || !opts.follow {
		terminalSessionID, err = terminalID(stdout, stderr)
		if err != nil {
			printErrorf(stderr, "%v", err)
			return 1
		}
		if opts.bind && terminalSessionID == "" {
			printErrorf(stderr, "绑定仅在交互终端中生效")
			return 1
		}
	}
	if opts.bind && !isInteractiveInput(stdin) {
		printErrorf(stderr, "绑定需要从交互终端读取问题")
		return 1
	}

	store, err := conversation.DefaultStore()
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	var bindingStore conversation.BindingStore
	if terminalSessionID != "" {
		bindingStore = conversation.NewBindingStore(store.Root, terminalSessionID)
	}

	var session conversation.Session
	setCurrent := false
	if opts.bind {
		if opts.bindHash != "" {
			session, err = store.Load(opts.bindHash)
		} else {
			session, err = store.LoadLast()
		}
		if err != nil {
			printErrorf(stderr, "%v", err)
			return 1
		}
		if err := bindingStore.Save(session.ID); err != nil {
			printErrorf(stderr, "%v", err)
			return 1
		}
		defer func() {
			if err := bindingStore.Remove(); err != nil {
				printErrorf(stderr, "解除终端绑定失败: %v", err)
			}
		}()
	} else if opts.follow {
		if opts.followHash != "" {
			session, err = store.Load(opts.followHash)
		} else {
			session, err = store.LoadLast()
			setCurrent = true
		}
		if err != nil {
			printErrorf(stderr, "%v", err)
			return 1
		}
	} else if terminalSessionID != "" {
		boundID, bindErr := bindingStore.Load()
		switch {
		case bindErr == nil:
			session, err = store.Load(boundID)
			if err != nil {
				bindingStore.Remove()
				printErrorf(stderr, "终端绑定的任务不可用: %v", err)
				return 1
			}
		case errors.Is(bindErr, conversation.ErrNotBound):
			session, err = conversation.NewSession()
			setCurrent = true
		default:
			printErrorf(stderr, "%v", bindErr)
			return 1
		}
		if err != nil {
			printErrorf(stderr, "%v", err)
			return 1
		}
	} else {
		session, err = conversation.NewSession()
		setCurrent = true
		if err != nil {
			printErrorf(stderr, "%v", err)
			return 1
		}
	}

	defaultModelPath, err := configPath()
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	configDir := filepath.Dir(defaultModelPath)
	metaPath := config.MetaPath(defaultModelPath)
	meta, _, err := config.LoadOrCreateMeta(metaPath)
	if err != nil {
		printErrorf(stderr, "%v: %s", err, metaPath)
		return 1
	}
	modelsPath, err := meta.PathFor("models", configDir)
	if err != nil {
		printErrorf(stderr, "%v: %s", err, metaPath)
		return 1
	}
	systemPromptPath, err := meta.PathFor("systemPrompt", configDir)
	if err != nil {
		printErrorf(stderr, "%v: %s", err, metaPath)
		return 1
	}
	models, created, err := config.LoadOrCreateModels(modelsPath)
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	languagePath, err := meta.PathFor("language", configDir)
	if err != nil {
		printErrorf(stderr, "%v: %s", err, metaPath)
		return 1
	}
	language, _, err := config.LoadOrCreateLanguage(languagePath)
	if err != nil {
		printErrorf(stderr, "%v: %s", err, languagePath)
		return 1
	}
	setLanguage(language.Resolve(os.Getenv))
	systemPrompt, _, err := config.LoadOrCreateSystemPrompt(systemPromptPath, i18n.PromptLanguage())
	if err != nil {
		printErrorf(stderr, "%v: %s", err, systemPromptPath)
		return 1
	}
	webSearchPath, err := meta.PathFor("webSearch", configDir)
	if err != nil {
		printErrorf(stderr, "%v: %s", err, metaPath)
		return 1
	}
	webSearchConfig, _, err := config.LoadOrCreateWebSearch(webSearchPath)
	if err != nil {
		printErrorf(stderr, "%v: %s", err, webSearchPath)
		return 1
	}
	if created {
		printCreatedConfig(stderr, modelsPath, languagePath, systemPromptPath, metaPath)
		return 1
	}
	if err := config.ValidateModels(models); err != nil {
		printErrorf(stderr, "%v: %s", err, modelsPath)
		return 1
	}
	var webTools []chat.Tool
	var executeWebTool func(context.Context, chat.ToolCall) string
	if webSearchConfig.IsConfigured() {
		provider, err := webSearchConfig.ResolveProvider()
		if err != nil {
			printErrorf(stderr, "%v: %s", err, webSearchPath)
			return 1
		}
		webTools, err = webToolDefinitionsFromMeta(meta, i18n.PromptLanguage())
		if err != nil {
			printErrorf(stderr, "%v: %s", err, metaPath)
			return 1
		}
		executor := webToolExecutor{
			client: websearch.ExaClient{
				APIKey:   provider.Key,
				Language: i18n.PromptLanguage(),
			},
			language: i18n.PromptLanguage(),
		}
		executeWebTool = executor.Execute
		systemPrompt = addWebToolInstructions(systemPrompt, i18n.PromptLanguage())
	}
	var provider config.Provider
	var model config.Model
	if session.Provider != "" {
		provider, model, err = models.Resolve(session.Provider, session.Model)
	} else {
		provider, model, err = models.ResolveDefault()
		if err == nil {
			session.Provider = provider.ID
			session.Model = model.ID
		}
	}
	if err != nil {
		printErrorf(stderr, "%v: %s", err, modelsPath)
		return 1
	}

	client := chat.Client{
		APIURL: provider.URL,
		APIKey: provider.Key,
	}
	runQuestion := func(question string, images []chat.ImageAttachment) error {
		messages := buildMessagesWithImages(
			config.RenderSystemPrompt(systemPrompt, time.Now()),
			session.Messages,
			question,
			images,
		)
		answer, err := ask(ctx, client, chat.Request{
			Model:           model.ID,
			ReasoningEffort: model.ReasoningEffort,
			Messages:        messages,
		}, stdout, stderr, opts.showThink, provider.ID+"/"+model.ID, webTools, executeWebTool)
		if err != nil {
			return err
		}
		session.Messages = append(session.Messages,
			conversation.Message{Role: "user", Content: question},
			conversation.Message{Role: "assistant", Content: answer},
		)
		saveSession := store.Update
		if setCurrent {
			saveSession = store.Save
		}
		return saveSession(session)
	}

	if opts.bind {
		printBoundTask(stderr, session.ID)
		printBindHint(stderr)
		return runBoundQuestions(ctx, stdin, stderr, opts.question, pictures, runQuestion)
	}

	if err := runQuestion(opts.question, pictures); err != nil {
		return questionErrorCode(err, stderr)
	}
	return 0
}

func languageFallbackAllowed(args []string) bool {
	if len(args) == 0 || languageConfigOperation(args) {
		return true
	}
	for _, arg := range args {
		switch arg {
		case "help", "version", "-h", "--help", "-v", "--version":
			return true
		}
	}
	return false
}

func printBoundTask(w io.Writer, sessionID string) {
	style := newOutputStyle(w)
	fmt.Fprintf(w, "%s %s\n", style.green(i18n.T("已绑定任务:")), style.boldCyan(sessionID))
}

type questionRunner func(string, []chat.ImageAttachment) error

func runBoundQuestions(ctx context.Context, stdin io.Reader, stderr io.Writer, initialQuestion string, initialImages []chat.ImageAttachment, runQuestion questionRunner) int {
	reader := bufio.NewReader(stdin)
	needDivider := false
	pendingImages := initialImages

	if initialQuestion = strings.TrimSpace(initialQuestion); initialQuestion != "" {
		if err := runQuestion(initialQuestion, pendingImages); err != nil {
			return questionErrorCode(err, stderr)
		}
		pendingImages = nil
		needDivider = true
	}

	for {
		if needDivider {
			printBindDivider(stderr)
			needDivider = false
		}
		printBindPrompt(stderr)
		line, err := readQuestionLine(ctx, reader)
		if err != nil {
			switch {
			case errors.Is(err, context.Canceled):
				return 130
			case errors.Is(err, io.EOF):
				printBindExit(stderr)
				return 0
			default:
				printErrorf(stderr, "%v", err)
				return 1
			}
		}

		question := strings.TrimSpace(line)
		if question == "" {
			continue
		}
		if question == "/exit" {
			printBindExit(stderr)
			return 0
		}
		if err := runQuestion(question, pendingImages); err != nil {
			return questionErrorCode(err, stderr)
		}
		pendingImages = nil
		needDivider = true
	}
}

func readQuestionLine(ctx context.Context, reader *bufio.Reader) (string, error) {
	type result struct {
		line string
		err  error
	}
	resultChannel := make(chan result, 1)
	go func() {
		line, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) && line != "" {
			err = nil
		}
		resultChannel <- result{line: line, err: err}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-resultChannel:
		return result.line, result.err
	}
}

func questionErrorCode(err error, stderr io.Writer) int {
	if errors.Is(err, context.Canceled) {
		return 130
	}
	printErrorf(stderr, "%v", err)
	return 1
}

func printBindHint(w io.Writer) {
	style := newOutputStyle(w)
	fmt.Fprintln(w, style.dim(i18n.T("输入问题后按 Enter；输入 /exit 或按 Ctrl+C 退出并解绑。")))
}

func printBindDivider(w io.Writer) {
	style := newOutputStyle(w)
	fmt.Fprintln(w, style.gray(strings.Repeat("─", 48)))
}

func printBindPrompt(w io.Writer) {
	style := newOutputStyle(w)
	fmt.Fprint(w, style.bold(i18n.T("问题> ")))
}

func printBindExit(w io.Writer) {
	style := newOutputStyle(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.dim(i18n.T("已退出连续对话，终端绑定已解除。")))
}

func printCreatedConfig(w io.Writer, configPath, languagePath, promptPath, metaPath string) {
	style := newOutputStyle(w)
	fmt.Fprintf(w, "%s %s\n", style.green(i18n.T("已生成配置文件:")), style.cyan(configPath))
	fmt.Fprintf(w, "%s %s\n", style.green(i18n.T("已生成语言配置:")), style.cyan(languagePath))
	fmt.Fprintf(w, "%s %s\n", style.green(i18n.T("已生成系统提示词:")), style.cyan(promptPath))
	fmt.Fprintf(w, "%s %s\n", style.green(i18n.T("已生成配置元数据:")), style.cyan(metaPath))
	fmt.Fprintln(w, style.yellow(i18n.T("请填写 provider 的 url、key 和 models 后重试。")))
}

var (
	errHelpRequested    = errors.New("帮助")
	errVersionRequested = errors.New("版本")
)

type cliOptions struct {
	follow     bool
	followHash string
	bind       bool
	bindHash   string
	showThink  bool
	pictures   []string
	question   string
}

func parseArgs(args []string) (cliOptions, error) {
	var opts cliOptions
	var questionParts []string
	parseOptions := true
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		if !parseOptions {
			questionParts = append(questionParts, arg)
			continue
		}

		switch arg {
		case "-h", "--help":
			return cliOptions{}, errHelpRequested
		case "-v", "--version":
			return cliOptions{}, errVersionRequested
		case "-f", "--follow":
			opts.follow = true
			if len(args) > 0 && isHashCandidate(args[0]) {
				opts.followHash = strings.ToLower(args[0])
				args = args[1:]
			}
		case "-b", "--bind":
			opts.bind = true
			if len(args) > 0 && isHashCandidate(args[0]) {
				opts.bindHash = strings.ToLower(args[0])
				args = args[1:]
			}
		case "--show-think":
			opts.showThink = true
		case "-p", "--picture":
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return cliOptions{}, i18n.New("-p/--picture 需要图片路径")
			}
			opts.pictures = append(opts.pictures, args[0])
			args = args[1:]
		case "--":
			parseOptions = false
		default:
			if strings.HasPrefix(arg, "-") {
				return cliOptions{}, i18n.Errorf("未知选项: %s", arg)
			}
			questionParts = append(questionParts, arg)
		}
	}
	opts.question = strings.Join(questionParts, " ")
	if opts.follow && opts.bind {
		return cliOptions{}, i18n.New("-f 和 -b 不能同时使用")
	}
	if opts.question == "" && !opts.bind {
		return opts, i18n.New("缺少问题")
	}
	return opts, nil
}

func isHashCandidate(value string) bool {
	if len(value) < 6 || len(value) > 32 {
		return false
	}
	for _, char := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return false
		}
	}
	return true
}

func buildMessagesWithImages(systemPrompt string, history []conversation.Message, question string, images []chat.ImageAttachment) []chat.Message {
	messages := make([]chat.Message, 0, len(history)+2)
	if systemPrompt != "" {
		messages = append(messages, chat.Message{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	for _, message := range history {
		messages = append(messages, chat.Message{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	messages = append(messages, chat.NewUserMessage(question, images))
	return messages
}

func ask(ctx context.Context, client chat.Client, request chat.Request, stdout, stderr io.Writer, showThink bool, modelRef string, tools []chat.Tool, executeTool func(context.Context, chat.ToolCall) string) (string, error) {
	renderer := render.NewMarkdown(stdout, newOutputStyle(stdout).enabled)
	defer renderer.Close()

	contentSanitizer := terminaltext.NewStream()
	reasoningSanitizer := terminaltext.NewStream()
	lastByte := byte(0)
	contentStarted := false
	stderrStyle := newOutputStyle(stderr)
	var answer strings.Builder
	messages := append([]chat.Message(nil), request.Messages...)
	var totalUsage chat.Usage
	hasUsage := false
	usageComplete := true
	var generationDuration time.Duration

	const maxToolRounds = 8
	for round := 0; round < maxToolRounds; round++ {
		thinking := newThinkingIndicator(stderr)
		thinking.show()
		roundStartedAt := time.Now()
		var firstDeltaAt time.Time
		var roundContent strings.Builder
		roundReasoningStarted := false
		roundReasoningLastByte := byte(0)

		roundRequest := request
		roundRequest.Messages = messages
		roundRequest.Tools = tools
		if len(tools) > 0 {
			roundRequest.ToolChoice = "auto"
		}
		result, err := client.Stream(ctx, roundRequest, func(delta chat.Delta) error {
			if firstDeltaAt.IsZero() {
				firstDeltaAt = time.Now()
			}
			if delta.Reasoning != "" && showThink && !contentStarted {
				safeReasoning := reasoningSanitizer.WriteString(delta.Reasoning)
				if safeReasoning != "" {
					thinking.clearBeforeOutput()
					roundReasoningStarted = true
					if _, err := io.WriteString(stderr, stderrStyle.gray(safeReasoning)); err != nil {
						return i18n.Errorf("写入思考过程失败: %w", err)
					}
					roundReasoningLastByte = safeReasoning[len(safeReasoning)-1]
				}
			}
			if delta.Content == "" {
				return nil
			}
			safeContent := contentSanitizer.WriteString(delta.Content)
			if safeContent == "" {
				return nil
			}
			if contentStarted && lastByte != '\n' && !strings.HasPrefix(safeContent, "\n") {
				if _, err := renderer.Write([]byte("\n")); err != nil {
					return err
				}
				answer.WriteByte('\n')
				lastByte = '\n'
			}

			if roundReasoningStarted && !contentStarted {
				if roundReasoningLastByte != '\n' {
					if _, err := io.WriteString(stderr, "\n"); err != nil {
						return i18n.Errorf("分隔思考过程与正文失败: %w", err)
					}
				}
				divider := stderrStyle.gray(thinkingDividerText(time.Since(roundStartedAt)))
				if _, err := fmt.Fprintf(stderr, "%s\n\n", divider); err != nil {
					return i18n.Errorf("写入思考时长失败: %w", err)
				}
			} else if !contentStarted {
				thinking.clearBeforeOutput()
			}
			contentStarted = true
			if _, err := renderer.Write([]byte(safeContent)); err != nil {
				return err
			}
			answer.WriteString(safeContent)
			roundContent.WriteString(safeContent)
			lastByte = safeContent[len(safeContent)-1]
			return nil
		})
		roundEndedAt := time.Now()
		thinking.clear()
		if err != nil {
			return "", err
		}

		if firstDeltaAt.IsZero() {
			firstDeltaAt = roundStartedAt
		}
		generationDuration += roundEndedAt.Sub(firstDeltaAt)
		if result.HasUsage {
			hasUsage = true
			totalUsage.PromptTokens += result.Usage.PromptTokens
			totalUsage.CompletionTokens += result.Usage.CompletionTokens
			totalUsage.TotalTokens += result.Usage.TotalTokens
		} else {
			usageComplete = false
		}
		if len(result.ToolCalls) == 0 {
			break
		}
		if executeTool == nil {
			return "", i18n.New("模型请求工具，但当前没有可执行的工具")
		}
		if round == maxToolRounds-1 {
			return "", i18n.New("工具调用轮数超过限制")
		}

		assistantMessage := chat.Message{
			Role:      "assistant",
			ToolCalls: result.ToolCalls,
		}
		if roundContent.Len() > 0 {
			assistantMessage.Content = roundContent.String()
		}
		messages = append(messages, assistantMessage)
		for _, call := range result.ToolCalls {
			fmt.Fprintln(stderr, stderrStyle.gray("↳ "+toolStatusText(call)))
			toolResult := executeTool(ctx, call)
			if toolResult == "" {
				toolResult = "{}"
			}
			printToolResultPreview(stderr, call, toolResult)
			messages = append(messages, chat.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    toolResult,
			})
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}

	contentSanitizer.Close()
	reasoningSanitizer.Close()
	if err := renderer.Close(); err != nil {
		return "", i18n.Errorf("完成 Markdown 渲染失败: %w", err)
	}
	if answer.Len() == 0 {
		return "", i18n.New("响应结束但未收到可显示正文")
	}
	if lastByte != '\n' {
		if _, err := io.WriteString(stdout, "\n"); err != nil {
			return "", i18n.Errorf("写入换行失败: %w", err)
		}
	}
	if hasUsage && usageComplete {
		if _, err := fmt.Fprintln(stderr); err != nil {
			return "", i18n.Errorf("写入统计信息分隔行失败: %w", err)
		}
		if err := printResponseStats(stderr, modelRef, totalUsage, generationDuration); err != nil {
			return "", i18n.Errorf("写入统计信息失败: %w", err)
		}
	}
	return answer.String(), nil
}

func printResponseStats(writer io.Writer, modelRef string, usage chat.Usage, elapsed time.Duration) error {
	_, err := fmt.Fprintln(writer, newOutputStyle(writer).gray(responseStatsText(modelRef, usage, elapsed)))
	return err
}

func responseStatsText(modelRef string, usage chat.Usage, elapsed time.Duration) string {
	var tokensPerSecond int64
	if seconds := elapsed.Seconds(); seconds > 0 {
		tokensPerSecond = int64(math.Round(float64(usage.CompletionTokens) / seconds))
	}
	return fmt.Sprintf(
		"%s | ↑ %d tok | ↓ %d tok | %d tok/s",
		modelRef,
		usage.PromptTokens,
		usage.CompletionTokens,
		tokensPerSecond,
	)
}

type thinkingIndicator struct {
	writer    io.Writer
	style     outputStyle
	startedAt time.Time
	frame     int
	visible   bool
	stop      chan struct{}
	stopped   chan struct{}
}

func newThinkingIndicator(writer io.Writer) *thinkingIndicator {
	return &thinkingIndicator{
		writer: writer,
		style:  newOutputStyle(writer),
	}
}

func (t *thinkingIndicator) show() {
	if !newOutputStyle(t.writer).enabled {
		return
	}
	t.startedAt = time.Now()
	t.visible = true
	t.stop = make(chan struct{})
	t.stopped = make(chan struct{})
	t.render()
	go t.run()
}

func (t *thinkingIndicator) run() {
	defer close(t.stopped)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.render()
		case <-t.stop:
			return
		}
	}
}

func (t *thinkingIndicator) render() {
	elapsed := int(time.Since(t.startedAt) / time.Second)
	frame := thinkingFrames[t.frame%len(thinkingFrames)]
	t.frame++
	fmt.Fprint(t.writer, "\r\033[2K"+t.style.cyan(frame)+" "+t.style.gray(thinkingElapsedText(elapsed)))
}

func thinkingElapsedText(seconds int) string {
	return i18n.T("已思考 %d 秒...", seconds)
}

func thinkingDurationText(duration time.Duration) string {
	seconds := int(duration / time.Second)
	if duration%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	return i18n.T("已思考 %d 秒", seconds)
}

func thinkingDividerText(duration time.Duration) string {
	return "──────── " + thinkingDurationText(duration) + " ────────"
}

func thinkingStatusText(frame string, seconds int) string {
	return frame + " " + thinkingElapsedText(seconds)
}

func (t *thinkingIndicator) clear() {
	if !t.visible {
		return
	}
	close(t.stop)
	<-t.stopped
	fmt.Fprint(t.writer, "\r\033[2K")
	t.visible = false
}

func (t *thinkingIndicator) clearBeforeOutput() {
	if !t.visible {
		return
	}
	t.clear()
	fmt.Fprint(t.writer, "\n")
}

func isTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func isInteractiveInput(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return true
	}
	return isTerminal(file)
}
