package extserver

// extserver 의 구조화 로거.
//
// ⚠ **stdout 을 쓰지 않는다.** stdout 첫 줄은 기동 핸드셰이크(ABLEOPS_EXT_READY …) 전용이며,
// Core 가 그 줄을 파싱해 확장 주소를 얻는다. 로그가 stdout 에 섞이면 기동이 실패하거나
// 엉뚱한 주소로 프록시하게 된다. 그래서 로그는 항상 stderr 로 나간다(Core 가 확장 ID 를 붙여 수집).
//
// ⚠ 토큰(CALL_TOKEN·HOST_TOKEN)은 어떤 인자로도 넘기지 않는다.

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	extv1 "github.com/heartblast/kafka-control-portal/sdk/extension/v1"
)

// stderrLogger 는 extensionv1.Logger 의 기본 구현이다(한 줄 = 한 이벤트).
//
// 형식: 2026-08-13T09:00:00Z INFO [sample-process] 메시지 key=value key=value
// 여러 고루틴이 동시에 기록하므로 뮤텍스로 줄이 섞이지 않게 한다.
type stderrLogger struct {
	mu          sync.Mutex
	w           io.Writer
	extensionID string
}

// 컴파일 타임 계약 확인.
var _ extv1.Logger = (*stderrLogger)(nil)

// newLogger 는 로거를 만든다(w 가 nil 이면 stderr).
func newLogger(w io.Writer, extensionID string) *stderrLogger {
	if w == nil {
		w = os.Stderr
	}
	return &stderrLogger{w: w, extensionID: extensionID}
}

func (l *stderrLogger) Info(msg string, args ...any)  { l.write("INFO", msg, args) }
func (l *stderrLogger) Warn(msg string, args ...any)  { l.write("WARN", msg, args) }
func (l *stderrLogger) Error(msg string, args ...any) { l.write("ERROR", msg, args) }

// write 는 한 줄을 기록한다. args 는 slog 관례대로 key, value 쌍이며 홀수면 마지막 값은 "!BADKEY" 로 남는다.
func (l *stderrLogger) write(level, msg string, args []any) {
	var b strings.Builder
	b.WriteString(time.Now().UTC().Format(time.RFC3339))
	b.WriteByte(' ')
	b.WriteString(level)
	if l.extensionID != "" {
		b.WriteString(" [")
		b.WriteString(l.extensionID)
		b.WriteByte(']')
	}
	b.WriteByte(' ')
	b.WriteString(msg)
	for i := 0; i < len(args); i += 2 {
		key := fmt.Sprint(args[i])
		value := "!BADKEY"
		if i+1 < len(args) {
			value = fmt.Sprint(args[i+1])
		}
		b.WriteByte(' ')
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(strings.ReplaceAll(value, "\n", " "))
	}
	b.WriteByte('\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = io.WriteString(l.w, b.String())
}
