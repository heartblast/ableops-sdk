package extensionv1

// AuditService — Extension 행위의 감사 기록(capability: audit.write).

import "context"

// AuditEntry 는 Extension 이 남기는 감사 기록 1건이다.
//
// 수행자(사용자)·시각·Extension ID·클라이언트 IP 등 **신뢰가 필요한 항목은 Core 가 채운다** —
// Extension 이 자기 마음대로 "누가 했는지"를 적을 수 있으면 감사 기록이 증거가 되지 못한다.
// Detail 에는 시크릿(비밀번호·토큰·키)을 넣지 않는다.
type AuditEntry struct {
	Action   string `json:"action"`           // 수행 행위(예: sample.report.export)
	Target   string `json:"target,omitempty"` // 대상 자원(토픽명·Principal 등)
	Result   string `json:"result"`           // SUCCESS | FAILED
	Detail   string `json:"detail,omitempty"` // 한국어 상세 설명
	HighRisk bool   `json:"highRisk,omitempty"`
}

// 감사 결과 값.
const (
	AuditResultSuccess = "SUCCESS"
	AuditResultFailed  = "FAILED"
)

// AuditService 는 감사 기록 계약이다.
type AuditService interface {
	// Record 는 감사 기록을 남긴다.
	//
	// **에러를 반환하지 않는다.** 감사 저장 실패를 이유로 사용자의 요청을 거부하면
	// 감사 인프라 장애가 곧 서비스 장애가 되고, Extension 마다 제각각의 실패 처리를 하게 된다.
	// 저장 실패는 Core 가 자체 로그로 남기고 재시도·경보를 판단한다.
	// 대신 Extension 은 Record 호출을 생략하지 않는다 — 기록 누락은 통제 실패다.
	Record(ctx context.Context, entry AuditEntry)
}
