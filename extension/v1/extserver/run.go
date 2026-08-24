package extserver

// 외부 프로세스 Extension 의 진입점과 수명주기.
//
// 순서(프로토콜 §1·§2·§3·§5):
//	1. Manifest 검증 → 2. 환경변수 로딩 → 3. HostContext 조립 → 4. ext.Start →
//	5. 라우트 마운트(+ /health) → 6. 127.0.0.1:0 리스닝 → 7. stdout 핸드셰이크 한 줄 →
//	8. 서빙(호출 토큰 검증 · Identity 복원) → 9. 종료 신호 → graceful shutdown → ext.Stop

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	extv1 "github.com/heartblast/kafka-control-portal/sdk/extension/v1"
)

const (
	// ReadyPrefix 는 기동 핸드셰이크 줄의 접두다(Core 가 이 줄을 기다린다).
	//
	// 값은 SDK 프로토콜 정의(extensionv1/protocol.go)를 그대로 재노출한다 — 접두 문자열이
	// 두 곳에 따로 있으면 한쪽만 고쳤을 때 Core 가 영영 기동 신호를 못 본다.
	ReadyPrefix = extv1.ReadyPrefix
	// IncompatiblePrefix 는 기동 전 호환성 실패를 알리는 줄의 접두다.
	IncompatiblePrefix = extv1.IncompatiblePrefix
	// healthPath 는 extserver 가 직접 제공하는 헬스 경로다(Core 가 주기적으로 폴링한다).
	healthPath = "/health"
	// listenAddr 는 리스닝 주소다. 포트 0 = 커널이 빈 포트를 고르고 우리가 실제 주소를 알린다.
	//
	// Core 가 포트를 미리 골라 넘기지 않는 이유: 확보와 사용 사이에 다른 프로세스가 그 포트를
	// 채가는 race 가 있다. 또 주소는 반드시 루프백이어야 한다 — 외부에 확장 API 가 직접 노출되면
	// Core 의 인증·인가·감사를 통째로 우회할 수 있다(요구A §31).
	listenAddr = "127.0.0.1:0"

	defaultShutdownTimeout = 15 * time.Second
	// readHeaderTimeout 은 Slowloris 방어값이다(루프백이라도 기본 방어는 둔다).
	readHeaderTimeout = 15 * time.Second
	// stopTimeout 은 ext.Stop 에 허용하는 시간이다.
	stopTimeout = 10 * time.Second
)

// reservedSubPaths 는 Core 관리 API 가 선점한 서브경로다(Manifest 등록 단계에서도 거부된다).
//
// Core 는 `/api/extensions/{id}/health` 같은 경로를 자기 관리 API 로 처리하므로, 확장이 같은
// 이름을 쓰면 그 라우트는 **영원히 도달할 수 없다**. 여기서 미리 거부해 기동 시점에 알려준다.
// ⚠ Core 의 internal/extensionhost/manifest_load.go(reservedSubPaths)와 **같은 목록**이어야 한다.
// 한쪽만 늘리면 Manifest 검증은 통과하는데 실제로는 도달할 수 없는 라우트가 생긴다.
var reservedSubPaths = map[string]bool{
	"health":         true,
	"install-events": true,
	"enable":         true,
	"disable":        true,
	"update":         true,
	"rollback":       true,
	"assets":         true,
}

// ReadyMessage 는 기동 핸드셰이크 줄의 JSON 본문이다(SDK 프로토콜 정의의 별칭).
//
// 별칭으로 두는 이유: Core 와 SDK 가 **같은 구조체**를 써야 필드를 추가할 때 한쪽만 바뀌는 일이
// 생기지 않는다. 기존 코드의 extserver.ReadyMessage{Addr:…, Version:…} 는 그대로 컴파일된다.
type ReadyMessage = extv1.ReadyMessage

