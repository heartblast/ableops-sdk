package extserver

// 수명주기·핸드셰이크·호출 토큰·신원 복원 검증(프로토콜 §2·§3·§5).
//
// 이 파일이 고정하는 것은 "서버가 뜨는가"가 아니라 **Core 와 맞물리는 계약**이다:
// 핸드셰이크 한 줄의 형식, 루프백 주소, 토큰 없는 호출의 401, 한글 신원의 왕복,
// 예약 서브경로 거부, 그리고 확장 하나의 패닉이 프로세스를 죽이지 않는다는 것.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	extv1 "github.com/heartblast/kafka-control-portal/sdk/extension/v1"
)

// ── 테스트 대역 ──────────────────────────────────────────────────────────────

// fakeExtension 은 extensionv1.Extension 의 테스트 구현이다.
type fakeExtension struct {
	manifest extv1.Manifest
	routes   []extv1.RouteHandler
	startErr error

	mu          sync.Mutex
	started     bool
	stopCalls   int
	host        extv1.HostContext
	healthPanic bool
}

var _ extv1.Extension = (*fakeExtension)(nil)

func (f *fakeExtension) Manifest() extv1.Manifest { return f.manifest }

func (f *fakeExtension) Start(ctx context.Context, host extv1.HostContext) error {
	if f.startErr != nil {
		return f.startErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = true
	f.host = host
	return nil
}

func (f *fakeExtension) Stop(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls++
	f.started = false
	return nil
}

func (f *fakeExtension) Health(ctx context.Context) extv1.Health {
	f.mu.Lock()
	panicNow := f.healthPanic
	started := f.started
	f.mu.Unlock()
	if panicNow {
		panic("헬스 확인 중 의도적 패닉")
	}
	return extv1.Health{OK: started, Message: "테스트 확장 상태", CheckedAt: time.Now()}
}

func (f *fakeExtension) Routes() []extv1.RouteHandler { return f.routes }

func (f *fakeExtension) hostContext() extv1.HostContext {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.host
}

func (f *fakeExtension) stopCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopCalls
}

// lineCapture 는 stdout 에 쓰인 줄을 채널로 흘려보낸다(핸드셰이크 관찰용).
type lineCapture struct {
	mu    sync.Mutex
	buf   []byte
	lines chan string
}

func newLineCapture() *lineCapture { return &lineCapture{lines: make(chan string, 8)} }

func (c *lineCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	for {
		i := bytes.IndexByte(c.buf, '\n')
		if i < 0 {
			break
		}
		line := string(c.buf[:i])
		c.buf = c.buf[i+1:]
		select {
		case c.lines <- line:
		default:
		}
	}
	return len(p), nil
}

// next 는 다음 줄을 기다린다.
func (c *lineCapture) next(t *testing.T) string {
	t.Helper()
	select {
	case line := <-c.lines:
		return line
	case <-time.After(5 * time.Second):
		t.Fatal("핸드셰이크 줄이 출력되지 않았다")
		return ""
	}
}

// runningServer 는 기동된 테스트 서버 핸들이다.
type runningServer struct {
	addr    string
	ready   ReadyMessage
	cancel  context.CancelFunc
	done    chan error
	callTok string
}

// stop 은 서버를 정지시키고 Run 의 반환값을 돌려준다.
func (s *runningServer) stop(t *testing.T) error {
	t.Helper()
	s.cancel()
	select {
	case err := <-s.done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("graceful shutdown 이 끝나지 않았다")
		return nil
	}
}

