package extensionv1

// Manifest 파싱·검증·마스킹 테스트.

import (
	"strings"
	"testing"
)

// validManifestYAML 은 설계 문서 §3.2 의 정본 예시다(이 SDK 가 반드시 통과시켜야 하는 형태).
const validManifestYAML = `
apiVersion: ableops.io/extension/v1
id: sample
name: 샘플 확장 기능
version: 1.0.0
description: Extension 계약 검증용 참조 확장
publisher: AbleOps
requires:
  core: ">=1.3.0 <2.0.0"
capabilities: [kafka.read, cluster.read, audit.write, config.read]
permissions:
  - key: ext.sample.view
    label: 샘플 확장 조회
    roles: [SystemAdmin, KafkaOperator, ServiceOwner, Auditor, SecurityOperator, Requester]
  - key: ext.sample.manage
    label: 샘플 확장 관리
    roles: [SystemAdmin]
backend: { enabled: true, kind: builtin }
frontend: { enabled: true, entry: "" }
routes:  [{ path: /api/extensions/sample, permission: ext.sample.view }]
menus:   [{ key: ext-sample, parent: ops-env, path: /extensions/sample,
            label: 샘플 확장, icon: appstore, permission: ext.sample.view }]
config:  { schema: [{ key: greeting, type: string, default: "안녕하세요", label: 인사말 }] }
`

func mustParse(t *testing.T, y string) Manifest {
	t.Helper()
	m, err := ParseManifest([]byte(y))
	if err != nil {
		t.Fatalf("ParseManifest 실패: %v", err)
	}
	return m
}

func TestParseManifest_정상(t *testing.T) {
	m := mustParse(t, validManifestYAML)

	if m.APIVersion != APIVersion {
		t.Errorf("apiVersion = %q, 기대 %q", m.APIVersion, APIVersion)
	}
	if m.ID != "sample" || m.Name != "샘플 확장 기능" || m.Version != "1.0.0" {
		t.Errorf("기본 필드 파싱 오류: %+v", m)
	}
	if m.Requires.Core != ">=1.3.0 <2.0.0" {
		t.Errorf("requires.core = %q", m.Requires.Core)
	}
	if len(m.Capabilities) != 4 || !m.HasCapability(CapKafkaRead) || m.HasCapability(CapSecretRef) {
		t.Errorf("capabilities 파싱 오류: %v", m.Capabilities)
	}
	if len(m.Permissions) != 2 || m.Permissions[0].Key != "ext.sample.view" || len(m.Permissions[0].Roles) != 6 {
		t.Errorf("permissions 파싱 오류: %+v", m.Permissions)
	}
	if !m.Backend.Enabled || m.Backend.Kind != BackendKindBuiltin {
		t.Errorf("backend 파싱 오류: %+v", m.Backend)
	}
	if !m.Frontend.Enabled || m.Frontend.Entry != "" {
		t.Errorf("frontend 파싱 오류: %+v", m.Frontend)
	}
	if len(m.Routes) != 1 || m.Routes[0].Path != "/api/extensions/sample" {
		t.Errorf("routes 파싱 오류: %+v", m.Routes)
	}
	if len(m.Menus) != 1 || m.Menus[0].Icon != "appstore" || m.Menus[0].Parent != "ops-env" {
		t.Errorf("menus 파싱 오류: %+v", m.Menus)
	}
	if len(m.Config.Schema) != 1 || m.Config.Schema[0].Key != "greeting" {
		t.Errorf("config 파싱 오류: %+v", m.Config)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("정상 Manifest 가 검증에 실패했다: %v", err)
	}
}

func TestParseManifest_빈입력과_깨진YAML(t *testing.T) {
	if _, err := ParseManifest(nil); err == nil {
		t.Error("빈 Manifest 를 통과시켰다")
	}
	if _, err := ParseManifest([]byte("id: [깨진")); err == nil {
		t.Error("깨진 YAML 을 통과시켰다")
	}
}

