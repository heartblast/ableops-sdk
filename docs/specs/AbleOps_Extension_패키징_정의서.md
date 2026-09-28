# AbleOps Extension 패키징 정의서

## 1. 문서 개요

### 1.1 목적

본 문서는 AbleOps 확장기능(Extension)을 개발·배포할 때 사용하는 표준 패키징 규격을 정의한다.

기준 구현은 `heartblast/ableops-kafka-mcp` 프로젝트의 Managed Extension 패키징 구조이며, AbleOps Core(`heartblast/ableops-kafka`)의 실제 설치·검증 규칙과 교차 확인한 내용을 반영한다.

본 정의서는 향후 다음과 같은 별도 Extension 프로젝트에서 공통 기준으로 재사용하는 것을 목적으로 한다.

- MCP 기반 Extension
- AI 기능 Extension
- Skill Builder Extension
- 모니터링 Extension
- 관리 UI Extension
- 데이터 처리/연계 Extension
- 기타 AbleOps Core에 설치되는 독립 확장기능

---

## 2. 적용 범위

본 문서는 다음 항목을 정의한다.

1. Extension 설치 패키지 형식
2. 패키지 디렉터리 구조
3. `manifest.yaml` 규격
4. Managed Extension 실행파일 배치 규칙
5. 플랫폼별 바이너리 빌드 규칙
6. 패키징 스크립트 요구사항
7. 패키지 보안 검증 규칙
8. 서명 정책
9. CI 검증 요구사항
10. 다른 Extension 프로젝트에 재사용하기 위한 권고 프로젝트 구조

본 문서에서 정의하는 설치 패키지는 다음 확장자를 사용한다.

```text
.ableops-ext
```

`.ableops-ext` 파일의 실제 컨테이너 형식은 ZIP이다.

---

## 3. 기준 구현

### 3.1 참조 프로젝트

```text
Repository:
https://github.com/heartblast/ableops-kafka-mcp

기준 브랜치:
main

확인 기준 SHA:
b955629103bcc4db9a12f67f5401ab921827268d
```

### 3.2 Core 설치 계약 참조

```text
Repository:
https://github.com/heartblast/ableops-kafka

주요 참조 영역:
internal/extensionhost/
```

특히 다음 구현이 패키지 설치 규칙의 기준이 된다.

```text
internal/extensionhost/security.go
internal/extensionhost/process.go
internal/extensionhost/signature.go
internal/extensionhost/sign.go
```

---

# 4. 패키지 기본 규격

## 4.1 패키지 파일명

패키지 파일명은 다음 형식을 사용한다.

```text
<extension-id>_<version>.ableops-ext
```

예:

```text
ableops-kafka-mcp_0.6.0.ableops-ext
```

### PKG-001 — 패키지 파일명

**필수**

패키지 파일명은 `manifest.yaml`의 `id`와 `version`을 기준으로 생성해야 한다.

별도의 버전 override 값을 사용해서는 안 된다.

---

## 4.2 패키지 내부 기본 구조

최소 패키지는 다음 구조를 가진다.

```text
manifest.yaml

bin/
  linux-amd64/
    <extension-binary>

  windows-amd64/
    <extension-binary>.exe
```

예:

```text
ableops-kafka-mcp_0.6.0.ableops-ext
│
├── manifest.yaml
│
└── bin/
    ├── linux-amd64/
    │   └── ableops-kafka-mcp-extension
    │
    └── windows-amd64/
        └── ableops-kafka-mcp-extension.exe
```

---

# 5. 허용 최상위 항목

AbleOps Core는 Extension 패키지 최상위에 다음 항목만 허용한다.

```text
manifest.yaml
web/
migrations/
bin/
META-INF/
LICENSE
README
README.md
README.txt
```

### PKG-002 — 최상위 화이트리스트

**필수**

패키지에는 위 목록에 없는 최상위 디렉터리나 파일을 포함해서는 안 된다.

예를 들어 다음 구조는 허용되지 않는다.

```text
src/
scripts/
config/
node_modules/
.env
```

---

# 6. Manifest 규격

## 6.1 Manifest 위치

Manifest는 패키지 최상위에 반드시 다음 이름으로 존재해야 한다.

