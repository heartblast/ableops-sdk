package extserver

// 프로토콜 협상·Host API 버전·capability 협상 검증(요구 §13·§14·§15).

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	extv1 "github.com/heartblast/kafka-control-portal/sdk/extension/v1"
)

// TestLoadEnvironment_프로토콜미설정은최초버전 은 구버전 Core 하위호환을 고정한다.
//
// ⚠ 이 테스트가 깨지면 구버전 Core 에 설치된 확장이 기동하지 못한다.
func TestLoadEnvironment_프로토콜미설정은최초버전(t *testing.T) {
	env, err := loadEnvironment(lookupFrom(fullEnv()), testManifest(extv1.CapKafkaRead))
	if err != nil {
		t.Fatalf("실패했다: %v", err)
	}
	if env.ProtocolVersion != extv1.MinProtocolVersion {
		t.Errorf("미설정은 최초 버전이어야 한다: %d", env.ProtocolVersion)
	}
	if env.HostAPIVersion != "" {
		t.Errorf("미설정 Host API 버전은 빈 값이어야 한다: %q", env.HostAPIVersion)
	}
	// 버전 세그먼트 없이 구 경로를 쓴다(구 Core 는 /v1 을 모른다).
	if got := env.HostAPIPath("/ping"); got != "/ping" {
		t.Errorf("구 Core 에서는 버전 없는 경로여야 한다: %q", got)
	}
}

// TestLoadEnvironment_상위프로토콜은거부 는 Core 가 더 새로운 프로토콜을 요구할 때
// 확장이 **기동 전에** 실패를 알리는지 고정한다.
func TestLoadEnvironment_상위프로토콜은거부(t *testing.T) {
	m := fullEnv()
	m[EnvProtocolVersion] = "99"
	_, err := loadEnvironment(lookupFrom(m), testManifest(extv1.CapKafkaRead))
	if err == nil {
		t.Fatal("모르는 상위 프로토콜이 통과했다")
	}
	var mismatch *protocolMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("프로토콜 불일치 오류로 분류되지 않았다: %T %v", err, err)
	}
	if !strings.Contains(err.Error(), "99") {
		t.Errorf("사유에 요구 버전이 없다: %v", err)
	}
}

// TestLoadEnvironment_깨진프로토콜값 은 잘못된 값을 조용히 기본값으로 떨어뜨리지 않음을 고정한다.
func TestLoadEnvironment_깨진프로토콜값(t *testing.T) {
	m := fullEnv()
	m[EnvProtocolVersion] = "일"
	if _, err := loadEnvironment(lookupFrom(m), testManifest()); err == nil {
		t.Fatal("깨진 프로토콜 값이 통과했다")
	}
}

// TestLoadEnvironment_HostAPI버전 은 Core 가 알려 준 버전으로 경로를 만드는지 고정한다.
func TestLoadEnvironment_HostAPI버전(t *testing.T) {
	m := fullEnv()
	m[EnvProtocolVersion] = "1"
	m[EnvHostAPIVersion] = "v1"
	env, err := loadEnvironment(lookupFrom(m), testManifest(extv1.CapKafkaRead))
	if err != nil {
		t.Fatalf("실패했다: %v", err)
	}
	if got := env.HostAPIPath("/kafka/topics"); got != "/v1/kafka/topics" {
		t.Errorf("버전 경로가 다르다: %q", got)
	}
	// 형식이 이상한 값은 버전 없는 경로로 떨어뜨린다(모든 호출이 404 가 되는 것보다 낫다).
	m[EnvHostAPIVersion] = "v1/extra"
	env, err = loadEnvironment(lookupFrom(m), testManifest(extv1.CapKafkaRead))
	if err != nil {
		t.Fatalf("실패했다: %v", err)
	}
	if got := env.HostAPIPath("/ping"); got != "/ping" {
		t.Errorf("이상한 버전값은 무시해야 한다: %q", got)
	}
}

