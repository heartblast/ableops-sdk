// Package extensionv1 은 AbleOps Studio for Kafka 의 **Extension API v1** 공개 계약이다.
//
// 이 패키지는 Core(포털 본체)와 Extension(확장 기능 패키지) 사이의 유일한 접점이며,
// 외부 저장소의 Extension 이 이 패키지 하나만 import 하면 되도록 **자족적(self-contained)** 으로 유지한다.
//
// # 철칙 — internal/ 을 절대 import 하지 않는다
//
// `sdk/` 는 `github.com/heartblast/kafka-control-portal/internal/...` 을 어떤 경로로도 import 하지 않는다.
// 이유는 두 가지다.
//
//  1. Go 의 internal 규칙상 외부 모듈은 `internal/` 을 import 할 수 없다. SDK 가 internal 타입을 노출하면
//     외부 Extension 은 이 SDK 를 **컴파일조차 할 수 없다**.
//  2. internal 도메인 타입(internal/domain 등)은 Core 의 내부 사정으로 수시로 바뀐다. 그것을 SDK 시그니처에
//     노출하면 Core 리팩터링이 곧바로 모든 Extension 의 파괴적 변경이 된다.
//
// 따라서 이 패키지는 **SDK 전용 DTO** 만 선언한다. Core 쪽 어댑터(internal/extensionhost)가
// internal 도메인 ↔ SDK DTO 변환을 책임진다. 이 규칙은 TestSDKDoesNotImportInternal 이 강제한다.
//
// 의존성은 표준 라이브러리 + gopkg.in/yaml.v3(Manifest 파싱) 뿐이다. 그 외 의존을 추가하지 않는다.
//
// # 버전 정책
//
// APIVersion 은 "ableops.io/extension/v1" 이다. v1 이 살아 있는 동안 이 패키지의 공개 심볼은
// **하위호환을 유지**한다: 기존 타입·필드·메서드 시그니처를 제거하거나 의미를 바꾸지 않는다.
// 확장이 필요하면 필드/상수/함수를 **추가**하고, 인터페이스에 메서드를 추가해야 하는 변경은
// 새 인터페이스를 별도로 만들거나 v2 패키지를 신설한다(인터페이스 메서드 추가는 기존 구현을 깨뜨린다).
//
// # 파일 지도
//
//	doc.go         이 문서
//	apiversion.go  APIVersion 상수·호환 판정
//	manifest.go    Manifest 스키마·YAML 파싱·검증·Sanitized(마스킹)
//	capability.go  Capability 6종(최소권한 단위)
//	permission.go  ext.<id>.<action> 권한 키 조립·검증·파싱
//	lifecycle.go   런타임 상태 9종·설치 단계 8종·전이 허용표
//	version.go     외부 의존 없는 semver 파서·제약(range) 판정
//	identity.go    Core 가 전달하는 검증된 사용자 컨텍스트
//	kafka.go       KafkaReadService(읽기 전용) + Kafka DTO
//	workflow.go    WorkflowSubmitService(신청만) + 신청 입력 DTO
//	audit.go       AuditService(감사 기록)
//	cluster.go     ClusterRegistry(접속정보 없는 클러스터 메타)
//	config.go      ConfigDecl/ConfigField + ConfigService(자기 설정만)
//	secret.go      SecretRef/SecretRefService(참조만, 평문 없음)
//	ui.go          RouteDecl/MenuDecl(프론트 기여 선언)
//	host.go        HostContext(capability 주입 지점)·Logger·안전 접근자
//	extension.go   built-in Extension 이 구현하는 계약(Manifest/Start/Stop/Health/Routes)
//
// # 권장 import
//
//	import extv1 "github.com/heartblast/ableops-sdk/extension/v1"
package extensionv1
