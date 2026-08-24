package extensionv1

// SDK Error Contract — Core ↔ Extension 경계의 **안정적인 오류 코드**.
//
// # 왜 코드가 필요한가
//
// 지금까지 경계의 오류는 한국어 문장 하나였다. 문장은 사용자에게 보여 주기 위한 것이라 언제든
// 다듬어지는데, 확장이 그 문장으로 분기하면(strings.Contains("capability")) Core 가 오탈자를
// 고치는 순간 확장의 동작이 조용히 바뀐다. 그래서 **기계가 읽는 값(Code)** 과
// **사람이 읽는 값(Message)** 을 분리한다.
//
//	Code    안정적인 계약이다. v1 이 사는 동안 의미를 바꾸지 않는다.
//	Message 안정적인 계약이 **아니다**. 언제든 다듬어진다. 분기 조건으로 쓰지 않는다.
//
// # in-process 와 외부 프로세스가 같은 코드를 쓴다
//
// built-in Extension 은 Go 오류로, 외부 프로세스 Extension 은 JSON 본문으로 오류를 받는다.
// 두 경로의 의미가 갈리면 "같은 코드인데 배포 형태만 바꿨더니 동작이 다르다"가 된다.
// 그래서 JSON 표현을 이 파일이 함께 정의한다.
//
//	{"code":"CAPABILITY_UNAVAILABLE","message":"…","requestId":"…"}
//
// ⚠ **오류에 시크릿·토큰을 담지 않는다.** Message 는 화면·로그·감사에 그대로 실린다.

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrorCode 는 경계 오류의 안정적인 식별자다.
type ErrorCode string

const (
	// CodeCapabilityUnavailable 은 Manifest 가 선언하지 않았거나 Core 가 제공하지 않는 기능을
	// 사용하려 한 경우다. 재시도해도 결과는 같다 — Manifest 를 고쳐야 한다.
	CodeCapabilityUnavailable ErrorCode = "CAPABILITY_UNAVAILABLE"
	// CodePermissionDenied 는 capability 는 있으나 **이 행위**가 허용되지 않는 경우다
	// (예: config.read 만 선언한 확장이 설정을 저장하려 함, 사용자 권한 부족).
	CodePermissionDenied ErrorCode = "PERMISSION_DENIED"
	// CodeHostUnavailable 은 Core Host 를 지금 사용할 수 없는 경우다(미기동·비활성·연결 실패).
	CodeHostUnavailable ErrorCode = "HOST_UNAVAILABLE"
	// CodeInvalidArgument 는 입력이 계약을 만족하지 않는 경우다.
	CodeInvalidArgument ErrorCode = "INVALID_ARGUMENT"
	// CodeNotFound 는 대상이 존재하지 않는 경우다.
	CodeNotFound ErrorCode = "NOT_FOUND"
	// CodeConflict 는 현재 상태와 충돌해 수행할 수 없는 경우다(중복·상태 불일치).
	CodeConflict ErrorCode = "CONFLICT"
	// CodeIncompatible 은 버전 계약이 맞지 않는 경우다(apiVersion·프로토콜·Core 버전 제약).
	CodeIncompatible ErrorCode = "INCOMPATIBLE"
	// CodeRateLimited 는 호출 빈도 제한에 걸린 경우다.
	CodeRateLimited ErrorCode = "RATE_LIMITED"
	// CodeTemporaryFailure 는 일시적 실패다 — **재시도해 볼 가치가 있는 유일한 코드**다.
	CodeTemporaryFailure ErrorCode = "TEMPORARY_FAILURE"
	// CodeInternal 은 위 어디에도 속하지 않는 내부 오류다.
	CodeInternal ErrorCode = "INTERNAL"
)

// AllErrorCodes 는 v1 이 정의하는 오류 코드 전체를 선언 순서대로 돌려준다.
func AllErrorCodes() []ErrorCode {
	return []ErrorCode{
		CodeCapabilityUnavailable,
		CodePermissionDenied,
		CodeHostUnavailable,
		CodeInvalidArgument,
		CodeNotFound,
		CodeConflict,
		CodeIncompatible,
		CodeRateLimited,
		CodeTemporaryFailure,
		CodeInternal,
	}
}

// KnownErrorCode 는 v1 이 아는 오류 코드인지 판정한다.
//
// 모르는 코드는 **오류로 만들지 않는다** — 새 코드를 추가한 Core 와 구버전 SDK 로 만든 확장이
// 함께 도는 상황에서, 코드 하나 때문에 응답 해석이 통째로 실패하면 안 된다. 호출부는 모르는
// 코드를 CodeInternal 처럼 다루면 된다.
func KnownErrorCode(c ErrorCode) bool {
	for _, x := range AllErrorCodes() {
		if x == c {
			return true
		}
	}
	return false
}