// TestLoadEnvironment_capability협상3분류 는 §15 의 세 오류를 구분하는지 고정한다.
func TestLoadEnvironment_capability협상3분류(t *testing.T) {
	m := fullEnv()
	// Core 는 kafka.read(정상) · secret.ref(SDK 미구현) · future.thing(모르는 이름)을 허용했고,
	// Manifest 는 cluster.read 도 선언했지만 Core 가 주지 않았다.
	m[EnvCapabilities] = "kafka.read,secret.ref,future.thing"
	env, err := loadEnvironment(lookupFrom(m), testManifest(extv1.CapKafkaRead, extv1.CapClusterRead, extv1.CapSecretRef))
	if err != nil {
		t.Fatalf("협상 실패로 기동이 막혔다(격리 원칙 위반): %v", err)
	}
	if len(env.Capabilities) != 1 || env.Capabilities[0] != extv1.CapKafkaRead {
		t.Errorf("실사용 capability 가 다르다: %v", env.Capabilities)
	}
	if len(env.MissingCapabilities) != 1 || env.MissingCapabilities[0] != extv1.CapClusterRead {
		t.Errorf("Core 미허용 capability 를 구분하지 못했다: %v", env.MissingCapabilities)
	}
	if len(env.UnsupportedCapabilities) != 1 || env.UnsupportedCapabilities[0] != extv1.CapSecretRef {
		t.Errorf("SDK 미구현 capability 를 구분하지 못했다: %v", env.UnsupportedCapabilities)
	}
	if len(env.UnknownCapabilities) != 1 || env.UnknownCapabilities[0] != "future.thing" {
		t.Errorf("모르는 capability 를 구분하지 못했다: %v", env.UnknownCapabilities)
	}
}

// TestEnvironment_String은토큰을가린다 는 새 필드를 추가해도 마스킹이 유지되는지 확인한다.
func TestEnvironment_String은토큰을가린다(t *testing.T) {
	env, err := loadEnvironment(lookupFrom(fullEnv()), testManifest(extv1.CapKafkaRead))
	if err != nil {
		t.Fatalf("실패했다: %v", err)
	}
	s := env.String()
	if strings.Contains(s, "call-token-값") || strings.Contains(s, "host-token-값") {
		t.Fatalf("토큰이 노출됐다: %s", s)
	}
}

// TestRun_프로토콜불일치는INCOMPATIBLE줄을출력 은 Core 가 재시작을 멈출 수 있게
// 확장이 사유를 stdout 한 줄로 알리는지 고정한다.
//
// ⚠ 이 줄이 없으면 Core 는 크래시로 보고 백오프를 두고 영원히 재기동을 반복한다.
func TestRun_프로토콜불일치는INCOMPATIBLE줄을출력(t *testing.T) {
	m := fullEnv()
	m[EnvProtocolVersion] = "99"
	var stdout strings.Builder
	err := RunContext(context.Background(), &protoTestExt{},
		withLookupEnv(lookupFrom(m)), WithStdout(&stdout), WithLogWriter(io.Discard),
		withSignalHandling(false), withStdinWatch(false))
	if err == nil {
		t.Fatal("프로토콜 불일치인데 기동에 성공했다")
	}
	line := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(line, extv1.IncompatiblePrefix) {
		t.Fatalf("호환성 실패 줄이 없다: %q", line)
	}
	var msg extv1.IncompatibleMessage
	payload := strings.TrimSpace(strings.TrimPrefix(line, extv1.IncompatiblePrefix))
	if uerr := json.Unmarshal([]byte(payload), &msg); uerr != nil {
		t.Fatalf("호환성 실패 줄을 해석할 수 없다(%q): %v", payload, uerr)
	}
	if msg.Reason == "" || msg.ProtocolVersion != extv1.ProtocolVersion {
		t.Errorf("호환성 실패 본문이 부실하다: %+v", msg)
	}
}

// TestRun_핸드셰이크에프로토콜정보포함 은 정상 기동 시 새 필드를 채우는지 고정한다.
func TestRun_핸드셰이크에프로토콜정보포함(t *testing.T) {
	m := fullEnv()
	m[EnvProtocolVersion] = "1"
	m[EnvHostAPIVersion] = "v1"
	var stdout strings.Builder

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RunContext(ctx, &protoTestExt{},
			withLookupEnv(lookupFrom(m)), WithStdout(&stdout), WithLogWriter(io.Discard),
			withSignalHandling(false), withStdinWatch(false))
	}()
	// 핸드셰이크가 나올 때까지 잠깐 기다린다.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(stdout.String(), extv1.ReadyPrefix) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("기동이 실패했다: %v", err)
	}

	line := strings.TrimSpace(stdout.String())
	payload := strings.TrimSpace(strings.TrimPrefix(line, extv1.ReadyPrefix))
	var ready ReadyMessage
	if err := json.Unmarshal([]byte(payload), &ready); err != nil {
		t.Fatalf("핸드셰이크를 해석할 수 없다(%q): %v", payload, err)
	}
	if ready.ProtocolVersion != extv1.ProtocolVersion {
		t.Errorf("프로토콜 버전이 없다: %+v", ready)
	}
	if ready.APIVersion != extv1.APIVersion {
		t.Errorf("API 버전이 없다: %+v", ready)
	}
	if ready.Addr == "" || ready.Version != "1.2.3" {
		t.Errorf("기존 필드가 유실됐다: %+v", ready)
	}
}

