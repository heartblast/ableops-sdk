package extserver

// X-Ableops-* 헤더 ↔ extensionv1.Identity 변환과 요청 컨텍스트(프로토콜 §3).
//
// ⚠ 이 헤더들은 **Core 가 만든 값**이다. Core 는 프록시할 때 클라이언트가 보낸 X-Ableops-* 헤더를
// 전부 삭제하고 자신이 계산한 값으로 다시 채운다(요구A §32). 따라서 Extension 은 이 값을 신뢰해도
// 되지만, **다른 어떤 헤더에서도 사용자 정보를 읽어서는 안 된다**.
//
// 헤더 값 인코딩은 URL 인코딩이다(doc.go 참조). 사용자 이름·부서에는 한글이 들어가므로
// 비-ASCII 값을 헤더에 그대로 실으면 프록시·게이트웨이 구간에서 깨지거나 요청이 거부된다.

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	extv1 "github.com/heartblast/kafka-control-portal/sdk/extension/v1"
)

// Core 가 생성하는 요청 헤더 이름.
const (
	// HeaderPrefix 는 Core 가 소유하는 헤더 네임스페이스다(Core 가 프록시 시 전량 삭제 후 재생성).
	HeaderPrefix = "X-Ableops-"

	// HeaderCallToken 은 Core 가 보낸 요청임을 증명하는 토큰이다.
	HeaderCallToken = "X-Ableops-Call-Token"
	// HeaderExtensionID 는 대상 확장 ID 다.
	HeaderExtensionID = "X-Ableops-Extension-Id"
	// HeaderRequestID 는 요청 추적 ID 다(로그·감사 상관관계).
	HeaderRequestID = "X-Ableops-Request-Id"
	// HeaderUserID 는 요청 사용자 ID 다.
	HeaderUserID = "X-Ableops-User-Id"
	// HeaderUserName 은 요청 사용자 이름이다(URL 인코딩).
	HeaderUserName = "X-Ableops-User-Name"
	// HeaderUserDepartment 는 요청 사용자 부서다(URL 인코딩).
	HeaderUserDepartment = "X-Ableops-User-Department"
	// HeaderRoles 는 쉼표로 구분한 포털 역할 목록이다.
	HeaderRoles = "X-Ableops-Roles"
	// HeaderPermissions 는 쉼표로 구분한 유효 권한 키 목록이다.
	HeaderPermissions = "X-Ableops-Permissions"

	// HeaderOnBehalfOf 는 Extension → Core 호출에서 "누구의 명의인가"를 알리는 헤더다(프로토콜 §4).
	//
	// ⚠ 이 헤더 값만은 **URL 인코딩하지 않는다**(다른 신원 헤더와 다르다).
	// Core 는 이 값을 단기 요청 토큰에 서명해 넣은 원본 사용자 ID 와 그대로 비교하므로,
	// 다시 인코딩하면 '@'·공백이 든 사용자 ID 에서 비교가 어긋나 호출이 통째로 거부된다.
	HeaderOnBehalfOf = "X-Ableops-On-Behalf-Of"
	// HeaderRequestToken 은 Core 가 프록시 시 발급한 단기 요청 토큰이다(있으면 그대로 되돌려 준다).
	//
	// 임의 사용자 사칭을 막기 위한 장치다: Extension 이 On-Behalf-Of 로 아무 사용자나 적어도,
	// Core 는 "그 토큰이 지금 그 사용자에게 발급한 것인가"를 확인할 수 있다. extserver 는 이 값을
	// **해석하지 않고 불투명하게 되돌려 주기만** 한다(형식·수명은 Core 소유).
	HeaderRequestToken = "X-Ableops-Request-Token"
)

// ctxKey 는 이 패키지 전용 컨텍스트 키 타입이다(다른 패키지와 충돌하지 않는다).
type ctxKey int

const (
	ctxKeyIdentity ctxKey = iota
	ctxKeyRequestToken
)

// WithIdentity 는 컨텍스트에 사용자 신원을 넣는다.
//
// 운영 경로에서는 extserver 의 미들웨어가 호출한다. 공개하는 이유는 Extension 의 단위 테스트가
// 핸들러를 직접 호출할 때 신원을 주입할 수 있어야 하기 때문이다.
func WithIdentity(ctx context.Context, id extv1.Identity) context.Context {
	return context.WithValue(ctx, ctxKeyIdentity, id)
}

