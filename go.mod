// SDK 는 **독립 모듈**이다(220 4단계 준비 · 207 레버 B).
//
// # 왜 루트에서 떼어냈는가
//
// 외부 확장 개발자는 `sdk/extension/v1` 하나만 쓴다. 그런데 SDK 가 루트 모듈의 일부이면
// 그 사람의 모듈 그래프에 **루트가 요구하는 모든 것**(Kafka 클라이언트·DB 드라이버·Prometheus,
// 그리고 앞으로 들어올 TUI 라이브러리)이 함께 들어온다. 빌드되지도 다운로드되지도 않지만
// 의존성 해석 대상에는 포함된다.
//
// 모듈을 분리하면 소비자의 go.mod 에는 `require .../sdk` **하나만** 남고 루트는 아예
// require 되지 않는다 — 이것이 「SDK 만 쓰는 사람에게 CLI 의 의존성이 따라가지 않게」 하는
// 유일한 구조다(207 §1-4 가 실험으로 확인).
//
// ⚠ import 경로는 바뀌지 않는다. Go 가 최장 일치 모듈 접두로 해소하므로
// `github.com/heartblast/kafka-control-portal/sdk/extension/v1` 그대로다.
//
// ⚠ 이 파일에 의존성을 늘리지 마라. SDK 가 가벼운 것이 이 분리의 목적이다.
module github.com/heartblast/kafka-control-portal/sdk

go 1.27.0

toolchain go1.27.1

require gopkg.in/yaml.v3 v3.0.1