// do 는 확장 서버에 요청을 보낸다(기본적으로 유효한 호출 토큰을 붙인다).
func (s *runningServer) do(t *testing.T, path string, mutate func(*http.Request)) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://"+s.addr+path, nil)
	if err != nil {
		t.Fatalf("요청 생성 실패: %v", err)
	}
	req.Header.Set(HeaderCallToken, s.callTok)
	if mutate != nil {
		mutate(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("요청 실패(%s): %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// startServer 는 확장 서버를 기동하고 핸드셰이크를 파싱한다.
func startServer(t *testing.T, ext extv1.Extension, envOverrides map[string]string) *runningServer {
	t.Helper()
	f := newFakeCore(t)
	f.json(http.MethodGet, hostPath("/ping"), http.StatusOK, map[string]any{"ok": true, "extensionId": "test-ext"})

	envMap := fullEnv()
	envMap[EnvHostURL] = f.server.URL + "/api/extensions/_host"
	for k, v := range envOverrides {
		envMap[k] = v
	}

	capture := newLineCapture()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RunContext(ctx, ext,
			withLookupEnv(lookupFrom(envMap)),
			WithStdout(capture),
			WithLogWriter(io.Discard),
			withSignalHandling(false),
			withStdinWatch(false),
			WithShutdownTimeout(3*time.Second),
		)
	}()
	t.Cleanup(cancel)

	line := capture.next(t)
	ready := parseReadyLine(t, line)
	return &runningServer{addr: ready.Addr, ready: ready, cancel: cancel, done: done, callTok: envMap[EnvCallToken]}
}

// parseReadyLine 은 기동 핸드셰이크 줄을 검사·파싱한다.
func parseReadyLine(t *testing.T, line string) ReadyMessage {
	t.Helper()
	prefix := ReadyPrefix + " "
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("핸드셰이크 접두가 다르다: %q", line)
	}
	var msg ReadyMessage
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, prefix)), &msg); err != nil {
		t.Fatalf("핸드셰이크 JSON 해석 실패: %v (줄: %q)", err, line)
	}
	host, port, err := net.SplitHostPort(msg.Addr)
	if err != nil {
		t.Fatalf("핸드셰이크 주소 형식이 다르다: %q", msg.Addr)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		t.Fatalf("리스닝 주소가 루프백이 아니다: %q", msg.Addr)
	}
	if port == "" || port == "0" {
		t.Fatalf("실제 포트가 통보되지 않았다: %q", msg.Addr)
	}
	return msg
}

// decodeBody 는 응답 본문을 해석한다.
func decodeBody[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var out T
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("본문 읽기 실패: %v", err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("본문 JSON 해석 실패: %v (본문: %s)", err, string(data))
	}
	return out
}

// echoIdentityRoute 는 요청 컨텍스트의 신원을 그대로 돌려주는 라우트다.
func echoIdentityRoute() extv1.RouteHandler {
	return extv1.RouteHandler{
		Method:  http.MethodGet,
		Pattern: "/whoami",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := IdentityFrom(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "신원 없음")
				return
			}
			writeJSON(w, http.StatusOK, id)
		}),
	}
}

// ── 기동·핸드셰이크 ──────────────────────────────────────────────────────────

// TestRun_핸드셰이크와헬스 는 프로토콜 §2·§5 를 고정한다.
func TestRun_핸드셰이크와헬스(t *testing.T) {
	ext := &fakeExtension{manifest: testManifest(extv1.CapKafkaRead)}
	srv := startServer(t, ext, nil)

	if srv.ready.Version != "1.2.3" {
		t.Errorf("핸드셰이크 version 이 다르다: %q", srv.ready.Version)
	}

	resp := srv.do(t, healthPath, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/health 상태코드가 다르다: %d", resp.StatusCode)
	}
	health := decodeBody[extv1.Health](t, resp)
	if !health.OK || health.Message == "" || health.CheckedAt.IsZero() {
		t.Errorf("/health 응답이 계약과 다르다: %+v", health)
	}

	if err := srv.stop(t); err != nil {
		t.Fatalf("정상 종료가 실패했다: %v", err)
	}
	if n := ext.stopCount(); n != 1 {
		t.Errorf("Stop 호출 횟수가 다르다: %d", n)
	}
}

// TestRun_호출토큰검증 은 프로토콜 §3 의 호출 토큰 검증을 고정한다.
//
// ⚠ /health 도 예외가 아니다 — Core 의 헬스 폴러도 토큰을 보내야 한다.
func TestRun_호출토큰검증(t *testing.T) {
	ext := &fakeExtension{manifest: testManifest(), routes: []extv1.RouteHandler{echoIdentityRoute()}}
	srv := startServer(t, ext, nil)

	for _, tc := range []struct {
		name  string
		token string
	}{
		{"토큰없음", ""},
		{"토큰불일치", "wrong-token"},
		{"접두만같음", "call-token-값-추가"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{healthPath, "/whoami"} {
				resp := srv.do(t, path, func(r *http.Request) {
					if tc.token == "" {
						r.Header.Del(HeaderCallToken)
						return
					}
					r.Header.Set(HeaderCallToken, tc.token)
				})
				if resp.StatusCode != http.StatusUnauthorized {
					t.Fatalf("%s 가 401 이 아니다: %d", path, resp.StatusCode)
				}
				body := decodeBody[errorBody](t, resp)
				if body.Error == "" {
					t.Errorf("한국어 오류 메시지가 없다: %+v", body)
				}
				// 토큰 값이 응답에 새면 안 된다.
				if strings.Contains(body.Error, "call-token-값") || strings.Contains(body.Error, tc.token) && tc.token != "" {
					t.Errorf("응답에 토큰이 노출되었다: %q", body.Error)
				}
			}
		})
	}
}

