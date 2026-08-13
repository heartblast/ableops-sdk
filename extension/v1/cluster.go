package extensionv1

// ClusterRegistry — Extension 에 제공되는 클러스터 메타데이터 조회(capability: cluster.read).

import "context"

// ClusterInfo 는 Kafka 클러스터 1개의 **메타데이터만** 담는다.
//
// ⚠ 브로커 주소(brokers)·SASL 계정/비밀번호·TLS 인증서/트러스트스토어 경로 등
// **접속정보 필드를 의도적으로 두지 않는다**:
//
//   - Extension 에 접속정보가 전달되면 Extension 이 Core 를 우회해 Kafka 에 직접 붙을 수 있고,
//     그 순간 정책 검증·승인·감사가 모두 무력화된다(요구A §17·§18).
//   - 접속정보에는 시크릿이 섞여 있어 전달 자체가 시크릿 유출이다(요구A §19).
//
// Kafka 접근이 필요하면 KafkaReadService 를, 변경이 필요하면 WorkflowSubmitService 를 쓴다.
// 필드를 "나중에 추가"하지 않는다 — 한 번 추가되면 되돌릴 수 없다.
type ClusterInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Environment  string `json:"environment"` // prod | test | dev | lab
	KafkaVersion string `json:"kafkaVersion,omitempty"`
	Active       bool   `json:"active"`
}

// ClusterRegistry 는 클러스터 목록·단건 조회 계약이다.
// 반환 목록은 **현재 사용자가 접근 가능한 클러스터로 Core 가 이미 필터링한 결과**다.
type ClusterRegistry interface {
	List(ctx context.Context) ([]ClusterInfo, error)
	Get(ctx context.Context, id string) (ClusterInfo, bool)
}
