package extensionv1

// Extension Manifest — 확장 기능 패키지의 선언 파일(extension.yaml) 스키마·파싱·검증.
//
// Manifest 는 Extension 이 **무엇을 요구하고 무엇을 기여하는지**를 데이터로만 선언한다.
// 코드·스크립트·시크릿을 담지 않으며, Core 는 Manifest 를 실행하지 않고 해석만 한다.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// maxNameLen 은 Manifest name 의 최대 길이(문자 수)다.
	maxNameLen = 100
	// maxMenuDepth 는 허용하는 메뉴 단계 수다(그룹 > 항목 = 2단).
	maxMenuDepth = 2
	// routePrefixBase 는 Extension 백엔드 라우트의 고정 접두다.
	routePrefixBase = "/api/extensions/"
	// menuPrefixBase 는 Extension 프론트 경로의 고정 접두다.
	menuPrefixBase = "/extensions/"
)

// Backend kind 값. process 는 외부 프로세스 Host(후속 Phase)를 위한 계약만 수용한 값이다.
const (
	BackendKindBuiltin = "builtin"
	BackendKindProcess = "process"
)

// 마이그레이션 되돌리기 정책 값(backend.migrations.rollback).
//
// ⚠ **기본값은 none 이고, none 은 "현재 동작 그대로"** 를 뜻한다. 선언이 없는 기존 패키지는
// 롤백 시 코드만 되돌아가고 DB 스키마는 그대로 남는다(v1.4.0 동작).
const (
	// MigrationRollbackNone 은 down 스크립트를 실행하지 않는다는 선언이다(기본값).
	MigrationRollbackNone = "none"
	// MigrationRollbackDown 은 `*.down.sql` 로 되돌릴 수 있다는 **선언**이다.
	//
	// ⚠ 선언했다고 자동으로 실행되지 않는다. 실행은 관리자가 롤백 요청에 명시적으로
	// revertMigrations=true 를 담았을 때뿐이며, 업데이트 실패 시 자동 복구 경로에서는
	// 어떤 경우에도 실행하지 않는다(자동 경로에서 데이터를 지우면 관리자가 의도를 표명할 기회가 없다).
	MigrationRollbackDown = "down"
)