// options 는 Run 의 동작 옵션이다(기본값은 운영 환경 기준).
type options struct {
	stdout          io.Writer
	logWriter       io.Writer
	shutdownTimeout time.Duration
	hostTimeout     time.Duration
	lookupEnv       func(string) (string, bool)
	listen          func() (net.Listener, error)
	handleSignals   bool
	watchStdin      bool
}

// Option 은 Run 의 동작을 바꾸는 설정이다.
type Option func(*options)

// defaultOptions 는 운영 기본값이다.
func defaultOptions() options {
	return options{
		stdout:          os.Stdout,
		logWriter:       os.Stderr,
		shutdownTimeout: defaultShutdownTimeout,
		hostTimeout:     defaultHostTimeout,
		lookupEnv:       os.LookupEnv,
		listen:          func() (net.Listener, error) { return net.Listen("tcp", listenAddr) },
		handleSignals:   true,
		watchStdin:      true,
	}
}

// WithStdout 은 기동 핸드셰이크를 쓸 대상을 바꾼다(기본 os.Stdout).
func WithStdout(w io.Writer) Option {
	return func(o *options) {
		if w != nil {
			o.stdout = w
		}
	}
}

// WithLogWriter 는 extserver 자체 로그의 출력 대상을 바꾼다(기본 os.Stderr).
// ⚠ stdout 을 지정하지 않는다 — 핸드셰이크 줄이 오염된다.
func WithLogWriter(w io.Writer) Option {
	return func(o *options) {
		if w != nil {
			o.logWriter = w
		}
	}
}

// WithShutdownTimeout 은 graceful shutdown 대기 시간을 바꾼다.
func WithShutdownTimeout(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.shutdownTimeout = d
		}
	}
}

// WithHostTimeout 은 Core Host API 호출 1건의 타임아웃을 바꾼다.
func WithHostTimeout(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.hostTimeout = d
		}
	}
}

// --- 테스트 전용 seam(패키지 내부에서만 사용) ---

func withLookupEnv(lookup func(string) (string, bool)) Option {
	return func(o *options) {
		if lookup != nil {
			o.lookupEnv = lookup
		}
	}
}

func withSignalHandling(enabled bool) Option {
	return func(o *options) { o.handleSignals = enabled }
}

func withStdinWatch(enabled bool) Option {
	return func(o *options) { o.watchStdin = enabled }
}

// Run 은 외부 프로세스 Extension 을 기동하고 종료 신호까지 서빙한다.
//
//	func main() {
//	    if err := extserver.Run(myext.New()); err != nil {
//	        fmt.Fprintln(os.Stderr, err)
//	        os.Exit(1)
//	    }
//	}
func Run(ext extv1.Extension, opts ...Option) error {
	return RunContext(context.Background(), ext, opts...)
}

