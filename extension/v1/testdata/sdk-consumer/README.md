# 외부 Consumer 컴파일 픽스처

이 디렉터리는 **별도 Go 모듈**이다. 목적은 하나다:

> AbleOps Extension SDK 를 저장소 밖의 모듈에서 import 했을 때 실제로 컴파일되는가?

같은 모듈 안에서만 테스트하면 `internal/` 규칙 위반이나 불필요한 Core 의존이 드러나지 않는다
(같은 모듈에서는 `internal/` import 가 정상 빌드되기 때문이다). 여기서 빌드해야 진짜 외부
개발자가 겪을 상황이 재현된다.

- `go.mod` 의 `replace` 가 이 저장소를 가리키므로 로컬 SDK 변경이 즉시 반영된다.
- 실행은 `sdk/extension/v1/consumer_compile_test.go` 가 임시 디렉터리로 복사해 수행한다.
- `testdata/` 이므로 저장소 루트의 `go build ./...` 는 이 디렉터리를 보지 않는다.

⚠ 이 픽스처에 Core 패키지(`internal/…`)를 import 하지 않는다. 그러면 검증이 무의미해진다.