// extensionIDRe 는 Extension ID 형식이다(소문자 시작, 소문자/숫자/하이픈, 소문자·숫자로 종료, 3~32자).
var extensionIDRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`)

// menuIconRe 는 메뉴 아이콘 ID 형식이다(소문자 식별자 — 프론트 whitelist 조회 키).
var menuIconRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ValidExtensionID 는 Extension ID 형식을 판정한다.
func ValidExtensionID(id string) bool {
	return extensionIDRe.MatchString(strings.TrimSpace(id))
}

// Requires 는 Extension 이 요구하는 실행 환경 제약이다.
type Requires struct {
	// Core 는 Core 버전 제약이다(예: ">=1.3.0 <2.0.0"). **비어 있으면 제한 없음**.
	// "dev"/"unknown" 같은 개발 빌드 버전의 처리는 Core 정책이며 이 SDK 가 결정하지 않는다(version.go 주석 참조).
	Core string `json:"core,omitempty" yaml:"core,omitempty"`
}

// MigrationsDecl 은 패키지 DB 마이그레이션의 되돌리기 계약이다(backend.migrations).
//
// # 왜 선언이 필요한가
//
// 계약 없이 down 스크립트를 실행하면 롤백이 **데이터 손실 경로**가 된다. 무엇을 어떻게 되돌려야
// 하는지는 패키지만 알고, Core 가 "파일이 있으니 실행한다"고 추측하면 새 버전이 이미 쓴 운영
// 데이터를 조용히 지운다. 그래서 패키지가 "이 스크립트로 되돌려도 된다"고 **먼저 선언**해야 한다.
type MigrationsDecl struct {
	// Rollback 은 되돌리기 정책이다: none(기본) | down.
	//
	// ⚠ 알 수 없는 값은 Manifest 검증에서 **거부**한다. 조용히 none 으로 떨어뜨리면
	// 오타(`Down`·`downs`) 하나로 "되돌릴 수 있다고 믿었는데 안 되는" 상태가 만들어진다.
	Rollback string `json:"rollback,omitempty" yaml:"rollback,omitempty"`
}

// BackendDecl 은 Extension 백엔드 실행 방식 선언이다.
type BackendDecl struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Kind    string `json:"kind,omitempty" yaml:"kind,omitempty"` // builtin | process(후속). 비우면 builtin 취급.
	// Migrations 는 패키지 DB 마이그레이션 되돌리기 계약이다(비우면 rollback: none).
	//
	// ⚠ JSON 태그가 **omitzero** 인 이유: encoding/json 의 `omitempty` 는 구조체 필드에 적용되지
	// 않는다. `omitempty` 로 두면 선언하지 않은 모든 확장의 응답에 `"migrations":{}` 가 새로
	// 실려(v1.4.0 응답과 다른 모양) 응답을 그대로 비교·스냅샷하는 소비자가 차이를 본다.
	// yaml.v3 는 `omitempty` 로 구조체 zero 값을 생략하므로 태그가 서로 다르다.
	Migrations MigrationsDecl `json:"migrations,omitzero" yaml:"migrations,omitempty"`
}

// MigrationRollbackMode 는 선언된 되돌리기 정책을 반환한다(비어 있으면 none).
//
// 호출부가 빈 문자열과 "none" 을 각자 판정하면 한쪽만 고쳐도 컴파일이 통과하므로 여기로 모은다.
func (m Manifest) MigrationRollbackMode() string {
	if v := strings.TrimSpace(m.Backend.Migrations.Rollback); v != "" {
		return v
	}
	return MigrationRollbackNone
}

// SupportsMigrationRollback 은 down 스크립트로 되돌릴 수 있다고 **선언**했는지다.
// (실제 실행 가능 여부 — 짝 없는 down·검증 위반 — 는 Core 가 별도로 판정한다.)
func (m Manifest) SupportsMigrationRollback() bool {
	return m.MigrationRollbackMode() == MigrationRollbackDown
}

// 프론트엔드 실행 모드(frontend.kind) — **신뢰 모델의 선언**이다.
//
// 현재 원격 번들은 Core 와 같은 브라우저 컨텍스트에서 실행되는 ES 모듈이며 보안 샌드박스가
// **아니다**(같은 페이지의 JavaScript 는 localStorage 의 로그인 토큰에 접근할 수 있다).
// 그래서 지금 허용하는 값은 trusted-module 하나뿐이고, 미신뢰 제3자 UI 는 지원하지 않는다.
//
// 이름을 지금 정의해 두는 이유는 **나중에 실행 모드를 추가할 수 있게** 하기 위해서다.
// 필드가 없으면 미래의 격리 실행 모드를 도입할 때 Manifest 스키마 자체를 바꿔야 하고,
// 그것은 이미 배포된 패키지 전부에 영향을 준다.
const (
	// FrontendKindTrustedModule 은 현재 유일한 모드다(Core 컨텍스트에서 ES 모듈로 실행).
	FrontendKindTrustedModule = "trusted-module"
	// FrontendKindIsolatedFrame 은 **후속 과제**다 — iframe 으로 격리 실행한다.
	//
	// ⚠ 상수만 정의하고 Core 는 아직 실행하지 않는다. 선언하면 설치가 거부된다
	// (조용히 trusted 로 실행하면 "격리했다고 믿는데 아닌" 최악의 상태가 된다).
	FrontendKindIsolatedFrame = "isolated-frame"
)

// FrontendDecl 은 Extension 프론트엔드 기여 방식 선언이다.
type FrontendDecl struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	// Entry 가 비어 있으면 Core 프론트의 built-in 레지스트리(id → lazy 페이지)를 사용한다.
	Entry string `json:"entry,omitempty" yaml:"entry,omitempty"`
	// Kind 는 실행 모드다(비우면 trusted-module).
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`
}