// TestConfigService_write미선언이면거부 는 config.read 만으로 저장할 수 없음을 고정한다.
func TestConfigService_write미선언이면거부(t *testing.T) {
	f := newFakeCore(t)
	f.on(http.MethodPut, hostPath("/config"), func(w http.ResponseWriter, r *http.Request) {
		t.Error("호출되면 안 된다 — SDK 가 먼저 막아야 한다")
		w.WriteHeader(http.StatusNoContent)
	})
	c, _ := newTestClient(t, f)
	svc := &configService{c: c} // canWrite=false

	err := svc.Set(context.Background(), map[string]any{"greeting": "안녕"})
	if err == nil {
		t.Fatal("config.write 없이 저장이 성공했다")
	}
	if extv1.CodeOf(err) != extv1.CodePermissionDenied {
		t.Errorf("오류 코드가 다르다: %q (%v)", extv1.CodeOf(err), err)
	}
}

// TestHostClient_버전경로 는 Core 가 알려 준 버전 세그먼트가 실제 요청 경로에 붙는지 고정한다.
func TestHostClient_버전경로(t *testing.T) {
	f := newFakeCore(t)
	f.json(http.MethodGet, hostPath("/v1/ping"), http.StatusOK, map[string]any{"ok": true, "extensionId": "test-ext"})

	c, _ := newTestClient(t, f)
	c.apiPath = func(p string) string { return "/v1" + p }
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("버전 경로 호출이 실패했다: %v", err)
	}
	reqs := f.recorded()
	if len(reqs) == 0 || reqs[0].Path != hostPath("/v1/ping") {
		t.Fatalf("버전 경로가 붙지 않았다: %+v", reqs)
	}
}

// TestHostClient_오류코드보존 은 Core 응답의 코드를 그대로 전달하는지 고정한다.
func TestHostClient_오류코드보존(t *testing.T) {
	f := newFakeCore(t)
	f.on(http.MethodGet, hostPath("/clusters"), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "이 확장 기능은 cluster.read capability 를 선언하지 않았습니다",
			"code":  string(extv1.CodeCapabilityUnavailable),
		})
	})
	c, _ := newTestClient(t, f)
	_, err := (clusterService{c: c}).List(context.Background())
	if err == nil {
		t.Fatal("403 인데 성공했다")
	}
	if HostErrorCode(err) != extv1.CodeCapabilityUnavailable {
		t.Errorf("코드가 보존되지 않았다: %q", HostErrorCode(err))
	}
	// in-process 확장과 **같은 방식**으로 분기할 수 있어야 한다.
	if !errors.Is(err, extv1.NewError(extv1.CodeCapabilityUnavailable, "")) {
		t.Error("errors.Is 코드 비교가 성립하지 않는다")
	}
}

// TestHostClient_코드없는구버전Core 는 상태코드 폴백을 고정한다.
func TestHostClient_코드없는구버전Core(t *testing.T) {
	f := newFakeCore(t)
	f.on(http.MethodGet, hostPath("/clusters"), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "설치되지 않은 확장 기능입니다"})
	})
	c, _ := newTestClient(t, f)
	_, err := (clusterService{c: c}).List(context.Background())
	if HostErrorCode(err) != extv1.CodeNotFound {
		t.Errorf("상태코드 폴백이 동작하지 않는다: %q", HostErrorCode(err))
	}
}

// protoTestExt 는 프로토콜 테스트용 최소 확장이다.
type protoTestExt struct{}

func (*protoTestExt) Manifest() extv1.Manifest                       { return testManifest(extv1.CapKafkaRead) }
func (*protoTestExt) Start(context.Context, extv1.HostContext) error { return nil }
func (*protoTestExt) Stop(context.Context) error                     { return nil }
func (*protoTestExt) Routes() []extv1.RouteHandler                   { return nil }
func (*protoTestExt) Health(context.Context) extv1.Health {
	return extv1.Health{OK: true, Message: "정상", CheckedAt: time.Now()}
}
