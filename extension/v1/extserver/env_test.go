package extserver

// 환경변수 계약(프로토콜 §1) 검증.
//
// 여기서 고정하는 것은 "값을 읽는가"가 아니라 **틀린 기동을 확실히 거부하는가**다:
// 직접 실행·잘못된 바이너리·형식이 깨진 설정은 조용히 넘어가지 않고 한국어로 실패해야 한다.

import (
	"strings"
	"testing"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

// testManifest 는 검증을 통과하는 최소 Manifest 를 만든다.
func testManifest(caps ...extv1.Capability) extv1.Manifest {
	return extv1.Manifest{
		APIVersion:   extv1.APIVersion,
		ID:           "test-ext",
		Name:         "테스트 확장",
		Version:      "1.2.3",
		Publisher:    "AbleOps",
		Capabilities: caps,
		Permissions: []extv1.PermissionDecl{
			{Key: extv1.PermissionKey("test-ext", "view"), Label: "조회", Roles: []string{"SystemAdmin"}},
		},
		Backend: extv1.BackendDecl{Enabled: true, Kind: extv1.BackendKindProcess},
		Routes: []extv1.RouteDecl{
			{Path: "/api/extensions/test-ext", Permission: extv1.PermissionKey("test-ext", "view")},
		},
	}
}

// fullEnv 는 정상 기동 환경변수 집합이다.
func fullEnv() map[string]string {
	return map[string]string{
		EnvExtensionID:      "test-ext",
		EnvExtensionVersion: "1.2.3",
		EnvCallToken:        "call-token-값",
		EnvHostURL:          "http://127.0.0.1:8080/api/extensions/_host",
		EnvHostToken:        "host-token-값",
		EnvCapabilities:     "kafka.read,cluster.read",
		EnvConfig:           `{"greeting":"안녕하세요"}`,
	}
}

// lookupFrom 은 맵 기반 환경변수 조회 함수를 만든다.
func lookupFrom(m map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

// TestLoadEnvironment_정상 은 전체 환경이 갖춰졌을 때의 파싱 결과를 고정한다.
func TestLoadEnvironment_정상(t *testing.T) {
	env, err := loadEnvironment(lookupFrom(fullEnv()), testManifest(extv1.CapKafkaRead, extv1.CapClusterRead))
	if err != nil {
		t.Fatalf("정상 환경에서 실패했다: %v", err)
	}
	if env.ExtensionID != "test-ext" || env.Version != "1.2.3" {
		t.Errorf("ID/버전이 다르다: %+v", env)
	}
	if env.CallToken != "call-token-값" || env.HostToken != "host-token-값" {
		t.Errorf("토큰이 읽히지 않았다")
	}
	// 끝의 / 는 제거되고 나머지 경로는 보존되어야 한다(Host API 는 하위 경로를 붙여 호출한다).
	if env.HostURL != "http://127.0.0.1:8080/api/extensions/_host" {
		t.Errorf("HostURL 이 다르다: %q", env.HostURL)
	}
	if len(env.Capabilities) != 2 || !env.HasCapability(extv1.CapKafkaRead) || !env.HasCapability(extv1.CapClusterRead) {
		t.Errorf("capability 파싱이 다르다: %v", env.Capabilities)
	}
	if env.Config[cfgGreetingKey] != "안녕하세요" {
		t.Errorf("설정 JSON 이 파싱되지 않았다: %+v", env.Config)
	}
}

// cfgGreetingKey 는 테스트 설정 키다.
const cfgGreetingKey = "greeting"

// TestLoadEnvironment_필수누락시한국어오류 는 직접 실행을 정확한 안내로 거부하는지 고정한다.
//
// 이 안내가 없으면 사람이 바이너리를 직접 실행했을 때 "포트 바인딩 실패" 같은
// 엉뚱한 오류만 보게 되어 원인을 찾지 못한다.
func TestLoadEnvironment_필수누락시한국어오류(t *testing.T) {
	for _, key := range []string{EnvExtensionID, EnvCallToken, EnvHostURL, EnvHostToken} {
		t.Run(key, func(t *testing.T) {
			m := fullEnv()
			delete(m, key)
			_, err := loadEnvironment(lookupFrom(m), testManifest())
			if err == nil {
				t.Fatalf("%s 누락인데 성공했다", key)
			}
			if !strings.Contains(err.Error(), DirectRunMessage) {
				t.Errorf("직접 실행 안내가 없다: %v", err)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("누락된 환경변수 이름(%s)이 안내에 없다: %v", key, err)
			}
		})
	}
}

// TestLoadEnvironment_전부누락시모든이름안내 는 누락 목록을 한 번에 알려주는지 확인한다.
func TestLoadEnvironment_전부누락시모든이름안내(t *testing.T) {
	_, err := loadEnvironment(lookupFrom(map[string]string{}), testManifest())
	if err == nil {
		t.Fatal("빈 환경에서 성공했다")
	}
	for _, key := range []string{EnvExtensionID, EnvCallToken, EnvHostURL, EnvHostToken} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("%s 가 누락 안내에 없다: %v", key, err)
		}
	}
}