```text
manifest.yaml
```

개발 소스에서는 다음 위치 사용을 권장한다.

```text
internal/extension/manifest.yaml
```

---

## 6.2 Manifest 예시

```yaml
apiVersion: ableops.io/extension/v1

id: my-extension
name: My Extension
version: 1.0.0
description: AbleOps Extension
publisher: AbleOps

requires:
  core: ">=1.8.0 <2.0.0"

capabilities: []

permissions: []

backend:
  enabled: true
  kind: process

frontend:
  enabled: false
  entry: ""

routes: []

menus: []
```

---

## 6.3 Manifest의 역할

Manifest는 Extension의 다음 정보를 선언한다.

```text
Extension 식별자
Extension 이름
Extension 버전
설명
배포자
Core 호환 범위
Capability
Permission
Backend 유형
Frontend 사용 여부
공개 Route
Menu 기여
```

---

## 6.4 Single Source of Truth

### MAN-001 — Manifest 단일 원천

**필수**

다음 정보는 `manifest.yaml`을 단일 원천으로 사용해야 한다.

```text
id
version
Core 호환 버전
capabilities
permissions
backend
frontend
routes
menus
```

패키징 스크립트나 별도 환경변수에서 `id`, `version`을 중복 관리하지 않는 것을 원칙으로 한다.

---

## 6.5 API Version

### MAN-002 — API Version

**필수**

현재 Extension Manifest API는 다음 값을 사용한다.

```yaml
apiVersion: ableops.io/extension/v1
```

---

## 6.6 Extension ID

### MAN-003 — ID 일치

**필수**

Manifest의 `id`와 프로그램 내부 Extension ID는 반드시 같아야 한다.

예:

```yaml
id: my-extension
```

```go
const ID = "my-extension"
```

CI 또는 단위 테스트를 통해 두 값이 동일한지 검사해야 한다.

---

## 6.7 버전 일치

### MAN-004 — Version 일치

**필수**

다음 버전은 서로 일치해야 한다.

```text
manifest.yaml version
프로그램 내부 version
패키지 파일명 version
관리 UI에서 표시되는 설치 version
```

예:

```text
manifest.yaml              1.2.0
server.Version             1.2.0
my-extension_1.2.0.ableops-ext
```

버전 불일치는 테스트 단계에서 실패하도록 구성하는 것을 권장한다.

---

# 7. Manifest Embed

## 7.1 목적

Manifest를 실행파일 안에 포함하면 설치된 바이너리와 배포 Manifest가 서로 다른 상태가 되는 문제를 줄일 수 있다.

권장 구현:

```go
//go:embed manifest.yaml
var manifestYAML []byte
```

### MAN-005 — Manifest Embed

**권고**

Managed Process Extension은 `manifest.yaml`을 실행 바이너리에 embed하는 것을 권장한다.

---

# 8. Backend 유형

Process 기반 Extension은 다음과 같이 선언한다.

```yaml
backend:
  enabled: true
  kind: process
```

### EXT-001 — Process Extension

Process Extension은 AbleOps Core와 별도 프로세스로 실행되어야 한다.

Go plugin이나 Core 프로세스 내부 동적 로딩 방식에 의존하지 않는다.

---

# 9. Managed Extension 실행 진입점

Standalone 프로그램과 Managed Extension 실행파일을 분리하는 것을 권장한다.

예:

```text
cmd/
  my-service/
    main.go

  my-service-extension/
    main.go
```

Managed Extension의 `main.go`는 가능한 얇게 유지한다.

예:

