package extensionv1

// 외부 프로세스 Extension 프로토콜 — **API 버전과 분리된** wire 계약.
//
// # 왜 버전이 셋인가
//
//	Extension Version     확장 자신의 버전(1.2.0)          — 패키지 작성자가 올린다
//	API Version           SDK 계약 버전(…/extension/v1)     — Go 타입·인터페이스·Manifest 스키마
//	Protocol Version      프로세스 wire 계약 버전(1)         — 환경변수·핸드셰이크·헤더·Host API 경로
//
// 셋을 하나로 묶으면 둘 중 하나가 반드시 거짓말이 된다. 예를 들어 핸드셰이크 JSON 에 필드를
// 하나 더 실으려면 wire 계약은 바뀌지만 Go 타입은 그대로다 — 그걸 apiVersion 상승으로 표현하면
// 멀쩡한 확장이 전부 INCOMPATIBLE 이 되고, 반대로 아무 표시도 하지 않으면 Core 는 구버전
// 확장이 새 필드를 이해하는지 알 방법이 없다.
//
// # 하위호환 규칙
//
//	protocolVersion 필드가 없는 핸드셰이크 = 프로토콜 1(최초 버전). 거부하지 않는다.
//	Core 가 이해할 수 없는 프로토콜을 보고하면 그 Extension 만 INCOMPATIBLE 로 격리한다.
//	Core 가 요구하는 프로토콜을 SDK 가 모르면 Extension 이 기동 전에 스스로 실패를 알린다.

import "strconv"

const (
	// ProtocolVersion 은 이 SDK 가 구현하는 외부 프로세스 프로토콜 버전이다.
	ProtocolVersion = 1
	// MinProtocolVersion 은 이 SDK/Core 가 아직 받아 주는 최소 프로토콜 버전이다.
	//
	// 1 로 두는 한 "protocolVersion 을 보내지 않는 최초 릴리스 확장"이 계속 동작한다.
	MinProtocolVersion = 1
	// MaxProtocolVersion 은 이 SDK/Core 가 이해하는 최대 프로토콜 버전이다.
	MaxProtocolVersion = ProtocolVersion
)

const (
	// EnvProtocolVersion 은 Core 가 자식 프로세스에 알려 주는 프로토콜 버전 환경변수다.
	//
	// 구버전 Core 는 이 변수를 넣지 않는다 → 미설정은 "프로토콜 1" 로 해석한다.
	EnvProtocolVersion = "ABLEOPS_EXT_PROTOCOL"
	// EnvHostAPIVersion 은 Core 가 제공하는 Host API 버전 세그먼트다(예: "v1").
	//
	// 구버전 Core 는 이 변수를 넣지 않는다 → 미설정이면 SDK 는 **버전 없는 경로**를 쓴다
	// (구 Core 는 /v1 을 모르므로 404 가 된다).
	EnvHostAPIVersion = "ABLEOPS_HOST_API_VERSION"
)

// HostAPIVersion 은 이 SDK 가 사용하는 Host API 버전 세그먼트다.
//
// Core 는 `/api/extensions/_host/v1/...` 와 버전 없는 `/api/extensions/_host/...` 를 **둘 다**
// 받아 준다(구 경로는 호환 별칭이며 deprecated 다). 새 확장은 항상 버전 경로를 쓴다.
const HostAPIVersion = "v1"

// ReadyPrefix 는 기동 핸드셰이크 줄의 접두다(Extension → Core, stdout 한 줄).
//
//	ABLEOPS_EXT_READY {"addr":"127.0.0.1:54321","protocolVersion":1,…}
const ReadyPrefix = "ABLEOPS_EXT_READY"