// RunContext 는 상위 컨텍스트를 받는 Run 이다(ctx 취소 시 graceful shutdown).
func RunContext(ctx context.Context, ext extv1.Extension, opts ...Option) error {
	if ext == nil {
		return errors.New("Extension 구현이 nil 입니다(extserver.Run 인자를 확인하세요)")
	}
	o := defaultOptions()
	for _, apply := range opts {
		if apply != nil {
			apply(&o)
		}
	}

	// 1) Manifest — Core 가 이미 검증하지만, 잘못된 자산으로 뜬 프로세스를 여기서도 막는다.
	manifest := ext.Manifest()
	if err := manifest.Validate(); err != nil {
		return fmt.Errorf("Manifest 검증에 실패했습니다(패키지 자산을 확인하세요): %w", err)
	}

	// 2) 환경변수·프로토콜·capability 협상(프로토콜 §1)
	env, err := loadEnvironment(o.lookupEnv, manifest)
	if err != nil {
		// 프로토콜 불일치는 **재시작해도 결과가 같다**. Core 가 크래시로 오인해 백오프를 두고
		// 반복 재시작하지 않도록, stdout 한 줄로 사유를 알린 뒤 종료한다(protocol.go 주석).
		var mismatch *protocolMismatchError
		if errors.As(err, &mismatch) {
			writeIncompatible(o.stdout, mismatch.reason)
		}
		return err
	}
	logger := newLogger(o.logWriter, env.ExtensionID)
	for _, name := range env.UnknownCapabilities {
		logger.Warn("이 SDK 가 모르는 capability 를 Core 가 전달했습니다(무시합니다)", "capability", name)
	}
	for _, c := range env.MissingCapabilities {
		logger.Warn("Manifest 가 선언한 capability 를 Core 가 허용하지 않았습니다(사용 시 CAPABILITY_UNAVAILABLE)",
			"capability", string(c))
	}
	for _, c := range env.UnsupportedCapabilities {
		logger.Warn("이 SDK 는 외부 프로세스 확장에서 해당 capability 의 클라이언트를 제공하지 않습니다(사용 시 CAPABILITY_UNAVAILABLE)",
			"capability", string(c))
	}

	// 3) 종료 컨텍스트 — 신호·부모 프로세스 종료·상위 ctx 취소를 한 지점으로 모은다.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if o.handleSignals {
		// Windows 에서도 컴파일·동작한다: os.Interrupt 는 Ctrl+C, SIGTERM 은 Unix 에서만 발생하지만
		// 상수 자체는 Windows 에도 정의되어 있다. Windows 의 실제 종료 경로는 아래 stdin 감시다.
		sigCtx, stopSignals := signal.NotifyContext(runCtx, os.Interrupt, syscall.SIGTERM)
		defer stopSignals()
		runCtx = sigCtx
	}
	if o.watchStdin {
		watchParentPipe(cancel, logger)
	}

	// 4) HostContext 조립 — 선언된 capability 만 주입된다.
	host, client := newHostContext(env, logger, o.hostTimeout)

	// 5) 확장 시작
	if err := ext.Start(runCtx, host); err != nil {
		return fmt.Errorf("확장 기능 시작에 실패했습니다: %w", err)
	}
	stopExtension := stopOnce(ext, logger)
	defer stopExtension()

	// 6) 라우트 마운트(+ /health)
	handler, err := buildHandler(ext, env, logger)
	if err != nil {
		return err
	}

	// 7) 리스닝 — 반드시 루프백이어야 한다.
	ln, err := o.listen()
	if err != nil {
		return fmt.Errorf("확장 기능 HTTP 리스닝에 실패했습니다(%s): %w", listenAddr, err)
	}
	if err := checkLoopback(ln.Addr()); err != nil {
		_ = ln.Close()
		return err
	}

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		// http 서버의 내부 오류 로그도 stdout 을 오염시키지 않게 stderr 로 보낸다.
		ErrorLog: log.New(o.logWriter, "", 0),
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	// 8) 기동 핸드셰이크 — **리스닝을 시작한 뒤에만** 출력한다.
	//    Core 는 이 줄을 보는 즉시 프록시를 시작하므로, 먼저 출력하면 첫 요청이 연결 거부된다.
	if err := writeHandshake(o.stdout, ln.Addr().String(), env); err != nil {
		shutdown(srv, o.shutdownTimeout)
		return err
	}
	logger.Info("외부 프로세스 확장 기능을 시작했습니다",
		"addr", ln.Addr().String(), "version", env.Version,
		"protocol", env.ProtocolVersion, "apiVersion", extv1.APIVersion,
		"capabilities", capabilityNames(env.Capabilities))

	// 자기 진단: Host API 연결 확인. 실패해도 기동을 막지 않는다 —
	// Core 가 Host API 를 늦게 열 수 있고, 확장의 모든 기능이 Host API 를 쓰는 것도 아니다.
	go func() {
		pingCtx, pingCancel := context.WithTimeout(runCtx, 5*time.Second)
		defer pingCancel()
		if err := client.Ping(pingCtx); err != nil {
			logger.Warn("Core Host API 연결 확인에 실패했습니다", "error", err.Error())
		}
	}()

	// 9) 종료 대기
	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("확장 기능 HTTP 서버가 중단되었습니다: %w", err)
		}
	case <-runCtx.Done():
		logger.Info("종료 신호를 받아 확장 기능을 정지합니다")
		shutdown(srv, o.shutdownTimeout)
		<-serveErr
	}
	stopExtension()
	return nil
}

