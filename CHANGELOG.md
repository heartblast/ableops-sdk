# CHANGELOG — AbleOps SDK

이 파일은 **SDK 모듈의 버전 이력**이다. Core 제품(`ableops-kafka`)의 릴리스 노트와 별개이며,
SDK 공개 API 가 바뀌지 않았다면 Core 릴리스 때문에 여기 항목이 늘지 않는다(요청 §6).

버전 규칙(SemVer):

| 구분 | 올리는 경우 | 골든(`extension/v1/testdata/api-v1.golden`) |
| --- | --- | --- |
| Patch | 호환 버그 수정 · 문서 · 테스트킷 개선 | 변화 없음 |
| Minor | 하위 호환 API 추가 | 줄이 **추가만** 됨 |
| Major | Breaking Change | v1 에서는 금지 — `extension/v2` 를 만든다 |

---

## v1.0.0 — 2026-09-07

**AbleOps Core 저장소에서 분리된 첫 독립 릴리스.**

### 변경

- 모듈 경로가 `github.com/heartblast/kafka-control-portal/sdk` 에서
  `github.com/heartblast/ableops-sdk` 로 바뀌었다.
  import 경로도 함께 바뀐다:

  ```go
  // 이전
  import extensionv1 "github.com/heartblast/kafka-control-portal/sdk/extension/v1"
  // 이후
  import extensionv1 "github.com/heartblast/ableops-sdk/extension/v1"
  ```

- SDK 버전이 Core 버전과 **분리**되었다. 더 이상 Core 태그(`v1.6.4`)로 SDK 를 받지 않는다.
- `SDKVersion` 상수 추가(`extension/v1/compat.go`).
- 저장소 루트에 `README.md` · `CHANGELOG.md` · `examples/hello-extension` 추가.

### 유지 (Breaking Change 아님)

Go 타입·인터페이스·Manifest 스키마·wire 프로토콜은 **한 줄도 바뀌지 않았다.**

- API 버전: `ableops.io/extension/v1` (그대로)
- 프로세스 프로토콜 버전: `1` (그대로)
- 프론트엔드 브리지 버전: `1` (그대로)
- 공개 심볼: `extension/v1/testdata/api-v1.golden` 에 변화 없음(추가된 `SDKVersion` 제외)

즉 **기존 확장의 실행 바이너리와 패키지는 그대로 동작한다.** 영향을 받는 것은 확장을
**다시 빌드할 때의 import 경로**뿐이다 → [마이그레이션](README.md#기존-확장-마이그레이션).

### 호환성

| AbleOps SDK | AbleOps Core |
| --- | --- |
| v1.0.x | v1.7.0 이상 |

v1.6.4 이하 Core 는 분리 이전 SDK(`.../kafka-control-portal/sdk@v1.6.x`)를 쓴다.