// IncompatiblePrefix 는 **기동 전 호환성 실패**를 알리는 줄의 접두다(Extension → Core, stdout).
//
//	ABLEOPS_EXT_INCOMPATIBLE {"reason":"…","protocolVersion":1}
//
// 왜 별도 줄인가: 호환성 실패는 "프로세스가 죽었다"와 성질이 다르다. 그냥 종료하면 Core 는
// 크래시로 보고 FAILED 로 표시한 뒤 백오프를 두고 **계속 재시작**한다 — 몇 번을 다시 띄워도
// 결과가 같은데도. 이 줄을 보면 Core 는 재시작하지 않고 해당 확장만 INCOMPATIBLE 로 격리한다.
const IncompatiblePrefix = "ABLEOPS_EXT_INCOMPATIBLE"

// ReadyMessage 는 기동 핸드셰이크 줄의 JSON 본문이다.
//
// ⚠ 필드는 **추가만** 한다. 구버전 Core 는 모르는 필드를 무시하고, 구버전 확장은 새 필드를
// 보내지 않으므로 양방향 하위호환이 성립한다.
type ReadyMessage struct {
	// Addr 는 실제 리스닝 주소다(항상 루프백 — Core 가 검증한다).
	Addr string `json:"addr"`
	// Version 은 실행 중인 확장 버전이다(프로토콜 1의 원래 필드 — 계속 채운다).
	Version string `json:"version,omitempty"`
	// APIVersion 은 확장이 컴파일된 SDK 계약 버전이다(예: ableops.io/extension/v1).
	APIVersion string `json:"apiVersion,omitempty"`
	// ProtocolVersion 은 확장이 말하는 wire 프로토콜 버전이다(0/미기재 = 1).
	ProtocolVersion int `json:"protocolVersion,omitempty"`
}

// EffectiveProtocolVersion 은 핸드셰이크가 실제로 뜻하는 프로토콜 버전이다.
// 0(미기재)은 최초 버전으로 해석한다 — 구버전 확장을 거부하지 않기 위한 규칙이다.
func (m ReadyMessage) EffectiveProtocolVersion() int {
	if m.ProtocolVersion <= 0 {
		return MinProtocolVersion
	}
	return m.ProtocolVersion
}

// IncompatibleMessage 는 호환성 실패 줄의 JSON 본문이다.
type IncompatibleMessage struct {
	// Reason 은 화면·로그에 그대로 실을 한국어 사유다(시크릿 금지).
	Reason string `json:"reason"`
	// ProtocolVersion 은 이 확장이 구현하는 프로토콜 버전이다.
	ProtocolVersion int `json:"protocolVersion,omitempty"`
	// APIVersion 은 이 확장이 컴파일된 SDK 계약 버전이다.
	APIVersion string `json:"apiVersion,omitempty"`
}

// SupportsProtocolVersion 은 주어진 프로토콜 버전을 이 빌드가 처리할 수 있는지 판정한다.
func SupportsProtocolVersion(v int) bool {
	if v <= 0 {
		// 미기재는 최초 버전으로 본다.
		return true
	}
	return v >= MinProtocolVersion && v <= MaxProtocolVersion
}

// ParseProtocolVersion 은 환경변수 문자열을 프로토콜 버전으로 해석한다.
//
// 빈 값은 (MinProtocolVersion, true) — 구버전 Core 가 변수를 넣지 않은 경우다.
// 숫자가 아니면 (0, false) — 조용히 기본값으로 떨어뜨리지 않는다. 값이 깨졌다는 사실이
// 드러나야 "왜 확장이 이상하게 도는지"를 추적할 수 있다.
func ParseProtocolVersion(raw string) (int, bool) {
	if raw == "" {
		return MinProtocolVersion, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// ProtocolMismatchReason 은 프로토콜 불일치 사유 문구를 만든다(Core·SDK 공통 문구).
func ProtocolMismatchReason(got int) string {
	return "확장 기능이 보고한 프로세스 프로토콜 버전(" + strconv.Itoa(got) +
		")을 이 Core 가 지원하지 않습니다(지원 범위 " + strconv.Itoa(MinProtocolVersion) +
		"~" + strconv.Itoa(MaxProtocolVersion) + ") — 확장 기능 또는 Core 를 업데이트하세요"
}