// stopOnce 는 ext.Stop 을 한 번만 호출하는 함수를 만든다(defer 와 정상 경로 양쪽에서 불린다).
func stopOnce(ext extv1.Extension, logger extv1.Logger) func() {
	done := false
	return func() {
		if done {
			return
		}
		done = true
		ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
		defer cancel()
		if err := ext.Stop(ctx); err != nil {
			logger.Error("확장 기능 정지 중 오류가 발생했습니다", "error", err.Error())
		}
	}
}

// shutdown 은 HTTP 서버를 graceful 하게 닫는다(시간 초과 시 강제 종료).
func shutdown(srv *http.Server, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		_ = srv.Close()
	}
}

// watchParentPipe 는 부모(Core) 프로세스 종료를 stdin EOF 로 감지해 종료를 시작한다.
//
// Windows 에는 SIGTERM 이 없어 신호로 graceful shutdown 을 요청할 방법이 마땅치 않다.
// Core 가 stdin 파이프를 닫으면(또는 프로세스가 사라지면) 여기서 EOF 를 받아 정상 종료 경로를 탄다.
// stdin 이 파이프가 **아닌** 경우(콘솔·/dev/null·NUL)에는 감시하지 않는다 — devnull 은 즉시 EOF 라
// 감시했다간 기동하자마자 스스로 종료해 버린다.
func watchParentPipe(cancel context.CancelFunc, logger extv1.Logger) {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeNamedPipe == 0 {
		return
	}
	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := os.Stdin.Read(buf); err != nil {
				logger.Info("Core 와의 표준입력 연결이 끊겨 확장 기능을 정지합니다")
				cancel()
				return
			}
		}
	}()
}

// checkLoopback 은 리스닝 주소가 루프백인지 확인한다(외부 노출 차단).
func checkLoopback(addr net.Addr) error {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("확장 기능 리스닝 주소가 TCP 가 아닙니다: %v", addr)
	}
	if !tcp.IP.IsLoopback() {
		return fmt.Errorf("확장 기능은 루프백에만 리스닝해야 합니다(현재 %s)", tcp.String())
	}
	return nil
}

// writeHandshake 는 기동 핸드셰이크 한 줄을 출력한다.
//
// 프로토콜 1의 원래 필드(addr·version)는 그대로 채우고 apiVersion·protocolVersion 을 **추가**한다.
// 구버전 Core 는 모르는 필드를 무시하므로 하위호환이 유지된다.
func writeHandshake(w io.Writer, addr string, env Environment) error {
	body, err := json.Marshal(ReadyMessage{
		Addr:            addr,
		Version:         env.Version,
		APIVersion:      extv1.APIVersion,
		ProtocolVersion: extv1.ProtocolVersion,
	})
	if err != nil {
		return fmt.Errorf("기동 핸드셰이크 메시지를 만들 수 없습니다: %w", err)
	}
	if _, err := fmt.Fprintf(w, "%s %s\n", ReadyPrefix, body); err != nil {
		return fmt.Errorf("기동 핸드셰이크를 출력하지 못했습니다: %w", err)
	}
	if f, ok := w.(*os.File); ok {
		_ = f.Sync() // 파이프 대상에서는 실패할 수 있으나 Go 의 stdout 은 버퍼링하지 않으므로 무해하다.
	}
	return nil
}

