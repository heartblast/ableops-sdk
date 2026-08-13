package extensionv1

// Extension 동적 권한 키 — `ext.<extensionID>.<action>` 네임스페이스.
//
// Core 정적 권한(topic.create 등)과 절대 섞이지 않도록 `ext.` 접두를 강제한다.
// Extension 이 Core 권한 키를 선언해 권한 매트릭스를 덮어쓰는 일을 구조적으로 막기 위함이다.

import (
	"regexp"
	"strings"
)

// permissionActionRe 는 권한 키의 action 부분 형식이다.
// 소문자·숫자·하이픈으로 시작하고, 하위 구분자로 점(.)을 허용한다(예: view, manage, job.run).
var permissionActionRe = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z0-9-]+)*$`)

// PermissionDecl 은 Manifest 가 선언하는 Extension 권한 1건이다.
// Roles 는 이 권한을 기본 부여할 포털 역할 목록이다
// (Requester / ServiceOwner / KafkaOperator / SecurityOperator / Auditor / SystemAdmin).
type PermissionDecl struct {
	Key   string   `json:"key" yaml:"key"`     // ext.<extensionID>.<action>
	Label string   `json:"label" yaml:"label"` // 화면 표시용 한국어 설명
	Roles []string `json:"roles,omitempty" yaml:"roles,omitempty"`
}

// PermissionKey 는 Extension 권한 키를 조립한다(형식 검증은 하지 않는다 — ValidPermissionKey 를 쓴다).
func PermissionKey(extID, action string) string {
	return "ext." + strings.TrimSpace(extID) + "." + strings.TrimSpace(action)
}

// ValidPermissionKey 는 key 가 해당 Extension 의 권한 네임스페이스에 속하는 올바른 키인지 판정한다.
func ValidPermissionKey(extID, key string) bool {
	id, action, ok := ParsePermissionKey(key)
	if !ok {
		return false
	}
	return id == strings.TrimSpace(extID) && action != ""
}

// ParsePermissionKey 는 `ext.<extensionID>.<action>` 을 분해한다.
// 형식이 어긋나면 ok=false 이며, extID 는 Extension ID 규칙(ValidExtensionID)도 함께 만족해야 한다.
func ParsePermissionKey(key string) (extID, action string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(key), ".", 3)
	if len(parts) != 3 {
		return "", "", false
	}
	if parts[0] != "ext" {
		return "", "", false
	}
	if !ValidExtensionID(parts[1]) {
		return "", "", false
	}
	if !permissionActionRe.MatchString(parts[2]) {
		return "", "", false
	}
	return parts[1], parts[2], true
}