func TestManifestValidate_실패사례(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Manifest)
		want   string // 오류 메시지에 반드시 포함되어야 하는 조각
	}{
		{"apiVersion 불일치", func(m *Manifest) { m.APIVersion = "ableops.io/extension/v2" }, "apiVersion"},
		{"apiVersion 누락", func(m *Manifest) { m.APIVersion = "" }, "apiVersion"},
		{"id 대문자", func(m *Manifest) { m.ID = "Sample" }, "id 형식"},
		{"id 하이픈 종료", func(m *Manifest) { m.ID = "sample-" }, "id 형식"},
		{"id 너무 짧음", func(m *Manifest) { m.ID = "ab" }, "id 형식"},
		{"name 누락", func(m *Manifest) { m.Name = "" }, "name 이 비어"},
		{"name 초과", func(m *Manifest) { m.Name = strings.Repeat("가", maxNameLen+1) }, "name 이 너무"},
		{"version 비semver", func(m *Manifest) { m.Version = "1.0" }, "semver"},
		{"version dev", func(m *Manifest) { m.Version = "dev" }, "semver"},
		{"미지의 capability", func(m *Manifest) { m.Capabilities = append(m.Capabilities, "kafka.write") }, "capability"},
		{"권한 네임스페이스 위반(Core 권한)", func(m *Manifest) { m.Permissions[0].Key = "topic.create" }, "네임스페이스"},
		{"권한 네임스페이스 위반(타 Extension)", func(m *Manifest) { m.Permissions[0].Key = "ext.other.view" }, "네임스페이스"},
		{"권한 키 중복", func(m *Manifest) { m.Permissions[1].Key = m.Permissions[0].Key }, "중복"},
		{"backend.kind 오류", func(m *Manifest) { m.Backend.Kind = "docker" }, "backend.kind"},
		// ⚠ 알 수 없는 되돌리기 정책은 **거부**한다. none 으로 조용히 떨어뜨리면 오타 하나로
		// "되돌릴 수 있다고 믿었는데 안 되는" 상태가 만들어진다.
		{"migrations.rollback 오타", func(m *Manifest) { m.Backend.Migrations.Rollback = "downs" }, "backend.migrations.rollback"},
		{"migrations.rollback 대문자", func(m *Manifest) { m.Backend.Migrations.Rollback = "DOWN" }, "backend.migrations.rollback"},
		{"migrations.rollback 임의값", func(m *Manifest) { m.Backend.Migrations.Rollback = "auto" }, "backend.migrations.rollback"},
		{"route 접두 위반", func(m *Manifest) { m.Routes[0].Path = "/api/topics" }, "routes[].path"},
		{"route 접두 유사경로", func(m *Manifest) { m.Routes[0].Path = "/api/extensions/sample-other/x" }, "routes[].path"},
		{"route 권한 도용", func(m *Manifest) { m.Routes[0].Permission = "ext.other.view" }, "routes[].permission"},
		{"menu 접두 위반", func(m *Manifest) { m.Menus[0].Path = "/topics" }, "menus[].path"},
		{"menu icon 누락", func(m *Manifest) { m.Menus[0].Icon = "" }, "icon"},
		{"menu icon 대문자", func(m *Manifest) { m.Menus[0].Icon = "AppStore" }, "icon"},
		{"menu label 누락", func(m *Manifest) { m.Menus[0].Label = "" }, "label"},
		{"config key 중복", func(m *Manifest) {
			m.Config.Schema = append(m.Config.Schema, ConfigField{Key: "greeting", Type: "string"})
		}, "config.schema[].key"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := mustParse(t, validManifestYAML)
			tc.mutate(&m)
			err := m.Validate()
			if err == nil {
				t.Fatalf("검증이 통과했다(거부해야 함)")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("오류 메시지에 %q 가 없다: %v", tc.want, err)
			}
		})
	}
}

// backend.migrations.rollback — 선언이 없으면 none(= v1.4.0 동작 그대로)이다.
func TestManifest_마이그레이션_되돌리기_선언(t *testing.T) {
	// ① 선언 없음: 기존 패키지가 그대로 통과하고 none 으로 읽힌다(하위호환).
	m := mustParse(t, validManifestYAML)
	if err := m.Validate(); err != nil {
		t.Fatalf("선언 없는 Manifest 가 거부되었다: %v", err)
	}
	if m.MigrationRollbackMode() != MigrationRollbackNone || m.SupportsMigrationRollback() {
		t.Fatalf("선언 없음 = none 이어야 한다: %q", m.MigrationRollbackMode())
	}

	// ② down 선언: 통과하고 지원으로 읽힌다.
	withDown := strings.Replace(validManifestYAML,
		"backend: { enabled: true, kind: builtin }",
		"backend:\n  enabled: true\n  kind: builtin\n  migrations:\n    rollback: down", 1)
	d := mustParse(t, withDown)
	if err := d.Validate(); err != nil {
		t.Fatalf("down 선언이 거부되었다: %v", err)
	}
	if d.Backend.Migrations.Rollback != MigrationRollbackDown || !d.SupportsMigrationRollback() {
		t.Fatalf("down 선언이 반영되지 않았다: %+v", d.Backend)
	}

	// ③ none 명시: down 과 구분되어야 한다.
	n := mustParse(t, strings.Replace(withDown, "rollback: down", "rollback: none", 1))
	if err := n.Validate(); err != nil {
		t.Fatalf("none 선언이 거부되었다: %v", err)
	}
	if n.SupportsMigrationRollback() {
		t.Fatal("none 인데 지원으로 읽혔다")
	}

	// ④ 공백은 제거하되 값은 접지 않는다(정규화가 오타를 숨기면 안 된다).
	sp := mustParse(t, strings.Replace(withDown, "rollback: down", `rollback: "  down  "`, 1))
	if sp.Backend.Migrations.Rollback != MigrationRollbackDown {
		t.Fatalf("공백 정규화 실패: %q", sp.Backend.Migrations.Rollback)
	}

	// ⑤ Sanitized 사본도 선언을 유지해야 한다(관리 화면이 이 값으로 체크박스를 켠다).
	if !d.Sanitized().SupportsMigrationRollback() {
		t.Fatal("Sanitized 사본에서 되돌리기 선언이 사라졌다")
	}
}