// writeIncompatible 은 호환성 실패 한 줄을 출력한다(Core 가 INCOMPATIBLE 로 격리한다).
//
// 출력 실패는 무시한다 — 이 시점에는 이미 종료 경로이고, 알릴 방법이 없다고 해서 더 할 수 있는
// 일도 없다(Core 는 프로세스 조기 종료로 감지한다).
func writeIncompatible(w io.Writer, reason string) {
	body, err := json.Marshal(extv1.IncompatibleMessage{
		Reason:          reason,
		ProtocolVersion: extv1.ProtocolVersion,
		APIVersion:      extv1.APIVersion,
	})
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "%s %s\n", IncompatiblePrefix, body)
	if f, ok := w.(*os.File); ok {
		_ = f.Sync()
	}
}

// reservedSubPathList 는 오류 문구용 예약 서브경로 목록이다(정렬 — 문구가 실행마다 흔들리지 않게).
func reservedSubPathList() string {
	names := make([]string, 0, len(reservedSubPaths))
	for k := range reservedSubPaths {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// capabilityNames 는 로그용 capability 이름 목록을 만든다.
func capabilityNames(caps []extv1.Capability) string {
	if len(caps) == 0 {
		return "(없음)"
	}
	names := make([]string, 0, len(caps))
	for _, c := range caps {
		names = append(names, string(c))
	}
	return strings.Join(names, ",")
}

// ── HTTP 조립 ────────────────────────────────────────────────────────────────

// buildHandler 는 확장 라우트와 /health 를 마운트하고 공통 미들웨어를 두른다.
func buildHandler(ext extv1.Extension, env Environment, logger extv1.Logger) (http.Handler, error) {
	mux := http.NewServeMux()
	seen := make(map[string]bool)

	register := func(method, pattern string, h http.Handler) (err error) {
		key := method + " " + pattern
		if seen[key] {
			return fmt.Errorf("확장 라우트가 중복 선언되었습니다: %s", key)
		}
		seen[key] = true
		// ServeMux 는 잘못된/충돌하는 패턴에 panic 한다. 패닉을 그대로 두면 원인이 스택트레이스로만
		// 남으므로, 한국어 오류로 바꿔 기동 실패 사유가 Core 로그에 그대로 보이게 한다.
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("확장 라우트를 마운트할 수 없습니다(%s): %v", key, r)
			}
		}()
		mux.Handle(key, h)
		return nil
	}

	if err := register(http.MethodGet, healthPath, healthHandler(ext, logger)); err != nil {
		return nil, err
	}

	for _, rt := range ext.Routes() {
		pattern, err := normalizePattern(rt.Pattern)
		if err != nil {
			return nil, err
		}
		method := strings.ToUpper(strings.TrimSpace(rt.Method))
		if method == "" {
			method = http.MethodGet
		}
		if rt.Handler == nil {
			return nil, fmt.Errorf("확장 라우트 %s %s 의 핸들러가 nil 입니다", method, pattern)
		}
		if err := register(method, pattern, rt.Handler); err != nil {
			return nil, err
		}
	}

	return recoverMiddleware(logger, callTokenMiddleware(env.CallToken, identityMiddleware(mux))), nil
}

// normalizePattern 은 확장 라우트 패턴을 검사·정규화한다(Manifest routes[].path 기준 상대 경로).
func normalizePattern(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		p = "/"
	}
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("확장 라우트는 마운트 접두 기준 상대 경로여야 합니다(/ 로 시작): %q", raw)
	}
	if strings.HasPrefix(p, "/api/") {
		return "", fmt.Errorf("확장 라우트에 절대 API 경로를 쓸 수 없습니다: %q (Core 가 /api/extensions/<id> 아래에 마운트한다)", raw)
	}
	if head := firstSegment(p); reservedSubPaths[head] {
		return "", fmt.Errorf("확장 라우트 %q 는 Core 관리 API 가 선점한 예약 서브경로입니다(예약: %s)", raw, reservedSubPathList())
	}
	return p, nil
}