// Error 는 코드가 붙은 경계 오류다.
//
// JSON 표현이 곧 외부 프로세스 Extension 의 wire format 이다(위 주석 참조).
type Error struct {
	// Code 는 안정적인 오류 식별자다.
	Code ErrorCode `json:"code"`
	// Message 는 사용자에게 보여 줄 한국어 설명이다(계약 아님 — 분기 조건으로 쓰지 않는다).
	Message string `json:"message"`
	// RequestID 는 이 오류가 발생한 요청의 추적 ID 다(있을 때만).
	RequestID string `json:"requestId,omitempty"`

	// wrapped 는 원인 오류다. JSON 으로 나가지 않는다(내부 사정이 경계 밖으로 새면 안 된다).
	wrapped error
}

// NewError 는 코드가 붙은 오류를 만든다.
func NewError(code ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Errorf 는 서식 문자열로 코드가 붙은 오류를 만든다.
func Errorf(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WrapError 는 원인 오류를 감싸 코드를 붙인다(원인이 nil 이면 nil 을 돌려준다).
func WrapError(code ErrorCode, message string, cause error) *Error {
	if cause == nil && message == "" {
		return nil
	}
	msg := message
	if msg == "" && cause != nil {
		msg = cause.Error()
	}
	return &Error{Code: code, Message: msg, wrapped: cause}
}

// WithRequestID 는 추적 ID 를 붙인 사본을 돌려준다(원본은 바꾸지 않는다).
func (e *Error) WithRequestID(id string) *Error {
	if e == nil {
		return nil
	}
	out := *e
	out.RequestID = id
	return &out
}

// Error 는 오류 문자열을 만든다(코드 + 메시지).
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	msg := e.Message
	if msg == "" {
		msg = "오류 사유가 제공되지 않았습니다"
	}
	if e.RequestID != "" {
		return fmt.Sprintf("[%s] %s (요청 %s)", string(e.Code), msg, e.RequestID)
	}
	return fmt.Sprintf("[%s] %s", string(e.Code), msg)
}

// Unwrap 은 원인 오류를 돌려준다(errors.Is/As 용).
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.wrapped
}

// Is 는 같은 코드를 가진 *Error 를 동등하게 본다.
//
// 덕분에 `errors.Is(err, extensionv1.NewError(extensionv1.CodeNotFound, ""))` 처럼
// 코드만으로 비교할 수 있고, 메시지 비교로 분기하는 습관이 생기지 않는다.
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) || t == nil || e == nil {
		return false
	}
	return t.Code == e.Code
}

// CodeOf 는 오류에서 코드를 꺼낸다(코드가 붙지 않은 오류는 빈 문자열).
//
// 센티널 오류도 해석한다: ErrCapabilityUnavailable 로 감싼 오류는 CAPABILITY_UNAVAILABLE 이다.
func CodeOf(err error) ErrorCode {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) && e != nil {
		return e.Code
	}
	if errors.Is(err, ErrCapabilityUnavailable) {
		return CodeCapabilityUnavailable
	}
	return ""
}

// HTTPStatusForCode 는 오류 코드에 대응하는 HTTP 상태코드를 돌려준다.
//
// Core Host API 와 외부 프로세스 Extension 이 **같은 표**를 쓰게 하기 위한 것이다.
// 표가 두 벌이면 같은 사유가 한쪽에서는 403, 다른 쪽에서는 400 으로 나간다.
func HTTPStatusForCode(c ErrorCode) int {
	switch c {
	case CodeCapabilityUnavailable, CodePermissionDenied:
		return http.StatusForbidden
	case CodeHostUnavailable:
		return http.StatusServiceUnavailable
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeIncompatible:
		// 412 Precondition Failed — "요청 자체는 유효하나 이 조합으로는 처리할 수 없다".
		return http.StatusPreconditionFailed
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeTemporaryFailure:
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}

// CodeForHTTPStatus 는 HTTP 상태코드를 오류 코드로 되돌린다(HTTPStatusForCode 의 역방향).
//
// 완전한 역함수는 아니다 — 상태코드는 코드보다 표현력이 낮아 401 처럼 대응이 하나로 모이는
// 경우가 있다. 클라이언트가 코드 없는 구버전 Core 응답을 해석할 때의 폴백이다.
func CodeForHTTPStatus(status int) ErrorCode {
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return CodePermissionDenied
	case status == http.StatusNotFound:
		return CodeNotFound
	case status == http.StatusConflict:
		return CodeConflict
	case status == http.StatusPreconditionFailed:
		return CodeIncompatible
	case status == http.StatusTooManyRequests:
		return CodeRateLimited
	case status == http.StatusServiceUnavailable:
		return CodeHostUnavailable
	case status == http.StatusBadGateway, status == http.StatusGatewayTimeout:
		return CodeTemporaryFailure
	case status >= 400 && status < 500:
		return CodeInvalidArgument
	case status >= 500:
		return CodeInternal
	}
	return ""
}

// RetryableCode 는 재시도해도 되는 코드인지 판정한다.
//
// TEMPORARY_FAILURE 만 true 다. 나머지는 재시도해도 같은 결과이며, 특히 신청·감사 같은
// 비멱등 호출을 재시도하면 중복 기록이 남는다.
func RetryableCode(c ErrorCode) bool {
	return c == CodeTemporaryFailure
}
