package extserver

// 신원 헤더(프로토콜 §3) 왕복 검증.
//
// 핵심은 **한글 이름·부서가 왕복해도 원본과 같아야 한다**는 것이다. 인코딩이 어긋나면
// 오류 없이 이름만 깨진 채로 화면·감사에 남는다(가장 늦게 발견되는 종류의 결함).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

// TestHeaderValue_한글왕복 은 URL 인코딩 왕복을 고정한다.
func TestHeaderValue_한글왕복(t *testing.T) {
	for _, raw := range []string{
		"홍길동",
		"정보보호 1팀",
		"플랫폼팀/데이터파트",
		"O'Brien, Kim",
		"a+b",
		"100% 완료",
		"",
		"requester01",
	} {
		encoded := EncodeHeaderValue(raw)
		// 인코딩 결과는 반드시 ASCII 여야 한다 — 그것이 이 인코딩을 도입한 이유다.
		for i := 0; i < len(encoded); i++ {
			if encoded[i] > 0x7e || encoded[i] < 0x20 {
				t.Fatalf("인코딩 결과에 비-ASCII 바이트가 남았다: %q → %q", raw, encoded)
			}
		}
		if got := DecodeHeaderValue(encoded); got != raw {
			t.Errorf("왕복이 깨졌다: %q → %q → %q", raw, encoded, got)
		}
	}
}

// TestDecodeHeaderValue_인코딩안된값도통과 는 하위호환(관대한 디코딩)을 고정한다.
func TestDecodeHeaderValue_인코딩안된값도통과(t *testing.T) {
	if got := DecodeHeaderValue("requester01"); got != "requester01" {
		t.Errorf("순수 ASCII 값이 변형되었다: %q", got)
	}
	// 디코딩 불가능한 값이라도 요청을 실패시키지 않고 원문을 돌려준다.
	if got := DecodeHeaderValue("%zz"); got != "%zz" {
		t.Errorf("디코딩 실패 시 원문을 돌려줘야 한다: %q", got)
	}
}

// TestIdentityFromRequest_전체헤더복원 은 Core 가 보낸 헤더가 Identity 로 복원되는지 고정한다.
func TestIdentityFromRequest_전체헤더복원(t *testing.T) {
	want := extv1.Identity{
		UserID:      "owner01",
		Name:        "김서비스",
		Department:  "정보보호 1팀",
		Roles:       []string{"ServiceOwner", "Requester"},
		Permissions: []string{"topic.view", "ext.sample-process.view"},
		RequestID:   "req-0001",
	}

	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	SetIdentityHeaders(req.Header, want)

	// 헤더에는 원문 한글이 그대로 들어가면 안 된다(인코딩 확인).
	if req.Header.Get(HeaderUserName) == want.Name {
		t.Errorf("이름이 인코딩되지 않은 채 헤더에 실렸다: %q", req.Header.Get(HeaderUserName))
	}

	got := identityFromRequest(req)
	if got.UserID != want.UserID || got.Name != want.Name || got.Department != want.Department {
		t.Errorf("사용자 정보가 다르다: %+v", got)
	}
	if got.RequestID != want.RequestID {
		t.Errorf("requestId 가 다르다: %q", got.RequestID)
	}
	if len(got.Roles) != 2 || got.Roles[0] != "ServiceOwner" || got.Roles[1] != "Requester" {
		t.Errorf("roles 가 다르다: %v", got.Roles)
	}
	if len(got.Permissions) != 2 || !got.HasPermission("ext.sample-process.view") {
		t.Errorf("permissions 가 다르다: %v", got.Permissions)
	}
}

// TestIdentityFromRequest_헤더없으면빈신원 은 신원 없는 요청을 구분할 수 있는지 고정한다.
func TestIdentityFromRequest_헤더없으면빈신원(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	got := identityFromRequest(req)
	if got.UserID != "" || len(got.Roles) != 0 || len(got.Permissions) != 0 {
		t.Errorf("빈 신원이어야 한다: %+v", got)
	}
}

// TestIdentityFrom_컨텍스트왕복 은 컨텍스트 주입·조회를 고정한다(확장 단위 테스트가 쓰는 경로).
func TestIdentityFrom_컨텍스트왕복(t *testing.T) {
	ctx := context.Background()
	if _, ok := IdentityFrom(ctx); ok {
		t.Fatal("빈 컨텍스트에서 신원이 나왔다")
	}
	if _, ok := IdentityFrom(nil); ok { //nolint:staticcheck // nil 컨텍스트 방어 확인
		t.Fatal("nil 컨텍스트에서 신원이 나왔다")
	}

	ctx = WithIdentity(ctx, extv1.Identity{UserID: "admin01", Name: "관리자"})
	got, ok := IdentityFrom(ctx)
	if !ok {
		t.Fatal("주입한 신원을 꺼내지 못했다")
	}
	if got.UserID != "admin01" || got.Name != "관리자" {
		t.Errorf("신원이 다르다: %+v", got)
	}

	// UserID 가 비어 있으면 "신원 없음"으로 취급한다 — 사용자를 알 수 없는데 사용자 행위로
	// 처리하면 감사 기록이 거짓이 된다.
	ctx = WithIdentity(context.Background(), extv1.Identity{Name: "이름만"})
	if _, ok := IdentityFrom(ctx); ok {
		t.Error("UserID 없는 신원이 유효로 판정되었다")
	}
}

// TestRequestToken_컨텍스트왕복 은 단기 요청 토큰이 불투명하게 전달되는지 고정한다.
func TestRequestToken_컨텍스트왕복(t *testing.T) {
	ctx := withRequestToken(context.Background(), "")
	if _, ok := requestTokenFrom(ctx); ok {
		t.Error("빈 토큰이 저장되었다")
	}
	ctx = withRequestToken(context.Background(), "rt-abc")
	got, ok := requestTokenFrom(ctx)
	if !ok || got != "rt-abc" {
		t.Errorf("요청 토큰이 다르다: %q ok=%v", got, ok)
	}
}