```go
func main() {
    if err := extserver.Run(extension.New()); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

### EXT-002 — Managed Entry Point

**필수**

Managed Extension 패키지에는 AbleOps Core가 직접 기동할 전용 실행파일이 포함되어야 한다.

Standalone용 실행파일을 Managed Extension 패키지에 혼합하지 않는 것을 원칙으로 한다.

---

# 10. 플랫폼별 바이너리 규격

## 10.1 디렉터리 규칙

Core는 다음 경로에서 현재 OS/Architecture에 해당하는 실행파일을 찾는다.

```text
bin/<GOOS>-<GOARCH>/
```

예:

```text
bin/linux-amd64/
bin/linux-arm64/
bin/windows-amd64/
```

주의:

빌드 대상 입력은 보통:

```text
linux/amd64
```

형식을 사용하지만 패키지 내부 디렉터리는:

```text
linux-amd64
```

형식을 사용한다.

---

## 10.2 Windows

Windows 실행파일은 `.exe`를 사용할 수 있다.

```text
bin/windows-amd64/my-extension.exe
```

---

## 10.3 Unix 실행권한

Linux/macOS 실행파일은 실행 권한이 존재해야 한다.

권장:

```text
0755
```

### BIN-001 — 실행 권한

**필수**

비 Windows 플랫폼의 Managed Extension 실행파일에는 실행 권한을 설정해야 한다.

---

# 11. Go 빌드 기준

Go 기반 Extension은 다음과 같은 빌드 방식을 권장한다.

```bash
CGO_ENABLED=0
GOOS=linux
GOARCH=amd64
GOWORK=off

go build \
  -mod=readonly \
  -trimpath \
  -buildvcs=false \
  -ldflags '-s -w'
```

### BUILD-001 — CGO 비활성화

**권고**

다양한 운영 서버에서 실행될 수 있도록 가능하면 `CGO_ENABLED=0`으로 정적 실행파일을 생성한다.

### BUILD-002 — Local go.work 차단

**권고**

패키징 빌드 시 개발자 로컬의 `go.work`가 빌드 결과에 영향을 주지 않도록 `GOWORK=off`를 사용한다.

### BUILD-003 — Module 변경 방지

**권고**

패키징 빌드 시 다음 옵션 사용을 권장한다.

```text
-mod=readonly
```

---

# 12. 패키징 프로세스

표준 패키징 과정은 다음 단계로 구성한다.

```text
1. manifest.yaml 확인
2. id/version 읽기
3. 현재 코드 사전 컴파일
4. staging directory 초기화
5. manifest.yaml 원본 복사
6. 플랫폼별 cross compile
7. staging 구조 검증
8. ZIP 패키지 생성
9. 생성된 ZIP 재검증
10. .ableops-ext 결과물 생성
```

---

## 12.1 사전 컴파일 검사

### BUILD-004 — Precheck Build

**필수**

기존 정상 배포 파일을 삭제하기 전에 현재 코드가 컴파일 가능한지 먼저 확인해야 한다.

컴파일 실패 시 기존 산출물을 유지해야 한다.

---

# 13. Staging 구조

권장 staging 경로:

```text
build/extensions/<extension-id>/
```

예:

```text
build/extensions/my-extension/
│
├── manifest.yaml
└── bin/
```

최종 산출물은 다음 경로 사용을 권장한다.

```text
dist/extensions/
```

---

# 14. ZIP 생성 규칙

`.ableops-ext`는 ZIP 형식이어야 한다.

### ZIP-001 — ZIP 형식

**필수**

최종 패키지는 실제 ZIP 형식이어야 한다.

단순히 다른 archive를 생성한 후 `.ableops-ext` 확장자로 이름만 변경해서는 안 된다.

ZIP signature는 일반적으로 다음으로 시작한다.

```text
PK
```

---

## 14.1 ZIP 내부 경로

정상:

```text
manifest.yaml
bin/linux-amd64/my-extension
```

금지:

```text
./manifest.yaml
../manifest.yaml
/bin/linux-amd64/my-extension
bin\windows-amd64\my-extension.exe
```

### ZIP-002 — 경로 구분자

**필수**

ZIP 내부 경로 구분자는 `/`를 사용해야 한다.

### ZIP-003 — 상대경로

**필수**

ZIP Entry는 절대경로, 드라이브 경로, `..`, `./` prefix를 포함해서는 안 된다.

---

# 15. 실행파일 배치 제한

Core는 다음 확장자를 실행 가능한 파일로 간주한다.

```text
.exe
.dll
.so
.dylib
.bat
.cmd
.com
.ps1
.sh
.bash
.msi
.scr
```

### SEC-001 — 실행파일 위치

**필수**

실행 가능한 파일은 반드시 `bin/` 하위에 위치해야 한다.

예:

허용:

```text
bin/linux-amd64/my-extension
```

금지:

```text
scripts/install.sh
run.ps1
```

---

# 16. Symlink 금지

### SEC-002 — Symbolic Link

**필수**

Extension 패키지 내부에 Symbolic Link를 포함해서는 안 된다.

목적은 설치 루트 밖의 파일이나 실행파일을 참조하는 경로 탈출 공격을 차단하는 것이다.

---

# 17. 민감정보 및 개발파일 배제

다음 종류의 파일은 패키지에 포함하지 않아야 한다.

```text
.env
.env.*

