package extserver

// 외부 프로세스 Extension 의 환경변수 계약(프로토콜 §1).
//
// Core 는 Extension 프로세스를 기동하며 아래 환경변수를 주입한다. 이 파일이 그것을 읽고 검증해
// Environment 로 만드는 **유일한 지점**이다 — 다른 파일에서 os.Getenv 를 직접 부르지 않는다.
//
// ⚠ 평문 시크릿은 어떤 환경변수로도 전달되지 않는다(요구A §19). 여기서 읽는 두 토큰은
// 호출 인증용이며 로그·오류 메시지에 절대 싣지 않는다(Environment.String 이 마스킹한다).

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

// Core 가 주입하는 환경변수 이름.
const (
	// EnvExtensionID 는 확장 ID 다(Manifest 의 id 와 일치해야 한다).
	EnvExtensionID = "ABLEOPS_EXT_ID"
	// EnvExtensionVersion 은 Core 에 설치된 버전이다(비면 Manifest 의 version 을 쓴다).
	EnvExtensionVersion = "ABLEOPS_EXT_VERSION"
	// EnvCallToken 은 Core 가 보내는 요청을 Extension 이 검증할 토큰이다.
	EnvCallToken = "ABLEOPS_EXT_CALL_TOKEN"
	// EnvHostURL 은 Core Host API 의 베이스 URL 이다.
	EnvHostURL = "ABLEOPS_HOST_URL"
	// EnvHostToken 은 Extension 이 Host API 를 호출할 때 쓰는 Bearer 토큰이다.
	EnvHostToken = "ABLEOPS_HOST_TOKEN"
	// EnvCapabilities 는 쉼표로 구분한 capability 목록이다(Manifest 선언분).
	EnvCapabilities = "ABLEOPS_EXT_CAPABILITIES"
	// EnvConfig 는 Extension 설정 JSON 이다(없으면 빈 문자열).
	EnvConfig = "ABLEOPS_EXT_CONFIG"
	// EnvProtocolVersion 은 Core 가 요구하는 프로세스 프로토콜 버전이다(미설정 = 1).
	EnvProtocolVersion = extv1.EnvProtocolVersion
	// EnvHostAPIVersion 은 Core 가 제공하는 Host API 버전 세그먼트다(미설정 = 버전 없는 구 경로).
	EnvHostAPIVersion = extv1.EnvHostAPIVersion
)

// DirectRunMessage 는 사람이 이 바이너리를 직접 실행했을 때의 안내다.
//
// 외부 프로세스 Extension 은 Core 가 환경변수·토큰을 주입해야만 동작한다. 그것 없이 실행하면
// "포트 바인딩 실패" 같은 엉뚱한 오류로 끝나기 쉬우므로, 첫 줄에서 원인을 정확히 알려준다.
const DirectRunMessage = "이 프로그램은 AbleOps Core 가 실행합니다 — 직접 실행할 수 없습니다"

// tokenMask 는 마스킹 표기다(토큰 원문은 어떤 경로로도 출력하지 않는다).
const tokenMask = "***"