// TestLoadEnvironment_ID불일치거부 는 다른 확장의 환경으로 뜬 바이너리를 막는지 확인한다.
// 통과시키면 이 확장이 **다른 확장의 권한·설정으로** 동작하게 된다.
func TestLoadEnvironment_ID불일치거부(t *testing.T) {
	m := fullEnv()
	m[EnvExtensionID] = "other-ext"
	_, err := loadEnvironment(lookupFrom(m), testManifest())
	if err == nil {
		t.Fatal("ID 불일치인데 성공했다")
	}
	if !strings.Contains(err.Error(), "other-ext") || !strings.Contains(err.Error(), "test-ext") {
		t.Errorf("양쪽 ID 가 안내에 없다: %v", err)
	}
}

// TestLoadEnvironment_HostURL검증 은 잘못된 Host API 주소를 거부하는지 확인한다.
func TestLoadEnvironment_HostURL검증(t *testing.T) {
	for _, tc := range []struct{ name, url string }{
		{"스킴없음", "127.0.0.1:8080/api"},
		{"호스트없음", "http://"},
		{"파일스킴", "file:///etc/passwd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := fullEnv()
			m[EnvHostURL] = tc.url
			if _, err := loadEnvironment(lookupFrom(m), testManifest()); err == nil {
				t.Fatalf("%q 를 통과시켰다", tc.url)
			}
		})
	}

	// 끝의 슬래시는 정규화된다(호출 경로를 붙일 때 // 가 생기지 않도록).
	m := fullEnv()
	m[EnvHostURL] = "http://127.0.0.1:8080/api/extensions/_host/"
	env, err := loadEnvironment(lookupFrom(m), testManifest())
	if err != nil {
		t.Fatalf("정상 URL 이 거부되었다: %v", err)
	}
	if strings.HasSuffix(env.HostURL, "/") {
		t.Errorf("끝의 / 가 제거되지 않았다: %q", env.HostURL)
	}
}

// TestLoadEnvironment_capability교집합 은 최소권한 해석을 고정한다.
//
// 환경변수(Core 가 허용한 값)와 Manifest(확장이 요구한 값) **양쪽에 있는 것만** 사용한다.
// 한쪽에만 있는 capability 를 쓰면 둘 중 하나의 계약을 어기게 된다.
func TestLoadEnvironment_capability교집합(t *testing.T) {
	m := fullEnv()
	// Core 는 3종을 넘겼지만 Manifest 는 kafka.read 만 선언했다.
	m[EnvCapabilities] = "kafka.read,cluster.read,workflow.submit"
	env, err := loadEnvironment(lookupFrom(m), testManifest(extv1.CapKafkaRead))
	if err != nil {
		t.Fatalf("실패했다: %v", err)
	}
	if len(env.Capabilities) != 1 || !env.HasCapability(extv1.CapKafkaRead) {
		t.Fatalf("교집합이 아니다: %v", env.Capabilities)
	}
	if env.HasCapability(extv1.CapWorkflowSubmit) {
		t.Errorf("Manifest 가 선언하지 않은 capability 가 허용되었다")
	}
}