go.mod
go.sum
go.work
go.work.sum

*.go

secret.key
*.key
*.pem

*credentials*

*.auth-store.json
*auth-store*

*.mcp-token

runtime private config
인증 토큰
API Key
Private Key
사용자 Credential
```

### SEC-003 — Secret 배제

**필수**

Secret, Credential, Token, Private Key는 `.ableops-ext`에 포함해서는 안 된다.

### SEC-004 — Source 배제

**권고**

운영 Extension 패키지에는 Go 소스코드 및 개발 의존성 파일을 포함하지 않는다.

---

# 18. Manifest 원본 보존

패키징 단계에서는 Manifest를 재생성하지 않고 원본 파일을 그대로 복사하는 방식을 권장한다.

```text
internal/extension/manifest.yaml
           ↓ byte-for-byte
manifest.yaml
```

### MAN-006 — Byte-for-byte 보존

**권고**

최종 ZIP의 `manifest.yaml`이 개발 소스의 기준 Manifest와 byte-for-byte 동일한지 검증한다.

---

# 19. 패키지 생성 후 재검증

Staging 디렉터리가 정상이라고 해서 최종 ZIP이 정상이라는 보장은 없다.

따라서 최종 `.ableops-ext` 파일을 다시 열어 검사해야 한다.

### VERIFY-001 — 최종 패키지 검증

**필수**

최종 패키지에 대해 최소 다음 항목을 검증해야 한다.

```text
ZIP format 여부

패키지가 비어 있지 않은지

Entry 경로에 \ 문자가 없는지

절대경로가 없는지

./ prefix가 없는지

.. 경로가 없는지

허용된 top-level 항목만 존재하는지

manifest.yaml 존재 여부

manifest.yaml 원본 일치 여부

빌드 대상 binary 존재 여부

Standalone binary 미포함 여부

민감 파일 미포함 여부

Source Code 미포함 여부
```

---

# 20. 패키지 보안 상한

AbleOps Core는 패키지 해제 시 다음 유형의 공격을 방어하기 위한 제한을 둔다.

```text
과도한 Entry 개수
단일 파일 과대 크기
전체 압축 해제 크기
비정상적인 압축률
ZIP Bomb
```

패키지 생성 프로젝트에서도 과도한 파일 수 또는 비정상적으로 큰 파일이 포함되지 않도록 별도 검증을 추가할 수 있다.

---

# 21. Frontend Extension

화면을 제공하지 않는 경우:

```yaml
frontend:
  enabled: false
  entry: ""
```

화면 기능이 필요한 Extension은 `web/` 아래에 정적 리소스를 패키징하도록 설계한다.

권장 구조:

```text
web/
  index.html
  assets/
```

Frontend 관련 세부 규격은 AbleOps Core의 Extension Frontend 계약을 별도 정의서로 관리하는 것을 권장한다.

---

# 22. Migration

Extension이 DB Migration을 제공하는 경우 다음 디렉터리를 사용할 수 있다.

```text
migrations/
```

예:

```text
migrations/
  000001_init.sql
  000002_add_feature.sql
```

Migration 파일 형식과 실행 정책은 Core의 Migration 계약을 별도로 따른다.

---

# 23. Public Route

Extension이 Core 공개 프록시를 통해 API를 제공해야 하는 경우에만 Manifest에 Route를 선언한다.

내부 통신 전용 Endpoint는 불필요하게 공개 Route로 선언하지 않아야 한다.

### SEC-005 — 최소 Route

**권고**

사용하지 않는 Route는 선언하지 않는다.

---

# 24. Capability / Permission 최소권한

### SEC-006 — Least Privilege

**필수**

Extension은 실제 사용하는 Capability 및 Permission만 Manifest에 선언해야 한다.

사용하지 않는 권한을 사전에 넓게 선언해서는 안 된다.

예:

```yaml
capabilities: []
permissions: []
```

사용하지 않는 경우 빈 배열을 유지한다.

---

# 25. 서명

기본 빌드 결과는 미서명 패키지일 수 있다.

AbleOps Core의 서명 정책은 다음과 같다.

```text
signatureMode=off