// FrontendMode 는 선언된 프론트 실행 모드를 반환한다(비어 있으면 trusted-module).
func (m Manifest) FrontendMode() string {
	if v := strings.TrimSpace(m.Frontend.Kind); v != "" {
		return v
	}
	return FrontendKindTrustedModule
}

// Manifest 는 extension.yaml 과 1:1 대응하는 Extension 선언이다.
//
// **이 구조체에는 어떤 시크릿 필드도 두지 않는다.** Manifest 는 패키지 파일로 배포되고
// 관리 화면·감사·로그에 노출되는 데이터이므로 비밀번호·토큰·키를 담을 자리가 존재해서는 안 된다.
// (그럼에도 config 기본값에 실수로 들어올 수 있으므로 Sanitized 가 방어적으로 마스킹한다.)
type Manifest struct {
	APIVersion   string           `json:"apiVersion" yaml:"apiVersion"`
	ID           string           `json:"id" yaml:"id"`
	Name         string           `json:"name" yaml:"name"`
	Version      string           `json:"version" yaml:"version"`
	Description  string           `json:"description,omitempty" yaml:"description,omitempty"`
	Publisher    string           `json:"publisher,omitempty" yaml:"publisher,omitempty"`
	Requires     Requires         `json:"requires,omitempty" yaml:"requires,omitempty"`
	Capabilities []Capability     `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Permissions  []PermissionDecl `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Backend      BackendDecl      `json:"backend,omitempty" yaml:"backend,omitempty"`
	Frontend     FrontendDecl     `json:"frontend,omitempty" yaml:"frontend,omitempty"`
	Routes       []RouteDecl      `json:"routes,omitempty" yaml:"routes,omitempty"`
	Menus        []MenuDecl       `json:"menus,omitempty" yaml:"menus,omitempty"`
	Config       ConfigDecl       `json:"config,omitempty" yaml:"config,omitempty"`
}

// ParseManifest 는 extension.yaml 바이트열을 Manifest 로 파싱한다.
// 형식 검증은 하지 않는다 — 파싱 성공 후 반드시 Validate 를 호출한다.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if len(strings.TrimSpace(string(data))) == 0 {
		return Manifest{}, errors.New("Manifest 가 비어 있습니다(extension.yaml 내용 없음)")
	}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("Manifest YAML 파싱 실패: %w", err)
	}
	m.normalize()
	return m, nil
}

// normalize 는 스칼라 문자열의 앞뒤 공백을 제거한다.
// 공백이 섞인 ID·경로가 그대로 Core 로 넘어가면 라우트 마운트/권한 키가 조용히 어긋난다.
func (m *Manifest) normalize() {
	m.APIVersion = strings.TrimSpace(m.APIVersion)
	m.ID = strings.TrimSpace(m.ID)
	m.Name = strings.TrimSpace(m.Name)
	m.Version = strings.TrimSpace(m.Version)
	m.Description = strings.TrimSpace(m.Description)
	m.Publisher = strings.TrimSpace(m.Publisher)
	m.Requires.Core = strings.TrimSpace(m.Requires.Core)
	m.Backend.Kind = strings.TrimSpace(m.Backend.Kind)
	// ⚠ 공백만 제거하고 대소문자는 바꾸지 않는다 — 소문자로 접어 주면 "DOWN" 같은 오타가 조용히
	// 통과해 계약 값이 두 가지가 된다(backend.kind 와 같은 규칙).
	m.Backend.Migrations.Rollback = strings.TrimSpace(m.Backend.Migrations.Rollback)
	m.Frontend.Entry = strings.TrimSpace(m.Frontend.Entry)
	m.Frontend.Kind = strings.TrimSpace(m.Frontend.Kind)

	for i, c := range m.Capabilities {
		m.Capabilities[i] = Capability(strings.TrimSpace(string(c)))
	}
	for i := range m.Permissions {
		m.Permissions[i].Key = strings.TrimSpace(m.Permissions[i].Key)
		m.Permissions[i].Label = strings.TrimSpace(m.Permissions[i].Label)
		for j, r := range m.Permissions[i].Roles {
			m.Permissions[i].Roles[j] = strings.TrimSpace(r)
		}
	}
	for i := range m.Routes {
		m.Routes[i].Path = strings.TrimSpace(m.Routes[i].Path)
		m.Routes[i].Permission = strings.TrimSpace(m.Routes[i].Permission)
	}
	for i := range m.Menus {
		m.Menus[i].Key = strings.TrimSpace(m.Menus[i].Key)
		m.Menus[i].Parent = strings.TrimSpace(m.Menus[i].Parent)
		m.Menus[i].Path = strings.TrimSpace(m.Menus[i].Path)
		m.Menus[i].Label = strings.TrimSpace(m.Menus[i].Label)
		m.Menus[i].Icon = strings.TrimSpace(m.Menus[i].Icon)
		m.Menus[i].Permission = strings.TrimSpace(m.Menus[i].Permission)
	}
	for i := range m.Config.Schema {
		m.Config.Schema[i].Key = strings.TrimSpace(m.Config.Schema[i].Key)
		m.Config.Schema[i].Type = strings.TrimSpace(m.Config.Schema[i].Type)
		m.Config.Schema[i].Label = strings.TrimSpace(m.Config.Schema[i].Label)
	}
}

