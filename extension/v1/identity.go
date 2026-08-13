package extensionv1

// Identity — Core 가 Extension 에 전달하는 **검증된** 사용자 컨텍스트.

// Identity 는 현재 요청을 수행하는 포털 사용자다.
//
// ⚠ 이 값은 **Core 가 세션/토큰을 검증한 결과**이며, Extension 은 이 값을 신뢰해도 된다.
// 반대로 Extension 은 **HTTP 헤더에서 사용자 정보를 읽어서는 안 된다** — 헤더는 클라이언트가
// 임의로 채울 수 있으므로(X-User-Id 위조) 그것을 신뢰하면 권한 상승이 된다.
// Core 는 Extension 으로 프록시할 때 클라이언트가 보낸 신원 관련 헤더를 제거하고
// 자신이 계산한 값으로 덮어쓴다. 사용자 판별은 오직 HostContext.Identity(ctx) 로만 한다.
//
// Permissions 는 Core RBAC 이 계산한 **최종 권한 키 목록**이다(Core 정적 권한 + ext.<id>.<action>).
// Roles 만 보고 직접 권한을 유추하지 말고 HasPermission 을 쓴다 — 역할→권한 매핑은 Core 소유다.
type Identity struct {
	UserID      string   `json:"userId"`
	Name        string   `json:"name"`
	Department  string   `json:"department"`
	Roles       []string `json:"roles,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	// RequestID 는 요청 추적용 식별자다(로그·감사 상관관계 추적).
	RequestID string `json:"requestId,omitempty"`
}

// HasPermission 은 사용자가 해당 권한 키를 보유하는지 확인한다.
func (i Identity) HasPermission(key string) bool {
	if key == "" {
		return false
	}
	for _, p := range i.Permissions {
		if p == key {
			return true
		}
	}
	return false
}

// HasRole 은 사용자가 해당 포털 역할을 보유하는지 확인한다.
// 권한 판정에는 HasPermission 을 쓰고, 이 메서드는 화면 표기 등 보조 용도로만 쓴다.
func (i Identity) HasRole(role string) bool {
	if role == "" {
		return false
	}
	for _, r := range i.Roles {
		if r == role {
			return true
		}
	}
	return false
}
