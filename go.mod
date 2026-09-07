// AbleOps SDK — AbleOps 플랫폼과 Extension 사이의 **공개 계약 모듈**이다.
//
// # 무엇인가
//
// Kafka 전용 SDK 가 아니다. Core 제품(`ableops-kafka`)이 Extension 에게 약속하는
// 타입·인터페이스·wire 계약만을 담는다. 외부 Extension(예: `ableops-flink`)은 이 모듈
// **하나만** require 하면 개발·빌드·테스트가 끝나야 한다.
//
// # 왜 Core 저장소에서 떼어냈는가
//
// SDK 가 Core 모듈의 일부이면 확장 개발자의 모듈 그래프에 **Core 가 요구하는 모든 것**
// (Kafka 클라이언트·DB 드라이버 3종·Prometheus·TUI 라이브러리)이 함께 들어온다.
// 빌드되지도 다운로드되지도 않지만 의존성 해석 대상에는 포함된다.
// 모듈을 분리하면 소비자의 go.mod 에는 `require github.com/heartblast/ableops-sdk`
// **하나만** 남고 Core 는 아예 require 되지 않는다.
//
// # 저장소
//
// 이 디렉터리는 독립 저장소 `github.com/heartblast/ableops-sdk` 의 **루트**가 된다
// (Core 저장소 안에서는 `sdk/` 에 스테이징된다 — 추출 절차는
// docs/reference_docs/extension-sdk/12-SDK저장소분리.md).
// 따라서 이 아래의 경로는 그대로 공개 import 경로다:
//
//	github.com/heartblast/ableops-sdk/extension/v1
//	github.com/heartblast/ableops-sdk/extension/v1/extserver
//	github.com/heartblast/ableops-sdk/extension/v1/testkit
//
// ⚠ 이 파일에 의존성을 늘리지 마라. SDK 가 가벼운 것이 이 분리의 목적이다.
// (게이트: extension/v1/module_gate_test.go · extension/v1/imports_test.go)
//
// ⚠ `toolchain` 줄은 **소비자 하한과 무관하다.** 이 모듈이 메인 모듈일 때만 적용되고
// 의존 모듈에서는 무시되므로, 확장 개발자가 설치해야 하는 Go 버전은 아래 `go` 값이다.
module github.com/heartblast/ableops-sdk

go 1.27.0

toolchain go1.27.1

require gopkg.in/yaml.v3 v3.0.1