// Environment 는 검증을 마친 기동 환경이다.
//
// CallToken·HostToken 은 시크릿이다. 이 구조체를 %v/%+v 로 찍어도 새지 않도록
// String() 이 마스킹된 요약을 돌려준다(fmt 는 String() 을 우선한다).
type Environment struct {
	// ExtensionID 는 확장 ID 다.
	ExtensionID string
	// Version 은 실행 중인 확장 버전이다(환경변수 우선, 없으면 Manifest).
	Version string
	// CallToken 은 Core → Extension 요청을 검증할 토큰이다(로그 금지).
	CallToken string
	// HostURL 은 Host API 베이스 URL 이다(끝의 / 는 제거된다).
	HostURL string
	// HostToken 은 Extension → Core 호출용 Bearer 토큰이다(로그 금지).
	HostToken string
	// Capabilities 는 실제로 사용할 수 있는 capability 다(환경변수 ∩ Manifest).
	Capabilities []extv1.Capability
	// UnknownCapabilities 는 이 SDK 가 모르는 capability 이름이다(무시하고 경고만 남긴다).
	UnknownCapabilities []string
	// Config 는 Core 가 넘긴 설정 스냅샷이다(Host API 조회 실패 시의 폴백).
	Config map[string]any

	// MissingCapabilities 는 Manifest 가 선언했는데 **Core 가 허용하지 않은** capability 다.
	//
	// 정상적인 설치에서는 비어 있다(Core 는 Manifest 선언분을 그대로 넘긴다).
	//
	// ⚠ **기동을 막지 않는다.** Manifest 에는 "이 capability 가 없으면 아예 못 돈다"를 표현하는
	// 수단(선택/필수 구분)이 없고, Core 가 설치 시점에 특정 capability 를 거절하는 정책을 갖게
	// 되는 미래도 열려 있다. 그 상황에서 확장 전체를 죽이면 화면 기여까지 함께 사라진다.
	// 대신 기동 로그에 사유를 남기고, 해당 기능 사용 시 CAPABILITY_UNAVAILABLE 을 돌려준다.
	MissingCapabilities []extv1.Capability

	// UnsupportedCapabilities 는 Core 가 허용했지만 **이 SDK 가 클라이언트 구현을 제공하지 않는**
	// capability 다(외부 프로세스에서의 secret.ref 등).
	//
	// 기동은 막지 않는다. Core 쪽 계약은 정상이고 SDK 만 아직 못 따라간 상태이므로, 해당 기능을
	// 쓰지 않는 확장은 정상 동작해야 한다. 사용을 시도하면 RequireXxx 가 CAPABILITY_UNAVAILABLE
	// 을 돌려준다.
	UnsupportedCapabilities []extv1.Capability

	// ProtocolVersion 은 Core 가 요구한 프로세스 프로토콜 버전이다(미설정 = 최초 버전).
	ProtocolVersion int

	// HostAPIVersion 은 Core 가 제공하는 Host API 버전 세그먼트다(빈 값 = 버전 없는 구 경로).
	HostAPIVersion string
}

// processCapabilityClients 는 이 SDK 가 **외부 프로세스** 경로에서 클라이언트 구현을 제공하는
// capability 다(hostclient.go 의 서비스 구현과 1:1).
//
// ⚠ 여기 없는 capability 는 Core 가 허용해도 HostContext 에 주입되지 않는다.
// 임의로 이름을 추가하면 존재하지 않는 Host API 경로를 부르게 되므로, 반드시 구현과 함께 넣는다.
var processCapabilityClients = map[extv1.Capability]bool{
	extv1.CapKafkaRead:      true,
	extv1.CapClusterRead:    true,
	extv1.CapWorkflowSubmit: true,
	extv1.CapAuditWrite:     true,
	extv1.CapConfigRead:     true,
	extv1.CapConfigWrite:    true,
	// secret.ref 는 외부 프로세스 Host 프로토콜에 경로가 없다(hostclient.go 참조).
	extv1.CapSecretRef: false,
}

// String 은 마스킹된 요약을 돌려준다(토큰 유출 방지).
func (e Environment) String() string {
	caps := make([]string, 0, len(e.Capabilities))
	for _, c := range e.Capabilities {
		caps = append(caps, string(c))
	}
	return fmt.Sprintf("Environment{id:%s version:%s protocol:%d hostApi:%s hostUrl:%s callToken:%s hostToken:%s capabilities:[%s] configKeys:%d}",
		e.ExtensionID, e.Version, e.ProtocolVersion, e.hostAPIVersionForLog(), e.HostURL,
		tokenMask, tokenMask, strings.Join(caps, " "), len(e.Config))
}

// hostAPIVersionForLog 는 로그 표시용 Host API 버전이다(미설정이면 표시 문구).
func (e Environment) hostAPIVersionForLog() string {
	if e.HostAPIVersion == "" {
		return "(버전 없음)"
	}
	return e.HostAPIVersion
}

// HostAPIPath 는 Host API 상대 경로에 버전 세그먼트를 붙인다.
//
// Core 가 버전을 알려 주지 않으면(구버전 Core) 버전 없는 경로를 그대로 쓴다 —
// 구 Core 는 /v1 세그먼트를 모르므로 붙이면 전부 404 가 된다.
func (e Environment) HostAPIPath(rel string) string {
	if e.HostAPIVersion == "" {
		return rel
	}
	return "/" + e.HostAPIVersion + rel
}

// HasCapability 는 해당 capability 가 실제로 사용 가능한지 판정한다.
func (e Environment) HasCapability(c extv1.Capability) bool {
	for _, x := range e.Capabilities {
		if x == c {
			return true
		}
	}
	return false
}