// firstSegment 는 "/a/b" 에서 "a" 를 뽑는다.
func firstSegment(p string) string {
	p = strings.TrimPrefix(p, "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}

// healthHandler 는 GET /health 를 처리한다(프로토콜 §5).
//
// 확장이 비정상이어도 **HTTP 200 으로 상태를 보고**한다. 상태 판정(DEGRADED/FAILED)은 Core 의
// 몫이며, 비정상을 5xx 로 돌려주면 "확장이 죽었다"와 "확장이 스스로 비정상이라고 보고했다"를
// Core 가 구분할 수 없다.
func healthHandler(ext extv1.Extension, logger extv1.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		health := safeHealth(r.Context(), ext, logger)
		writeJSON(w, http.StatusOK, health)
	})
}

// safeHealth 는 확장의 Health 패닉을 흡수한다(헬스 확인이 프로세스를 죽이면 안 된다).
func safeHealth(ctx context.Context, ext extv1.Extension, logger extv1.Logger) (h extv1.Health) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("확장 기능 Health 확인 중 패닉이 발생했습니다", "panic", fmt.Sprint(r))
			h = extv1.Health{OK: false, Message: "확장 기능 상태 확인 중 오류가 발생했습니다", CheckedAt: time.Now()}
		}
	}()
	h = ext.Health(ctx)
	if h.CheckedAt.IsZero() {
		h.CheckedAt = time.Now()
	}
	return h
}

// callTokenMiddleware 는 모든 요청에서 호출 토큰을 **상수시간 비교**로 검증한다.
//
// 상수시간 비교를 쓰는 이유: == 비교는 첫 불일치 바이트에서 끝나므로 응답 시간으로 토큰을
// 한 바이트씩 알아낼 수 있다. 루프백이라도 같은 호스트의 다른 프로세스가 시도할 수 있다.
//
// ⚠ /health 도 예외가 아니다. Core 의 헬스 폴러도 이 헤더를 보내야 한다.
func callTokenMiddleware(want string, next http.Handler) http.Handler {
	expected := []byte(want)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get(HeaderCallToken))
		if subtle.ConstantTimeCompare(got, expected) != 1 {
			// 토큰 값은 어떤 경우에도 응답·로그에 싣지 않는다.
			writeError(w, http.StatusUnauthorized, "확장 기능 호출 토큰이 올바르지 않습니다")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// identityMiddleware 는 X-Ableops-* 헤더에서 사용자 신원을 복원해 요청 컨텍스트에 넣는다.
func identityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if id := identityFromRequest(r); id.UserID != "" {
			ctx = WithIdentity(ctx, id)
		}
		// ⚠ 요청 추적 ID 는 신원과 **독립적으로** 저장한다. Identity 안에만 두면 익명 요청
		// (사용자 컨텍스트 없는 호출)에서 상관관계가 끊겨 Browser→Core→Extension→Host API 를
		// 하나의 요청으로 이어 볼 수 없다(요구 §16).
		ctx = WithRequestID(ctx, strings.TrimSpace(r.Header.Get(HeaderRequestID)))
		ctx = withRequestToken(ctx, strings.TrimSpace(r.Header.Get(HeaderRequestToken)))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// recoverMiddleware 는 핸들러 패닉을 500 으로 바꾼다.
// 확장 하나의 버그로 프로세스가 죽으면 Core 가 그 확장을 FAILED 로 만들고 재기동해야 하므로,
// 요청 하나의 실패로 가두는 편이 훨씬 낫다.
func recoverMiddleware(logger extv1.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// ErrAbortHandler 는 "조용히 연결을 끊으라"는 net/http 의 약속이므로 그대로 넘긴다.
				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rec)
				}
				logger.Error("확장 기능 핸들러에서 패닉이 발생했습니다", "path", r.URL.Path, "panic", fmt.Sprint(rec))
				writeError(w, http.StatusInternalServerError, "확장 기능 처리 중 오류가 발생했습니다")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// errorBody 는 오류 응답 본문이다(Core·확장 공통 형식).
type errorBody struct {
	Error string `json:"error"`
}

// writeJSON 은 JSON 응답을 기록한다.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError 는 한국어 오류 응답을 기록한다.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}