func TestManifestValidate_3단메뉴_거부(t *testing.T) {
	m := mustParse(t, validManifestYAML)
	// ext-sample(2단: ops-env > ext-sample) 아래에 손자 항목을 추가하면 3단이 된다.
	m.Menus = append(m.Menus, MenuDecl{
		Key: "ext-sample-child", Parent: "ext-sample",
		Path: "/extensions/sample/child", Label: "손자 메뉴", Icon: "appstore",
	})
	err := m.Validate()
	if err == nil {
		t.Fatal("3단 메뉴를 통과시켰다")
	}
	if !strings.Contains(err.Error(), "3단") {
		t.Errorf("3단 거부 메시지가 아니다: %v", err)
	}
}

func TestManifestValidate_2단메뉴_허용(t *testing.T) {
	m := mustParse(t, validManifestYAML)
	// 최상위 그룹(parent 없음) + 그 아래 항목 = 2단
	m.Menus = []MenuDecl{
		{Key: "ext-sample-group", Path: "/extensions/sample", Label: "샘플 그룹", Icon: "appstore"},
		{Key: "ext-sample-item", Parent: "ext-sample-group", Path: "/extensions/sample/list", Label: "목록", Icon: "table"},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("2단 메뉴가 거부되었다: %v", err)
	}
}

func TestManifestValidate_메뉴_순환참조(t *testing.T) {
	m := mustParse(t, validManifestYAML)
	m.Menus = []MenuDecl{
		{Key: "a", Parent: "b", Path: "/extensions/sample/a", Label: "A", Icon: "appstore"},
		{Key: "b", Parent: "a", Path: "/extensions/sample/b", Label: "B", Icon: "appstore"},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("순환 참조 메뉴를 통과시켰다")
	}
}

func TestManifestValidate_오류_누적(t *testing.T) {
	m := Manifest{} // 전부 비어 있음
	err := m.Validate()
	if err == nil {
		t.Fatal("빈 Manifest 를 통과시켰다")
	}
	for _, want := range []string{"apiVersion", "id 형식", "name 이 비어", "semver"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("오류가 누적되지 않았다 — %q 없음: %v", want, err)
		}
	}
}

func TestManifest_공백_정규화(t *testing.T) {
	m := mustParse(t, "apiVersion: \"  ableops.io/extension/v1  \"\nid: \"  sample  \"\nname: \"  이름  \"\nversion: \" 1.0.0 \"\n")
	if m.ID != "sample" || m.APIVersion != APIVersion || m.Version != "1.0.0" || m.Name != "이름" {
		t.Errorf("공백이 정규화되지 않았다: %+v", m)
	}
}

func TestManifestSanitized_시크릿_마스킹(t *testing.T) {
	m := mustParse(t, validManifestYAML)
	m.Config.Schema = []ConfigField{
		{Key: "greeting", Type: "string", Default: "안녕하세요"},
		{Key: "adminPassword", Type: "string", Default: "P@ssw0rd"},
		{Key: "apiToken", Type: "string", Default: "tok-123"},
		{Key: "clientSecret", Type: "string", Default: "sec-123"},
		{Key: "signingKey", Type: "string", Default: "key-123"},
		{Key: "opaque", Type: "string", Default: "값", Secret: true},
	}

	s := m.Sanitized()

	if got := s.Config.Schema[0].Default; got != "안녕하세요" {
		t.Errorf("일반 값이 마스킹되었다: %v", got)
	}
	for _, i := range []int{1, 2, 3, 4, 5} {
		if got := s.Config.Schema[i].Default; got != maskedValue {
			t.Errorf("schema[%d](%s) 가 마스킹되지 않았다: %v", i, s.Config.Schema[i].Key, got)
		}
	}
	// 원본 불변
	if m.Config.Schema[1].Default != "P@ssw0rd" {
		t.Errorf("Sanitized 가 원본을 변경했다: %v", m.Config.Schema[1].Default)
	}
	// 슬라이스 공유 금지(사본 수정이 원본에 영향 없어야 한다)
	s.Permissions[0].Roles[0] = "변경됨"
	if m.Permissions[0].Roles[0] == "변경됨" {
		t.Error("Sanitized 가 Roles 슬라이스를 공유한다")
	}
	// 나머지 필드는 그대로 유지
	if s.ID != m.ID || s.Name != m.Name || len(s.Routes) != len(m.Routes) {
		t.Error("Sanitized 가 비시크릿 필드를 훼손했다")
	}
}