// IdentityFrom 은 요청 컨텍스트에서 사용자 신원을 꺼낸다.
// 인증 컨텍스트가 없으면(Core 프록시를 거치지 않은 호출) ok=false 다.
func IdentityFrom(ctx context.Context) (extv1.Identity, bool) {
	if ctx == nil {
		return extv1.Identity{}, false
	}
	id, ok := ctx.Value(ctxKeyIdentity).(extv1.Identity)
	if !ok || id.UserID == "" {
		return extv1.Identity{}, false
	}
	return id, true
}

// withRequestToken 은 Core 가 발급한 단기 요청 토큰을 컨텍스트에 넣는다.
func withRequestToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyRequestToken, token)
}

// requestTokenFrom 은 요청 토큰을 꺼낸다(없으면 ok=false).
func requestTokenFrom(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	token, ok := ctx.Value(ctxKeyRequestToken).(string)
	if !ok || token == "" {
		return "", false
	}
	return token, true
}

// EncodeHeaderValue 는 헤더 값을 URL 인코딩한다(한글 이름·부서 대응).
//
// Core 측 프록시도 **같은 인코딩**을 써야 한다 — 인코딩이 어긋나면 사용자 이름이 깨진 채로
// 화면·감사에 남고, 그 사실이 오류 없이 조용히 진행된다.
func EncodeHeaderValue(s string) string {
	return url.QueryEscape(s)
}

// DecodeHeaderValue 는 URL 인코딩된 헤더 값을 되돌린다.
//
// 관대하게 동작한다: 인코딩되지 않은 순수 ASCII 값(%, + 가 없는 값)은 그대로 통과시키고,
// 디코딩에 실패하면 원문을 그대로 돌려준다. 헤더 하나가 깨졌다고 요청 전체를 실패시키는 것보다
// "이름이 조금 이상하게 보이는" 편이 낫기 때문이다(권한 판정은 Permissions 로만 한다).
func DecodeHeaderValue(s string) string {
	if s == "" {
		return ""
	}
	if !strings.ContainsAny(s, "%+") {
		return s
	}
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

// splitList 는 쉼표 구분 목록 헤더를 파싱한다(각 항목도 URL 디코딩한다).
func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := DecodeHeaderValue(strings.TrimSpace(p))
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// joinList 는 쉼표 구분 목록 헤더 값을 만든다(테스트·Core 구현 참고용).
func joinList(values []string) string {
	if len(values) == 0 {
		return ""
	}
	encoded := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		encoded = append(encoded, EncodeHeaderValue(v))
	}
	return strings.Join(encoded, ",")
}

// identityFromRequest 는 요청 헤더에서 사용자 신원을 복원한다.
// UserID 가 비어 있으면 "신원 없음"이며, 호출자가 그 사실을 그대로 컨텍스트에 반영한다.
func identityFromRequest(r *http.Request) extv1.Identity {
	h := r.Header
	return extv1.Identity{
		UserID:      DecodeHeaderValue(strings.TrimSpace(h.Get(HeaderUserID))),
		Name:        DecodeHeaderValue(strings.TrimSpace(h.Get(HeaderUserName))),
		Department:  DecodeHeaderValue(strings.TrimSpace(h.Get(HeaderUserDepartment))),
		Roles:       splitList(h.Get(HeaderRoles)),
		Permissions: splitList(h.Get(HeaderPermissions)),
		RequestID:   strings.TrimSpace(h.Get(HeaderRequestID)),
	}
}

// SetIdentityHeaders 는 Identity 를 요청 헤더로 직렬화한다(프로토콜 §3 과 동일한 인코딩).
//
// extserver 자체는 이 함수를 쓰지 않는다. Extension 의 통합 테스트가 "Core 가 보낸 것과 같은
// 요청"을 만들 수 있도록, 그리고 Core 측 구현이 인코딩 규약을 참조할 수 있도록 공개한다.
func SetIdentityHeaders(h http.Header, id extv1.Identity) {
	setIfNotEmpty := func(key, value string) {
		if value == "" {
			h.Del(key)
			return
		}
		h.Set(key, value)
	}
	setIfNotEmpty(HeaderUserID, EncodeHeaderValue(id.UserID))
	setIfNotEmpty(HeaderUserName, EncodeHeaderValue(id.Name))
	setIfNotEmpty(HeaderUserDepartment, EncodeHeaderValue(id.Department))
	setIfNotEmpty(HeaderRoles, joinList(id.Roles))
	setIfNotEmpty(HeaderPermissions, joinList(id.Permissions))
	setIfNotEmpty(HeaderRequestID, id.RequestID)
}