// LoadEnvironment 는 프로세스 환경변수에서 기동 환경을 읽어 검증한다.
//
// manifest 는 교차 검증에 쓴다: 환경변수의 확장 ID 가 Manifest 와 다르면 **잘못된 바이너리를
// 실행한 것**이므로 즉시 실패한다(그대로 두면 다른 확장의 권한으로 동작하게 된다).
func LoadEnvironment(manifest extv1.Manifest) (Environment, error) {
	return loadEnvironment(os.LookupEnv, manifest)
}

// loadEnvironment 는 조회 함수를 주입받는 내부 구현이다(테스트에서 실제 환경을 건드리지 않는다).
func loadEnvironment(lookup func(string) (string, bool), manifest extv1.Manifest) (Environment, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	get := func(key string) string {
		v, _ := lookup(key)
		return strings.TrimSpace(v)
	}

	env := Environment{
		ExtensionID: get(EnvExtensionID),
		Version:     get(EnvExtensionVersion),
		CallToken:   get(EnvCallToken),
		HostURL:     get(EnvHostURL),
		HostToken:   get(EnvHostToken),
	}

	// 1) 필수 값 — 하나라도 없으면 "직접 실행"으로 보고 누락 목록을 함께 알려준다.
	var missing []string
	for _, req := range []struct {
		key   string
		value string
	}{
		{EnvExtensionID, env.ExtensionID},
		{EnvCallToken, env.CallToken},
		{EnvHostURL, env.HostURL},
		{EnvHostToken, env.HostToken},
	} {
		if req.value == "" {
			missing = append(missing, req.key)
		}
	}
	if len(missing) > 0 {
		return Environment{}, fmt.Errorf("%s(누락된 환경변수: %s)", DirectRunMessage, strings.Join(missing, ", "))
	}

	// 2) 확장 ID 교차 검증 — 다른 확장의 환경으로 이 바이너리가 뜨는 것을 막는다.
	if manifest.ID != "" && env.ExtensionID != manifest.ID {
		return Environment{}, fmt.Errorf("환경변수 %s(%q) 가 Manifest 의 id(%q) 와 다릅니다 — 잘못된 확장 바이너리를 실행했습니다",
			EnvExtensionID, env.ExtensionID, manifest.ID)
	}
	if env.Version == "" {
		env.Version = manifest.Version
	}

	// 3) Host API 베이스 URL
	normalized, err := normalizeHostURL(env.HostURL)
	if err != nil {
		return Environment{}, err
	}
	env.HostURL = normalized

	// 4) 프로토콜 버전 협상 — Core 가 요구하는 버전을 이 SDK 가 구현하는지 먼저 본다.
	rawProto, _ := lookup(EnvProtocolVersion)
	proto, ok := extv1.ParseProtocolVersion(strings.TrimSpace(rawProto))
	if !ok {
		return Environment{}, &protocolMismatchError{
			reason: fmt.Sprintf("환경변수 %s 값을 해석할 수 없습니다: %q", EnvProtocolVersion, strings.TrimSpace(rawProto)),
		}
	}
	if !extv1.SupportsProtocolVersion(proto) {
		return Environment{}, &protocolMismatchError{
			reason: fmt.Sprintf("Core 가 요구하는 프로세스 프로토콜 버전(%d)을 이 확장 기능의 SDK 가 구현하지 않습니다(구현 범위 %d~%d) — 확장 기능을 다시 빌드하세요",
				proto, extv1.MinProtocolVersion, extv1.MaxProtocolVersion),
		}
	}
	env.ProtocolVersion = proto
	env.HostAPIVersion = normalizeHostAPIVersion(get(EnvHostAPIVersion))

	// 5) capability 협상 — 세 가지를 **구분**한다(요구 §15).
	//
	//     ① Manifest 에 있는데 Core 가 안 준 것      → MissingCapabilities
	//     ② Core 는 줬는데 SDK 구현이 없는 것        → UnsupportedCapabilities
	//     ③ 이 SDK 가 이름조차 모르는 것             → UnknownCapabilities
	//
	// 셋 다 기동을 막지 않고 **사유를 남긴다**. 조용히 뭉뚱그리면 증상은 전부 "왜인지 기능
	// 하나가 안 된다"로 같아지고, 원인이 Manifest 인지 Core 인지 SDK 인지 구분할 단서가 없다.
	raw, declaredByEnv := lookup(EnvCapabilities)
	envCaps, unknown := parseCapabilities(raw)
	env.UnknownCapabilities = unknown

	granted := make(map[extv1.Capability]bool, len(envCaps))
	if !declaredByEnv {
		// 환경변수 자체가 없는 경우(구버전 Core)만 Manifest 선언을 그대로 허용분으로 본다.
		for _, c := range manifest.Capabilities {
			granted[c] = true
		}
	} else {
		for _, c := range envCaps {
			granted[c] = true
		}
	}
	for _, c := range manifest.Capabilities {
		if !granted[c] {
			env.MissingCapabilities = append(env.MissingCapabilities, c)
		}
	}
	for _, c := range envCaps {
		if !manifest.HasCapability(c) {
			// Core 가 더 준 경우 — Manifest 가 최소권한 선언이므로 쓰지 않는다.
			continue
		}
		if !processCapabilityClients[c] {
			env.UnsupportedCapabilities = append(env.UnsupportedCapabilities, c)
			continue
		}
		env.Capabilities = append(env.Capabilities, c)
	}
	if !declaredByEnv {
		for _, c := range manifest.Capabilities {
			if !processCapabilityClients[c] {
				env.UnsupportedCapabilities = append(env.UnsupportedCapabilities, c)
				continue
			}
			env.Capabilities = append(env.Capabilities, c)
		}
	}

	// 6) 설정 JSON
	cfg, err := parseConfigJSON(get(EnvConfig))
	if err != nil {
		return Environment{}, err
	}
	env.Config = cfg

	return env, nil
}