// Validate 는 Manifest 를 검증하고 발견한 문제를 **모두** 한국어 오류로 묶어 돌려준다.
// 문제가 없으면 nil 을 돌려준다.
func (m Manifest) Validate() error {
	var errs []error

	id := strings.TrimSpace(m.ID)

	// 1) API 버전
	if !SupportsAPIVersion(m.APIVersion) {
		errs = append(errs, fmt.Errorf("apiVersion 이 지원 범위 밖입니다: %q (필요: %q)", m.APIVersion, APIVersion))
	}

	// 2) Extension ID
	if !ValidExtensionID(id) {
		errs = append(errs, fmt.Errorf("id 형식이 올바르지 않습니다: %q (소문자로 시작, 소문자·숫자·하이픈만, 소문자·숫자로 종료, 3~32자)", m.ID))
	}

	// 3) 이름
	name := strings.TrimSpace(m.Name)
	switch {
	case name == "":
		errs = append(errs, errors.New("name 이 비어 있습니다"))
	case len([]rune(name)) > maxNameLen:
		errs = append(errs, fmt.Errorf("name 이 너무 깁니다: %d자 (최대 %d자)", len([]rune(name)), maxNameLen))
	}

	// 4) 버전(semver)
	if _, _, _, err := ParseSemver(m.Version); err != nil {
		errs = append(errs, fmt.Errorf("version 이 semver 형식이 아닙니다: %q (%v)", m.Version, err))
	}

	// 5) capability
	for _, c := range m.Capabilities {
		if !KnownCapability(c) {
			errs = append(errs, fmt.Errorf("알 수 없는 capability 입니다: %q", string(c)))
		}
	}

	// 6) 권한 네임스페이스 — ext.<id>.<action> 만 허용
	seenPerm := make(map[string]bool, len(m.Permissions))
	for _, p := range m.Permissions {
		if !ValidPermissionKey(id, p.Key) {
			errs = append(errs, fmt.Errorf("permissions[].key 가 권한 네임스페이스를 벗어났습니다: %q (필요 형식: %s)", p.Key, PermissionKey(id, "<action>")))
			continue
		}
		if seenPerm[p.Key] {
			errs = append(errs, fmt.Errorf("permissions[].key 가 중복되었습니다: %q", p.Key))
		}
		seenPerm[p.Key] = true
	}

	// 7) backend.kind
	if k := m.Backend.Kind; k != "" && k != BackendKindBuiltin && k != BackendKindProcess {
		errs = append(errs, fmt.Errorf("backend.kind 가 올바르지 않습니다: %q (%s | %s)", k, BackendKindBuiltin, BackendKindProcess))
	}

	// 7-1) backend.migrations.rollback — 알 수 없는 값은 거부한다(none 으로 조용히 떨어뜨리지 않는다).
	if v := m.Backend.Migrations.Rollback; v != "" && v != MigrationRollbackNone && v != MigrationRollbackDown {
		errs = append(errs, fmt.Errorf("backend.migrations.rollback 이 올바르지 않습니다: %q (%s | %s)",
			v, MigrationRollbackNone, MigrationRollbackDown))
	}

	// 7-2) frontend.kind — 알 수 없는 값과 아직 구현하지 않은 모드를 **거부**한다.
	//
	// isolated-frame 을 조용히 trusted-module 로 떨어뜨리면 배포자는 격리되었다고 믿는데
	// 실제로는 Core 와 같은 컨텍스트에서 도는 최악의 상태가 된다.
	switch k := m.Frontend.Kind; k {
	case "", FrontendKindTrustedModule:
	case FrontendKindIsolatedFrame:
		errs = append(errs, fmt.Errorf("frontend.kind %q 는 아직 지원하지 않습니다(현재 지원: %s)", k, FrontendKindTrustedModule))
	default:
		errs = append(errs, fmt.Errorf("frontend.kind 가 올바르지 않습니다: %q (%s)", k, FrontendKindTrustedModule))
	}

	// 8) 라우트 경로 접두
	routePrefix := routePrefixBase + id
	for _, r := range m.Routes {
		if !hasPathPrefix(r.Path, routePrefix) {
			errs = append(errs, fmt.Errorf("routes[].path 는 %q 접두여야 합니다: %q", routePrefix, r.Path))
		}
		if err := checkExtPermissionRef("routes[].permission", id, r.Permission); err != nil {
			errs = append(errs, err)
		}
	}

	// 9) 메뉴 경로 접두·아이콘·키
	menuPrefix := menuPrefixBase + id
	byKey := make(map[string]MenuDecl, len(m.Menus))
	for _, mi := range m.Menus {
		if mi.Key == "" {
			errs = append(errs, errors.New("menus[].key 가 비어 있습니다"))
			continue
		}
		if _, dup := byKey[mi.Key]; dup {
			errs = append(errs, fmt.Errorf("menus[].key 가 중복되었습니다: %q", mi.Key))
			continue
		}
		byKey[mi.Key] = mi
	}
	for _, mi := range m.Menus {
		if mi.Label == "" {
			errs = append(errs, fmt.Errorf("menus[%q].label 이 비어 있습니다", mi.Key))
		}
		if !hasPathPrefix(mi.Path, menuPrefix) {
			errs = append(errs, fmt.Errorf("menus[].path 는 %q 접두여야 합니다: %q", menuPrefix, mi.Path))
		}
		if !menuIconRe.MatchString(mi.Icon) {
			errs = append(errs, fmt.Errorf("menus[%q].icon 은 비어 있지 않은 소문자 식별자여야 합니다: %q", mi.Key, mi.Icon))
		}
		if err := checkExtPermissionRef("menus[].permission", id, mi.Permission); err != nil {
			errs = append(errs, err)
		}
	}
	// 메뉴 단계 — 2단(그룹 > 항목)까지만. 3단은 프론트 권한 필터가 손자를 버리므로 여기서 거부한다.
	for _, mi := range m.Menus {
		depth, err := menuDepth(mi, byKey)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if depth > maxMenuDepth {
			errs = append(errs, fmt.Errorf("menus[%q] 가 %d단입니다 — 메뉴는 %d단(그룹 > 항목)까지만 허용합니다", mi.Key, depth, maxMenuDepth))
		}
	}

	// 10) config 스키마 키 중복
	seenCfg := make(map[string]bool, len(m.Config.Schema))
	for _, f := range m.Config.Schema {
		if f.Key == "" {
			errs = append(errs, errors.New("config.schema[].key 가 비어 있습니다"))
			continue
		}
		if seenCfg[f.Key] {
			errs = append(errs, fmt.Errorf("config.schema[].key 가 중복되었습니다: %q", f.Key))
		}
		seenCfg[f.Key] = true
	}

	return errors.Join(errs...)
}