// TestRun_신원헤더복원 은 한글 이름·부서가 왕복하는지 **실제 HTTP 로** 확인한다.
func TestRun_신원헤더복원(t *testing.T) {
	ext := &fakeExtension{manifest: testManifest(), routes: []extv1.RouteHandler{echoIdentityRoute()}}
	srv := startServer(t, ext, nil)

	want := extv1.Identity{
		UserID:      "owner01",
		Name:        "김서비스",
		Department:  "정보보호 1팀",
		Roles:       []string{"ServiceOwner"},
		Permissions: []string{"ext.test-ext.view"},
		RequestID:   "req-abc",
	}
	resp := srv.do(t, "/whoami", func(r *http.Request) { SetIdentityHeaders(r.Header, want) })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("상태코드가 다르다: %d", resp.StatusCode)
	}
	got := decodeBody[extv1.Identity](t, resp)
	if got.UserID != want.UserID || got.Name != want.Name || got.Department != want.Department {
		t.Errorf("신원이 왕복하지 않았다: %+v", got)
	}
	if got.RequestID != want.RequestID || len(got.Roles) != 1 || !got.HasPermission("ext.test-ext.view") {
		t.Errorf("역할·권한·요청ID 가 다르다: %+v", got)
	}

	// 신원 헤더가 없으면 "신원 없음"이다(익명·백그라운드 호출과 구분된다).
	anon := srv.do(t, "/whoami", nil)
	if anon.StatusCode != http.StatusUnauthorized {
		t.Errorf("신원 없는 요청이 401 이 아니다: %d", anon.StatusCode)
	}
}

// TestRun_capability미선언시nil 은 프로세스 경계에서도 최소권한이 성립함을 고정한다.
func TestRun_capability미선언시nil(t *testing.T) {
	ext := &fakeExtension{manifest: testManifest(extv1.CapKafkaRead)}
	srv := startServer(t, ext, map[string]string{EnvCapabilities: "kafka.read,cluster.read"})

	host := ext.hostContext()
	if host.Kafka == nil {
		t.Error("kafka.read 가 주입되지 않았다")
	}
	// Manifest 가 선언하지 않은 cluster.read 는 Core 가 넘겼더라도 주입되지 않는다.
	if host.Clusters != nil {
		t.Error("Manifest 미선언 capability 가 주입되었다")
	}
	if host.Workflow != nil || host.Audit != nil || host.Config != nil || host.Secrets != nil {
		t.Error("선언하지 않은 capability 가 주입되었다")
	}
	if host.Identity == nil || host.Log == nil {
		t.Error("Identity/Log 는 항상 주입되어야 한다")
	}
	_ = srv.stop(t)
}

// TestRun_핸들러패닉은500 은 확장 버그가 프로세스를 죽이지 않음을 고정한다.
func TestRun_핸들러패닉은500(t *testing.T) {
	ext := &fakeExtension{
		manifest: testManifest(),
		routes: []extv1.RouteHandler{
			{Method: http.MethodGet, Pattern: "/boom", Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				panic("의도적 패닉")
			})},
			echoIdentityRoute(),
		},
	}
	srv := startServer(t, ext, nil)

	resp := srv.do(t, "/boom", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("패닉이 500 으로 변환되지 않았다: %d", resp.StatusCode)
	}
	if body := decodeBody[errorBody](t, resp); body.Error == "" {
		t.Error("한국어 오류 메시지가 없다")
	}
	// 패닉 이후에도 서버는 살아 있어야 한다.
	if after := srv.do(t, healthPath, nil); after.StatusCode != http.StatusOK {
		t.Errorf("패닉 이후 서버가 죽었다: %d", after.StatusCode)
	}
}

// TestRun_Health패닉도흡수 는 헬스 폴링이 프로세스를 죽이지 않음을 고정한다.
func TestRun_Health패닉도흡수(t *testing.T) {
	ext := &fakeExtension{manifest: testManifest(), healthPanic: true}
	srv := startServer(t, ext, nil)

	resp := srv.do(t, healthPath, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Health 패닉 시에도 200 으로 보고해야 한다: %d", resp.StatusCode)
	}
	health := decodeBody[extv1.Health](t, resp)
	if health.OK {
		t.Error("패닉 상황이 정상으로 보고되었다")
	}
	if health.Message == "" {
		t.Error("한국어 사유가 없다")
	}
}

// ── 기동 거부 경로 ───────────────────────────────────────────────────────────