// normalizeHostURL 은 Host API 베이스 URL 을 검증·정규화한다(끝의 / 제거).
func normalizeHostURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("환경변수 %s 가 올바른 URL 이 아닙니다: %w", EnvHostURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("환경변수 %s 는 http/https 절대 URL 이어야 합니다: %q", EnvHostURL, raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("환경변수 %s 에 호스트가 없습니다: %q", EnvHostURL, raw)
	}
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimSuffix(u.String(), "/"), nil
}

// parseCapabilities 는 쉼표 구분 capability 목록을 파싱한다.
//
// 이 SDK 가 모르는 이름은 오류로 만들지 않고 unknown 으로 돌려준다 — 새 capability 를 추가한
// Core 가 구버전 SDK 로 만든 확장을 기동할 때, 모르는 이름 하나 때문에 확장 전체가 죽으면 안 된다.
func parseCapabilities(raw string) (caps []extv1.Capability, unknown []string) {
	seen := make(map[extv1.Capability]bool)
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		c := extv1.Capability(name)
		if !extv1.KnownCapability(c) {
			unknown = append(unknown, name)
			continue
		}
		if seen[c] {
			continue
		}
		seen[c] = true
		caps = append(caps, c)
	}
	return caps, unknown
}

// parseConfigJSON 은 ABLEOPS_EXT_CONFIG 를 파싱한다(빈 값은 설정 없음).
//
// 형식이 깨진 설정은 조용히 무시하지 않는다 — 무시하면 관리자가 저장한 설정이 반영되지 않는데도
// 확장은 "정상"으로 보이고, 원인을 추적할 단서가 남지 않는다.
func parseConfigJSON(raw string) (map[string]any, error) {
	if raw == "" {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("환경변수 %s 가 올바른 JSON 객체가 아닙니다: %w", EnvConfig, err)
	}
	return out, nil
}

// normalizeHostAPIVersion 은 Host API 버전 세그먼트를 정규화한다.
//
// 값 형식이 예상 밖이면(경로 구분자·공백 포함) **버전 없는 경로**로 떨어뜨린다 —
// 잘못된 세그먼트로 경로를 만들면 모든 Host API 호출이 404 가 되기 때문이다.
func normalizeHostAPIVersion(raw string) string {
	v := strings.TrimSpace(raw)
	v = strings.Trim(v, "/")
	if v == "" || strings.ContainsAny(v, "/?#") {
		return ""
	}
	return v
}

// protocolMismatchError 는 프로토콜 협상 실패다.
//
// 일반 기동 오류와 구분하는 이유: Core 는 이 실패를 보면 **재시작하지 않고** 해당 확장만
// INCOMPATIBLE 로 격리해야 한다. 몇 번을 다시 띄워도 결과가 같기 때문이다(protocol.go 주석).
type protocolMismatchError struct{ reason string }

func (e *protocolMismatchError) Error() string { return e.reason }
