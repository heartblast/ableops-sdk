package extensionv1

// Extension 설정 — 스키마 선언(Manifest)과 런타임 접근(ConfigService).
//
// Extension 설정은 Core 실행 설정(internal/config.Config)에 추가하지 않는다.
// Core 는 `extensionID + configJSON` 만 보관하고, 스키마는 Manifest 가 선언한다.

import "context"

// 설정 필드 타입 식별자(권장 값). Core 는 이 값으로 관리 화면 입력 위젯을 고른다.
const (
	ConfigTypeString = "string"
	ConfigTypeInt    = "int"
	ConfigTypeBool   = "bool"
	ConfigTypeSelect = "select"
)

// ConfigField 는 Extension 설정 항목 1개의 스키마다.
//
// Secret 이 true 면 값은 화면·로그·감사·API 응답에서 마스킹된다(Manifest.Sanitized 참조).
// Default 에는 **평문 시크릿을 넣지 않는다** — Manifest 는 패키지에 그대로 담겨 배포되는 파일이다.
type ConfigField struct {
	Key      string `json:"key" yaml:"key"`
	Type     string `json:"type,omitempty" yaml:"type,omitempty"` // string | int | bool | select
	Default  any    `json:"default,omitempty" yaml:"default,omitempty"`
	Label    string `json:"label,omitempty" yaml:"label,omitempty"`
	Required bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Secret   bool   `json:"secret,omitempty" yaml:"secret,omitempty"`
}

// ConfigDecl 은 Manifest 의 config 절이다.
type ConfigDecl struct {
	Schema []ConfigField `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// ConfigReadService 는 Extension 설정 **조회** 계약이다(capability: config.read).
type ConfigReadService interface {
	Get(ctx context.Context) (map[string]any, error)
}

// ConfigWriteService 는 Extension 설정 **저장** 계약이다(capability: config.write).
//
// 조회와 분리한 이유는 capability 이름이 실제 권한과 일치해야 하기 때문이다(capability.go 주석).
// Resolver 로 얻는다:
//
//	w, err := extensionv1.RequireService[extensionv1.ConfigWriteService](host, extensionv1.CapConfigWrite)
type ConfigWriteService interface {
	Set(ctx context.Context, values map[string]any) error
}

// ConfigService 는 Extension 설정 접근 인터페이스다(HostContext.Config).
//
// **자기 Extension 설정만** 접근할 수 있다. 대상 Extension ID 를 인자로 받지 않는 이유가 그것이다 —
// Core 가 HostContext.ExtensionID 로 스코프를 고정해 주입하므로, 다른 Extension 의 설정이나
// Core 설정을 읽을 방법이 인터페이스 상 존재하지 않는다.
//
// ⚠ **Get 은 config.read, Set 은 config.write 다.**
// 이 인터페이스는 v1 하위호환을 위해 두 메서드를 그대로 들고 있지만, config.write 를 선언하지
// 않은 확장에서 Set 을 부르면 Core 가 PERMISSION_DENIED 오류를 돌려준다(타입은 그대로 두고
// 런타임에서 막는다 — 인터페이스에서 메서드를 빼면 기존 확장이 컴파일되지 않는다).
// 새로 작성하는 코드는 위 두 인터페이스를 Resolver 로 얻는 쪽을 권장한다.
//
// Set 은 Manifest 가 선언한 스키마 범위 밖의 키를 거부할 수 있다(Core 구현 정책).
type ConfigService interface {
	ConfigReadService
	ConfigWriteService
}