// runOnce 는 서빙까지 가지 않고 실패하는 경로를 검사한다.
func runOnce(t *testing.T, ext extv1.Extension, envMap map[string]string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return RunContext(ctx, ext,
		withLookupEnv(lookupFrom(envMap)),
		WithStdout(io.Discard),
		WithLogWriter(io.Discard),
		withSignalHandling(false),
		withStdinWatch(false),
	)
}

// TestRun_환경변수누락 은 직접 실행 안내를 고정한다.
func TestRun_환경변수누락(t *testing.T) {
	err := runOnce(t, &fakeExtension{manifest: testManifest()}, map[string]string{})
	if err == nil {
		t.Fatal("환경변수 없이 성공했다")
	}
	if !strings.Contains(err.Error(), DirectRunMessage) {
		t.Errorf("직접 실행 안내가 없다: %v", err)
	}
}

// TestRun_Manifest검증실패 는 손상된 자산으로 뜨지 않음을 고정한다.
func TestRun_Manifest검증실패(t *testing.T) {
	bad := testManifest()
	bad.APIVersion = "ableops.io/extension/v99"
	err := runOnce(t, &fakeExtension{manifest: bad}, fullEnv())
	if err == nil || !strings.Contains(err.Error(), "Manifest") {
		t.Fatalf("Manifest 검증 실패를 알리지 않았다: %v", err)
	}
}

// TestRun_Start실패 는 확장 시작 실패가 한국어로 보고되는지 고정한다.
func TestRun_Start실패(t *testing.T) {
	ext := &fakeExtension{manifest: testManifest(), startErr: errors.New("데이터 디렉터리를 만들 수 없습니다")}
	err := runOnce(t, ext, fullEnv())
	if err == nil {
		t.Fatal("Start 실패인데 성공했다")
	}
	if !strings.Contains(err.Error(), "확장 기능 시작에 실패했습니다") ||
		!strings.Contains(err.Error(), "데이터 디렉터리를 만들 수 없습니다") {
		t.Errorf("원인이 보존되지 않았다: %v", err)
	}
}

// TestRun_예약서브경로거부 는 §8-2 의 예약 경로를 기동 시점에 막는지 고정한다.
//
// Core 관리 API 가 정적 매칭으로 선점하므로, 이런 라우트는 등록되어도 **영원히 도달할 수 없다**.
// 런타임에 "404 도 아닌데 남의 응답이 온다"로 드러나는 것보다 기동 실패가 훨씬 싸다.
func TestRun_예약서브경로거부(t *testing.T) {
	for _, pattern := range []string{"/health", "/enable", "/disable", "/update", "/install-events", "/health/detail"} {
		t.Run(pattern, func(t *testing.T) {
			ext := &fakeExtension{
				manifest: testManifest(),
				routes: []extv1.RouteHandler{{
					Method: http.MethodGet, Pattern: pattern,
					Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
				}},
			}
			err := runOnce(t, ext, fullEnv())
			if err == nil {
				t.Fatalf("%q 를 통과시켰다", pattern)
			}
			if !strings.Contains(err.Error(), "예약") {
				t.Errorf("예약 서브경로 사유가 안내되지 않았다: %v", err)
			}
		})
	}
}

// TestRun_잘못된라우트거부 는 절대경로·nil 핸들러·중복 등록을 거부하는지 고정한다.
func TestRun_잘못된라우트거부(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

	cases := map[string][]extv1.RouteHandler{
		"절대API경로": {{Method: http.MethodGet, Pattern: "/api/extensions/test-ext/ping", Handler: handler}},
		"상대경로아님":  {{Method: http.MethodGet, Pattern: "ping", Handler: handler}},
		"핸들러nil":  {{Method: http.MethodGet, Pattern: "/ping"}},
		"중복등록": {
			{Method: http.MethodGet, Pattern: "/ping", Handler: handler},
			{Method: http.MethodGet, Pattern: "/ping", Handler: handler},
		},
	}
	for name, routes := range cases {
		t.Run(name, func(t *testing.T) {
			err := runOnce(t, &fakeExtension{manifest: testManifest(), routes: routes}, fullEnv())
			if err == nil {
				t.Fatal("잘못된 라우트를 통과시켰다")
			}
		})
	}
}

// TestRun_nil확장 은 배선 실수를 한국어로 알리는지 확인한다.
func TestRun_nil확장(t *testing.T) {
	if err := RunContext(context.Background(), nil); err == nil {
		t.Fatal("nil 확장으로 성공했다")
	}
}

// ── 실제 프로세스 핸드셰이크 통합 테스트 ─────────────────────────────────────

// helperEnvKey 는 자식 프로세스 모드 진입 표시다.
const helperEnvKey = "ABLEOPS_EXTSERVER_HELPER"