// TestLoadEnvironment_capability환경변수없으면Manifest 는 구버전 Core 하위호환을 고정한다.
func TestLoadEnvironment_capability환경변수없으면Manifest(t *testing.T) {
	m := fullEnv()
	delete(m, EnvCapabilities)
	env, err := loadEnvironment(lookupFrom(m), testManifest(extv1.CapKafkaRead, extv1.CapAuditWrite))
	if err != nil {
		t.Fatalf("실패했다: %v", err)
	}
	if len(env.Capabilities) != 2 {
		t.Errorf("Manifest 선언분을 그대로 써야 한다: %v", env.Capabilities)
	}

	// 반대로 "설정되었지만 빈 값"은 capability 없음이다(Core 가 명시적으로 아무것도 주지 않은 경우).
	m[EnvCapabilities] = ""
	env, err = loadEnvironment(lookupFrom(m), testManifest(extv1.CapKafkaRead))
	if err != nil {
		t.Fatalf("실패했다: %v", err)
	}
	if len(env.Capabilities) != 0 {
		t.Errorf("빈 값은 capability 없음이어야 한다: %v", env.Capabilities)
	}
}

// TestLoadEnvironment_모르는capability는무시 는 상위 Core 와의 전방호환을 고정한다.
func TestLoadEnvironment_모르는capability는무시(t *testing.T) {
	m := fullEnv()
	m[EnvCapabilities] = "kafka.read,future.capability"
	env, err := loadEnvironment(lookupFrom(m), testManifest(extv1.CapKafkaRead))
	if err != nil {
		t.Fatalf("모르는 capability 때문에 기동이 실패했다: %v", err)
	}
	if len(env.UnknownCapabilities) != 1 || env.UnknownCapabilities[0] != "future.capability" {
		t.Errorf("모르는 capability 를 보고하지 않았다: %v", env.UnknownCapabilities)
	}
}

// TestLoadEnvironment_설정JSON깨짐거부 는 설정 파싱 실패를 조용히 넘기지 않는지 확인한다.
func TestLoadEnvironment_설정JSON깨짐거부(t *testing.T) {
	m := fullEnv()
	m[EnvConfig] = `{"greeting":`
	_, err := loadEnvironment(lookupFrom(m), testManifest())
	if err == nil {
		t.Fatal("깨진 설정 JSON 을 통과시켰다")
	}
	if !strings.Contains(err.Error(), EnvConfig) {
		t.Errorf("어떤 환경변수가 문제인지 알 수 없다: %v", err)
	}
}

// TestEnvironment_String이토큰을가린다 는 **토큰 유출 방지**를 고정한다.
//
// Environment 를 %v 로 찍는 코드가 어디에 생기더라도 토큰이 로그에 남으면 안 된다.
func TestEnvironment_String이토큰을가린다(t *testing.T) {
	env, err := loadEnvironment(lookupFrom(fullEnv()), testManifest(extv1.CapKafkaRead, extv1.CapClusterRead))
	if err != nil {
		t.Fatalf("실패했다: %v", err)
	}
	s := env.String()
	if strings.Contains(s, "call-token-값") || strings.Contains(s, "host-token-값") {
		t.Fatalf("토큰이 문자열 표현에 노출되었다: %s", s)
	}
	if !strings.Contains(s, tokenMask) {
		t.Errorf("마스킹 표기가 없다: %s", s)
	}
	if !strings.Contains(s, "test-ext") {
		t.Errorf("확장 ID 는 남아야 한다(진단 정보): %s", s)
	}
}