// hasPathPrefix 는 경로 경계를 지키는 접두 검사다.
// 단순 HasPrefix 는 "/extensions/sample" 접두 검사에 "/extensions/sample-other" 를 통과시킨다.
func hasPathPrefix(path, prefix string) bool {
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}

// checkExtPermissionRef 는 route/menu 가 참조하는 권한 키를 검사한다.
// 비어 있으면(인증만 요구) 통과하고, Core 권한 키는 허용하되 `ext.` 로 시작하면
// 반드시 자기 Extension 네임스페이스여야 한다(다른 Extension 권한 도용 방지).
func checkExtPermissionRef(field, extID, key string) error {
	if key == "" || !strings.HasPrefix(key, "ext.") {
		return nil
	}
	if !ValidPermissionKey(extID, key) {
		return fmt.Errorf("%s 가 다른 Extension 의 권한을 참조합니다: %q (필요 형식: %s)", field, key, PermissionKey(extID, "<action>"))
	}
	return nil
}

// menuDepth 는 메뉴의 단계 수를 센다.
// parent 가 비면 1단(최상위 그룹), parent 가 Manifest 밖(Core 그룹 키)이면 2단으로 본다.
func menuDepth(mi MenuDecl, byKey map[string]MenuDecl) (int, error) {
	depth := 1
	seen := map[string]bool{mi.Key: true}
	cur := mi
	for cur.Parent != "" {
		depth++
		parent, ok := byKey[cur.Parent]
		if !ok {
			// Core 그룹 아래 항목 — 여기서 종료한다.
			return depth, nil
		}
		if seen[parent.Key] {
			return 0, fmt.Errorf("menus[%q] 의 parent 참조가 순환합니다", mi.Key)
		}
		seen[parent.Key] = true
		cur = parent
		if depth > maxMenuDepth+2 { // 순환이 아니어도 과도한 중첩은 여기서 끊는다
			return depth, nil
		}
	}
	return depth, nil
}