// TestHelperProcess 는 **자식 프로세스로 실행될 때만** 동작하는 확장 본체다.
// 부모 테스트(TestRun_실제프로세스핸드셰이크)가 이 테스트 바이너리를 다시 실행해 사용한다.
func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnvKey) != "1" {
		return // 일반 테스트 실행에서는 아무것도 하지 않는다
	}
	ext := &fakeExtension{
		manifest: testManifest(extv1.CapKafkaRead),
		routes:   []extv1.RouteHandler{echoIdentityRoute()},
	}
	// 기본 옵션 그대로 실행한다 — os.Stdout 핸드셰이크·신호·stdin 감시를 실제로 검증하기 위함이다.
	if err := Run(ext); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

// TestRun_실제프로세스핸드셰이크 는 Core 가 하는 일을 그대로 흉내 낸다:
// 프로세스를 띄우고 → stdout 한 줄을 파싱해 주소를 얻고 → 그 주소로 프록시하고 →
// stdin 을 닫아 종료시킨다(Windows 에는 SIGTERM 이 없으므로 이 경로가 실제 종료 수단이다).
func TestRun_실제프로세스핸드셰이크(t *testing.T) {
	if testing.Short() {
		t.Skip("짧은 모드에서는 프로세스 기동 테스트를 건너뛴다")
	}

	f := newFakeCore(t)
	f.json(http.MethodGet, hostPath("/ping"), http.StatusOK, map[string]any{"ok": true, "extensionId": "test-ext"})

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Env = append(os.Environ(),
		helperEnvKey+"=1",
		EnvExtensionID+"=test-ext",
		EnvExtensionVersion+"=1.2.3",
		EnvCallToken+"=call-token-값",
		EnvHostURL+"="+f.server.URL+"/api/extensions/_host",
		EnvHostToken+"=host-token-값",
		EnvCapabilities+"=kafka.read",
		EnvConfig+"=",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout 파이프 생성 실패: %v", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin 파이프 생성 실패: %v", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("확장 프로세스 기동 실패: %v", err)
	}
	killed := false
	defer func() {
		if !killed {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	// 1) 핸드셰이크 한 줄 — Core 는 이 줄을 기다린다(기본 15초).
	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, rerr := bufio.NewReader(stdout).ReadString('\n')
		if rerr != nil {
			errCh <- rerr
			return
		}
		lineCh <- line
	}()

	var ready ReadyMessage
	select {
	case line := <-lineCh:
		ready = parseReadyLine(t, strings.TrimRight(line, "\r\n"))
	case rerr := <-errCh:
		t.Fatalf("핸드셰이크를 읽지 못했다: %v (stderr: %s)", rerr, stderr.String())
	case <-time.After(20 * time.Second):
		t.Fatalf("핸드셰이크가 오지 않았다 (stderr: %s)", stderr.String())
	}

	// 2) 통보된 주소로 프록시 — Core 가 하는 일과 동일하다.
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, "http://"+ready.Addr+"/whoami", nil)
	req.Header.Set(HeaderCallToken, "call-token-값")
	SetIdentityHeaders(req.Header, extv1.Identity{UserID: "admin01", Name: "관리자", Department: "정보보호 1팀"})
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("확장 프로세스 호출 실패: %v (stderr: %s)", err, stderr.String())
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("상태코드가 다르다: %d", resp.StatusCode)
	}
	got := decodeBody[extv1.Identity](t, resp)
	if got.UserID != "admin01" || got.Name != "관리자" || got.Department != "정보보호 1팀" {
		t.Errorf("프로세스 경계를 넘은 신원이 다르다: %+v", got)
	}

	// 3) 토큰 없는 호출은 401 — 같은 호스트의 다른 프로세스가 붙어도 소용없어야 한다.
	bare, err := client.Get("http://" + ready.Addr + healthPath)
	if err != nil {
		t.Fatalf("헬스 호출 실패: %v", err)
	}
	_ = bare.Body.Close()
	if bare.StatusCode != http.StatusUnauthorized {
		t.Errorf("토큰 없는 호출이 401 이 아니다: %d", bare.StatusCode)
	}

	// 4) stdin 을 닫아 부모 종료를 알린다 → 확장은 스스로 정상 종료해야 한다.
	_ = stdin.Close()
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	select {
	case werr := <-waitCh:
		killed = true
		if werr != nil {
			t.Fatalf("확장 프로세스가 비정상 종료했다: %v (stderr: %s)", werr, stderr.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("stdin 종료 후에도 프로세스가 살아 있다 (stderr: %s)", stderr.String())
	}
}