미서명:
설치 가능


signatureMode=warn

미서명:
설치 가능 + 경고

잘못된 서명:
설치 거부


signatureMode=require

미서명:
설치 거부

유효한 서명:
설치 가능
```

---

## 25.1 서명 파일 위치

서명된 패키지는 다음 경로를 사용한다.

```text
META-INF/ableops-signature.json
```

서명 알고리즘:

```text
Ed25519
```

### SIGN-001 — 공식 서명 도구

**필수**

서명 파일 형식을 Extension 프로젝트에서 임의 구현하지 않는다.

AbleOps Core와 호환되는 공식 signing tool을 사용한다.

---

# 26. 권장 Build Pipeline

표준 빌드 파이프라인은 다음과 같다.

```text
Source
  ↓
Unit Test
  ↓
Static Check
  ↓
Compile Precheck
  ↓
Cross Build
  ↓
Package
  ↓
Package Verification
  ↓
Security Check
  ↓
Optional Signing
  ↓
Release
```

---

# 27. CI 요구사항

최소 CI는 다음 검증을 포함해야 한다.

```text
go test ./...
go vet ./...
go build
build-extension.sh
```

추가 권고:

```text
go test -race
dependency vulnerability scan
manifest contract test
package validation
```

### CI-001 — 패키징 검증

**필수**

CI에서 실제 `.ableops-ext` 패키지를 생성하고 검증해야 한다.

소스 컴파일만 성공했다고 Release 가능한 것으로 판단해서는 안 된다.

---

# 28. 권장 프로젝트 구조

새 AbleOps Extension 프로젝트는 다음 구조를 권장한다.

```text
my-ableops-extension/
│
├── cmd/
│   └── my-ableops-extension/
│       └── main.go
│
├── internal/
│   └── extension/
│       ├── manifest.yaml
│       ├── manifest.go
│       ├── extension.go
│       └── extension_test.go
│
├── scripts/
│   ├── build-extension.sh
│   └── build-extension.ps1
│
├── web/
│
├── migrations/
│
├── go.mod
├── go.sum
└── README.md
```

`web/`, `migrations/`는 사용하는 프로젝트에서만 둔다.

---

# 29. 공통 패키징 스크립트 변수

`ableops-kafka-mcp`의 현재 스크립트는 단일 Extension을 위한 고정 설정을 사용한다.

여러 Extension 프로젝트에서 재사용하려면 다음 값을 변수화하는 것을 권장한다.

```text
EXTENSION_ID
MANIFEST_PATH
COMMAND_PACKAGE
BINARY_NAME
OUTPUT_DIR
DEFAULT_TARGETS
```

예:

```bash
MANIFEST="internal/extension/manifest.yaml"
CMD_PKG="./cmd/my-extension"
BIN_NAME="my-extension"
OUT_DIR="dist/extensions"
TARGETS="linux/amd64,windows/amd64"
```

---

# 30. 재사용 가능한 공통 패키징 모듈

향후 다음 구조의 공통화가 가능하다.

```text
AbleOps Extension SDK
        │
        ├─ Manifest Contract
        ├─ Runtime Contract
        ├─ Packaging Contract
        ├─ Package Validator
        └─ Signing Tool
```

Extension 프로젝트는 다음만 정의하도록 단순화할 수 있다.

```text
manifest.yaml
entrypoint
application logic
frontend(optional)
migration(optional)
```

공통 Packaging Tool은 다음을 담당한다.

```text
Cross Build
Staging
ZIP Packaging
Structure Validation
Secret Validation
Manifest Validation
Binary Validation
Signing Integration
```

---

# 31. 표준 산출물 예시

Extension ID:

```text
ableops-skill-builder
```

Version:

```text
1.0.0
```

최종 결과:

```text
dist/extensions/
└── ableops-skill-builder_1.0.0.ableops-ext
```

내부:

```text
manifest.yaml