// maskedValue 는 마스킹된 값 표기다.
const maskedValue = "***"

// secretKeyHints 는 키 이름만으로 시크릿을 추정하는 힌트다(보수적으로 넓게 잡는다).
var secretKeyHints = []string{"password", "secret", "token", "key"}

// Sanitized 는 로그·API 응답·감사에 실어도 안전한 Manifest 사본을 돌려준다.
//
// 전제: **Manifest 구조체에는 시크릿 전용 필드가 없다**(위 Manifest 주석 참조).
// 그럼에도 작성자가 config 기본값에 자격증명을 적어 넣는 사고를 막기 위해,
// Secret=true 이거나 키 이름에 password/secret/token/key 가 포함된 항목의 Default 를 "***" 로 가린다.
// 원본은 변경하지 않는다(슬라이스는 새로 만들어 복사한다).
func (m Manifest) Sanitized() Manifest {
	out := m

	out.Capabilities = append([]Capability(nil), m.Capabilities...)
	out.Routes = append([]RouteDecl(nil), m.Routes...)
	out.Menus = append([]MenuDecl(nil), m.Menus...)

	out.Permissions = make([]PermissionDecl, len(m.Permissions))
	for i, p := range m.Permissions {
		p.Roles = append([]string(nil), p.Roles...)
		out.Permissions[i] = p
	}

	out.Config.Schema = make([]ConfigField, len(m.Config.Schema))
	for i, f := range m.Config.Schema {
		if f.Default != nil && isSecretKey(f.Key, f.Secret) {
			f.Default = maskedValue
		}
		out.Config.Schema[i] = f
	}
	if len(m.Config.Schema) == 0 {
		out.Config.Schema = nil
	}

	return out
}

// isSecretKey 는 설정 항목을 시크릿으로 볼지 판정한다.
func isSecretKey(key string, declared bool) bool {
	if declared {
		return true
	}
	lower := strings.ToLower(key)
	for _, hint := range secretKeyHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}
