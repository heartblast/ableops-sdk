package extensionv1

// SDK Error Contract 테스트 — 코드가 계약이고 메시지는 계약이 아니라는 규칙을 고정한다.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

// TestErrorCode_상수값 은 코드 문자열이 바뀌지 않음을 고정한다.
//
// ⚠ 이 값은 wire 계약이다. 바꾸면 이미 배포된 Extension 의 분기가 조용히 실패한다.
func TestErrorCode_상수값(t *testing.T) {
	want := map[ErrorCode]string{
		CodeCapabilityUnavailable: "CAPABILITY_UNAVAILABLE",
		CodePermissionDenied:      "PERMISSION_DENIED",
		CodeHostUnavailable:       "HOST_UNAVAILABLE",
		CodeInvalidArgument:       "INVALID_ARGUMENT",
		CodeNotFound:              "NOT_FOUND",
		CodeConflict:              "CONFLICT",
		CodeIncompatible:          "INCOMPATIBLE",
		CodeRateLimited:           "RATE_LIMITED",
		CodeTemporaryFailure:      "TEMPORARY_FAILURE",
		CodeInternal:              "INTERNAL",
	}
	if len(AllErrorCodes()) != len(want) {
		t.Fatalf("오류 코드 개수가 다르다: %d (기대 %d)", len(AllErrorCodes()), len(want))
	}
	for c, s := range want {
		if string(c) != s {
			t.Errorf("오류 코드 값이 바뀌었다: %q != %q", string(c), s)
		}
		if !KnownErrorCode(c) {
			t.Errorf("KnownErrorCode(%q) = false", c)
		}
	}
	// 모르는 코드는 **거부하지 않는다**(상위 Core 와의 전방호환).
	if KnownErrorCode("FUTURE_CODE") {
		t.Error("모르는 코드가 known 으로 판정됐다")
	}
}

// TestError_코드로비교 는 메시지가 아니라 코드로 분기할 수 있음을 고정한다.
func TestError_코드로비교(t *testing.T) {
	err := Errorf(CodeNotFound, "Topic 을 찾을 수 없습니다: %s", "orders")
	if !errors.Is(err, NewError(CodeNotFound, "")) {
		t.Error("같은 코드끼리 errors.Is 가 성립하지 않는다")
	}
	if errors.Is(err, NewError(CodeConflict, "")) {
		t.Error("다른 코드가 같다고 판정됐다")
	}
	wrapped := fmt.Errorf("상위 문맥: %w", err)
	if CodeOf(wrapped) != CodeNotFound {
		t.Errorf("감싼 오류에서 코드를 꺼내지 못했다: %q", CodeOf(wrapped))
	}
	if CodeOf(errors.New("코드 없는 오류")) != "" {
		t.Error("코드 없는 오류에서 코드가 나왔다")
	}
}

// TestError_원인보존 은 WrapError 가 errors.Is 사슬을 끊지 않음을 확인한다.
func TestError_원인보존(t *testing.T) {
	cause := errors.New("근본 원인")
	err := WrapError(CodeHostUnavailable, "Core 에 연결할 수 없습니다", cause)
	if !errors.Is(err, cause) {
		t.Error("원인 오류가 사라졌다")
	}
	if CodeOf(err) != CodeHostUnavailable {
		t.Errorf("코드가 다르다: %q", CodeOf(err))
	}
}

// TestError_JSON형식 은 외부 프로세스 wire format 을 고정한다.
func TestError_JSON형식(t *testing.T) {
	err := NewError(CodeInvalidArgument, "Topic 이름이 비어 있습니다").WithRequestID("abc123")
	data, merr := json.Marshal(err)
	if merr != nil {
		t.Fatalf("직렬화 실패: %v", merr)
	}
	var got map[string]any
	if uerr := json.Unmarshal(data, &got); uerr != nil {
		t.Fatalf("역직렬화 실패: %v", uerr)
	}
	if got["code"] != "INVALID_ARGUMENT" || got["requestId"] != "abc123" {
		t.Errorf("wire format 이 다르다: %s", data)
	}
	if _, has := got["wrapped"]; has {
		t.Error("내부 원인 오류가 경계 밖으로 새어 나갔다")
	}
}

// TestHTTPStatusForCode_왕복 은 Core 와 Extension 이 같은 상태코드 표를 쓰는지 고정한다.
func TestHTTPStatusForCode_왕복(t *testing.T) {
	cases := map[ErrorCode]int{
		CodeCapabilityUnavailable: http.StatusForbidden,
		CodePermissionDenied:      http.StatusForbidden,
		CodeHostUnavailable:       http.StatusServiceUnavailable,
		CodeInvalidArgument:       http.StatusBadRequest,
		CodeNotFound:              http.StatusNotFound,
		CodeConflict:              http.StatusConflict,
		CodeIncompatible:          http.StatusPreconditionFailed,
		CodeRateLimited:           http.StatusTooManyRequests,
		CodeTemporaryFailure:      http.StatusBadGateway,
		CodeInternal:              http.StatusInternalServerError,
	}
	for code, status := range cases {
		if got := HTTPStatusForCode(code); got != status {
			t.Errorf("%s 의 상태코드가 다르다: %d (기대 %d)", code, got, status)
		}
	}
	// 역방향은 완전한 역함수가 아니다(상태코드가 코드보다 표현력이 낮다) — 계약은 "합리적 폴백".
	if CodeForHTTPStatus(http.StatusUnauthorized) != CodePermissionDenied {
		t.Error("401 폴백이 다르다")
	}
	if CodeForHTTPStatus(http.StatusOK) != "" {
		t.Error("성공 상태코드에서 오류 코드가 나왔다")
	}
}

// TestRetryableCode 는 재시도 대상이 TEMPORARY_FAILURE 하나뿐임을 고정한다.
//
// ⚠ 넓히면 신청·감사 같은 비멱등 호출이 중복 기록된다.
func TestRetryableCode(t *testing.T) {
	if !RetryableCode(CodeTemporaryFailure) {
		t.Error("TEMPORARY_FAILURE 가 재시도 대상이 아니다")
	}
	for _, c := range AllErrorCodes() {
		if c == CodeTemporaryFailure {
			continue
		}
		if RetryableCode(c) {
			t.Errorf("%s 가 재시도 대상으로 판정됐다", c)
		}
	}
}