bin/
  linux-amd64/
    ableops-skill-builder

  windows-amd64/
    ableops-skill-builder.exe

web/
  index.html
  assets/

README.md
```

서명된 운영 패키지라면:

```text
META-INF/
  ableops-signature.json
```

이 추가될 수 있다.

---

# 32. 필수 요구사항 요약

| ID | 요구사항 | 수준 |
|---|---|---|
| PKG-001 | 파일명은 Manifest ID/Version에서 생성 | 필수 |
| PKG-002 | Core 허용 최상위 항목만 사용 | 필수 |
| MAN-001 | Manifest를 메타데이터 단일 원천으로 사용 | 필수 |
| MAN-002 | `ableops.io/extension/v1` 사용 | 필수 |
| MAN-003 | Manifest ID와 코드 ID 일치 | 필수 |
| MAN-004 | Manifest/프로그램/패키지 버전 일치 | 필수 |
| MAN-005 | Manifest binary embed | 권고 |
| MAN-006 | Manifest byte-for-byte 검증 | 권고 |
| EXT-001 | Process Extension은 별도 프로세스로 실행 | 필수 |
| EXT-002 | Managed용 실행 Entry Point 제공 | 필수 |
| BIN-001 | Unix 실행파일 실행 권한 설정 | 필수 |
| BUILD-001 | 가능하면 `CGO_ENABLED=0` | 권고 |
| BUILD-002 | `GOWORK=off` | 권고 |
| BUILD-003 | `-mod=readonly` | 권고 |
| BUILD-004 | 기존 산출물 삭제 전 compile precheck | 필수 |
| ZIP-001 | 실제 ZIP 형식 | 필수 |
| ZIP-002 | ZIP 내부 `/` 경로 사용 | 필수 |
| ZIP-003 | 절대경로/`..`/`./` 금지 | 필수 |
| SEC-001 | 실행파일은 `bin/` 아래만 허용 | 필수 |
| SEC-002 | Symlink 금지 | 필수 |
| SEC-003 | Secret/Credential 미포함 | 필수 |
| SEC-004 | Source Code 미포함 | 권고 |
| SEC-005 | 불필요 Route 미선언 | 권고 |
| SEC-006 | Capability/Permission 최소권한 | 필수 |
| VERIFY-001 | 최종 ZIP 재검증 | 필수 |
| SIGN-001 | 공식 signing tool 사용 | 필수 |
| CI-001 | CI에서 실제 패키지 생성·검증 | 필수 |

---

# 33. 권장 구현 원칙

AbleOps Extension 패키징의 핵심 원칙은 다음과 같다.

```text
1. Manifest First
2. Single Source of Truth
3. Managed Runtime 분리
4. Platform Binary 명시
5. Whitelist Packaging
6. Secret Zero Packaging
7. Build 후 Package 재검증
8. Least Privilege
9. Signing 분리
10. CI에서 실제 설치 패키지 검증
```

---

# 34. 최종 표준

AbleOps Extension의 표준 배포 단위는 다음으로 정의한다.

```text
.ableops-ext
```

패키지는 다음 세 계층의 계약을 모두 만족해야 한다.

```text
┌─────────────────────────────┐
│ Package Contract            │
│ manifest / bin / web / etc. │
└──────────────┬──────────────┘
               │
┌──────────────▼──────────────┐
│ Runtime Contract            │
│ SDK / handshake / lifecycle │
└──────────────┬──────────────┘
               │
┌──────────────▼──────────────┐
│ Security Contract           │
│ whitelist / secret / sign   │
└──────────────┬──────────────┘
               │
          AbleOps Core
```

따라서 새로운 Extension은 단순한 실행파일이 아니라 다음 구성으로 개발되어야 한다.

```text
Extension Application
        +
Extension Manifest
        +
Managed Runtime
        +
Packaging
        +
Validation
        +
Optional Signature
```

이 규격을 공통화하면 이후 AbleOps용 신규 Extension 프로젝트는 동일한 설치·업데이트·검증·서명 체계를 공유할 수 있다.
