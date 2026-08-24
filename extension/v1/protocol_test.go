package extensionv1

// 외부 프로세스 프로토콜 계약 테스트 — 하위호환 규칙을 고정한다.

import (
	"encoding/json"
	"testing"
)

// TestProtocol_상수값 은 wire 계약 상수가 바뀌지 않음을 고정한다.
func TestProtocol_상수값(t *testing.T) {
	if ProtocolVersion != 1 || MinProtocolVersion != 1 || MaxProtocolVersion != ProtocolVersion {
		t.Fatalf("프로토콜 버전 상수가 바뀌었다: %d %d %d", ProtocolVersion, MinProtocolVersion, MaxProtocolVersion)
	}
	if ReadyPrefix != "ABLEOPS_EXT_READY" || IncompatiblePrefix != "ABLEOPS_EXT_INCOMPATIBLE" {
		t.Fatalf("핸드셰이크 접두가 바뀌었다: %q %q", ReadyPrefix, IncompatiblePrefix)
	}
	if EnvProtocolVersion != "ABLEOPS_EXT_PROTOCOL" || EnvHostAPIVersion != "ABLEOPS_HOST_API_VERSION" {
		t.Fatalf("환경변수 이름이 바뀌었다: %q %q", EnvProtocolVersion, EnvHostAPIVersion)
	}
	if HostAPIVersion != "v1" {
		t.Fatalf("Host API 버전 세그먼트가 바뀌었다: %q", HostAPIVersion)
	}
}

// TestReadyMessage_구버전호환 은 protocolVersion 을 보내지 않는 확장이 계속 동작함을 고정한다.
//
// ⚠ 이 테스트가 깨지면 Core 를 올리는 순간 기존 외부 프로세스 확장이 전부 기동 실패한다.
func TestReadyMessage_구버전호환(t *testing.T) {
	var m ReadyMessage
	if err := json.Unmarshal([]byte(`{"addr":"127.0.0.1:5000","version":"1.0.0"}`), &m); err != nil {
		t.Fatalf("구 형식 파싱 실패: %v", err)
	}
	if m.EffectiveProtocolVersion() != MinProtocolVersion {
		t.Errorf("미기재 프로토콜은 최초 버전으로 봐야 한다: %d", m.EffectiveProtocolVersion())
	}
	if !SupportsProtocolVersion(m.EffectiveProtocolVersion()) {
		t.Error("구 형식 확장이 거부됐다")
	}
	if m.APIVersion != "" {
		t.Errorf("없는 필드가 채워졌다: %q", m.APIVersion)
	}
}

// TestReadyMessage_신형식 은 새 필드가 왕복하는지 확인한다.
func TestReadyMessage_신형식(t *testing.T) {
	in := ReadyMessage{Addr: "127.0.0.1:5000", Version: "1.2.0", APIVersion: APIVersion, ProtocolVersion: 1}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("직렬화 실패: %v", err)
	}
	var out ReadyMessage
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("역직렬화 실패: %v", err)
	}
	if out != in {
		t.Errorf("왕복이 어긋났다: %+v != %+v", out, in)
	}
}

// TestSupportsProtocolVersion 은 지원 범위 판정을 고정한다.
func TestSupportsProtocolVersion(t *testing.T) {
	if !SupportsProtocolVersion(0) {
		t.Error("미기재(0)는 최초 버전으로 받아 줘야 한다")
	}
	if !SupportsProtocolVersion(1) {
		t.Error("1 이 거부됐다")
	}
	if SupportsProtocolVersion(MaxProtocolVersion + 1) {
		t.Error("모르는 상위 프로토콜이 통과했다")
	}
}

// TestParseProtocolVersion 은 환경변수 해석 규칙을 고정한다.
func TestParseProtocolVersion(t *testing.T) {
	if v, ok := ParseProtocolVersion(""); !ok || v != MinProtocolVersion {
		t.Errorf("빈 값은 최초 버전이어야 한다: %d %v", v, ok)
	}
	if v, ok := ParseProtocolVersion("2"); !ok || v != 2 {
		t.Errorf("숫자 해석이 다르다: %d %v", v, ok)
	}
	// 깨진 값을 조용히 기본값으로 떨어뜨리지 않는다.
	for _, raw := range []string{"abc", "-1", "0", "1.0"} {
		if _, ok := ParseProtocolVersion(raw); ok {
			t.Errorf("잘못된 값이 통과했다: %q", raw)
		}
	}
}
