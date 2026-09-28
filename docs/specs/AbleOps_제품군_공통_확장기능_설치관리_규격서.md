# AbleOps 제품군 공통 확장기능 설치·관리 규격서

## 1. 문서 목적

본 문서는 AbleOps 제품군에서 공통으로 사용하는 **확장기능(Extension) 설치·관리 표준 규격**을 정의한다.

이 규격은 특정 제품(`ableops-kafka`, `ableops-flink`, `ableops-mcp-gateway` 등)에 종속되지 않으며, 향후 모든 AbleOps 서비스가 동일한 방식으로 확장기능을 설치·검증·기동·중지·업데이트·롤백·삭제·감사할 수 있도록 하는 공통 계약을 목적으로 한다.

본 규격을 준수하는 제품은 다음을 공통으로 제공해야 한다.

- `.ableops-ext` 확장기능 패키지 설치
- Manifest 기반 호환성 및 권한 검증
- 플랫폼별 실행파일 선택
- Extension lifecycle 관리
- Enable / Disable 관리
- 업데이트 및 롤백
- 서명 및 신뢰 검증
- 패키지 보안 검사
- 상태 및 오류 관리
- 관리 UI / API
- 감사 이력
- 장애 격리
- 제품별 기능 확장 포인트

---

# 2. 적용 대상

본 규격은 다음 AbleOps 제품군에 공통 적용하는 것을 목표로 한다.

```text
ableops-kafka
ableops-flink
ableops-mcp-gateway
ableops-ai
ableops-skill
향후 신규 AbleOps 서비스
```

각 제품은 제품 고유 기능을 Extension으로 확장할 수 있으나, **설치·관리·보안·Lifecycle 계약은 본 규격을 따라야 한다.**

---

# 3. 설계 원칙

AbleOps Extension 플랫폼은 다음 원칙을 따른다.

```text
1. Package Standardization
2. Manifest First
3. Single Source of Truth
4. Process Isolation
5. Least Privilege
6. Secure by Default
7. Fail Closed
8. Explicit Compatibility
9. Persistent Operational State
10. Observable Lifecycle
11. Safe Update / Rollback
12. Product Independent Contract
```

---

# 4. 공통 용어

## 4.1 Host

Extension을 설치하고 관리하는 AbleOps 제품을 의미한다.

예:

```text
AbleOps Kafka
AbleOps Flink
AbleOps MCP Gateway
```

---

## 4.2 Extension

AbleOps Host의 기능을 확장하는 독립 배포 단위이다.

Extension은 다음 중 하나 이상의 기능을 제공할 수 있다.

```text
Backend Process
Frontend UI
Menu
API Route
Migration
MCP 기능
AI 기능
Monitoring
Workflow
Connector
Automation
```

---

## 4.3 Managed Extension

Host가 직접 설치하고 Lifecycle을 관리하는 Extension을 의미한다.

Host가 다음을 담당한다.

```text
Install
Validate
Start
Stop
Restart
Enable
Disable
Update
Rollback
Uninstall
Health Check
Audit
```

---

## 4.4 Extension Package

Extension 배포 파일이다.

표준 확장자:

```text
.ableops-ext
```

실제 컨테이너 형식:

```text
ZIP
```

---

# 5. Extension Package 표준

## 5.1 파일명

패키지 파일명은 다음 형식을 권장한다.

```text
<extension-id>_<version>.ableops-ext
```

예:

```text
ableops-ai-extension_1.0.0.ableops-ext
```

### PKG-001

패키지 파일명의 `id`와 `version`은 `manifest.yaml`에서 가져와야 한다.

---

# 6. 패키지 내부 구조

허용되는 최상위 항목은 다음과 같다.

```text
manifest.yaml
bin/
web/
migrations/
META-INF/
LICENSE
README
README.md
README.txt
```

최소 Process Extension:

```text
manifest.yaml

bin/
  linux-amd64/
    extension-binary

  windows-amd64/
    extension-binary.exe
```

Frontend가 있는 경우:

```text
web/
  index.html
  assets/
```

Migration이 있는 경우:

```text
migrations/
```

서명된 경우:

```text
META-INF/
  ableops-signature.json
```

### PKG-002

위 허용 목록에 없는 최상위 항목은 설치 단계에서 거부해야 한다.

---

# 7. Manifest 표준

## 7.1 위치

패키지 최상위:

```text
manifest.yaml
```

---

## 7.2 기본 형식

```yaml
apiVersion: ableops.io/extension/v1

id: ableops-example
name: AbleOps Example Extension
version: 1.0.0
description: Example Extension
publisher: AbleOps

requires:
  host:
    api: ">=1.0.0 <2.0.0"

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

# 8. Manifest 식별자

### MAN-001

`id`는 제품군 전체에서 가능한 한 전역적으로 유일해야 한다.

권장 형식:

```text
ableops-<domain>-<feature>
```

예:

```text
ableops-kafka-mcp
ableops-ai-chat
ableops-skill-builder
```

---

# 9. Manifest 버전

### MAN-002

Extension 버전은 Semantic Versioning을 권장한다.

```text
MAJOR.MINOR.PATCH
```

예:

```text
1.2.3
```

### MAN-003

다음 버전은 일치해야 한다.

```text
manifest.yaml version
Extension Runtime version
Package filename version
관리 UI 표시 version
```

---

# 10. Host 호환성

Extension은 Host 호환 범위를 명시해야 한다.

제품 공통 규격에서는 다음 형태를 권장한다.

```yaml
requires:
  host:
    api: ">=1.0.0 <2.0.0"
```

제품별 추가 요구사항은 선택적으로 허용한다.

```yaml
requires:
  host:
    api: ">=1.0.0 <2.0.0"

  products:
    ableops-kafka: ">=1.8.0 <2.0.0"
```

### COMP-001

Host는 설치 전에 현재 Host 버전 또는 Extension API 버전이 요구 범위를 만족하는지 확인해야 한다.

### COMP-002

호환되지 않는 Extension은 설치 또는 기동을 거부하고 상태를 `INCOMPATIBLE`로 표시해야 한다.

---

# 11. Backend 유형

현재 표준 Backend 유형:

```yaml
backend:
  enabled: true
  kind: process
```

### RUN-001

`kind: process` Extension은 Host 프로세스와 별도 OS 프로세스로 실행한다.

### RUN-002

Extension 장애가 Host 프로세스 장애로 전파되어서는 안 된다.

---

# 12. 플랫폼별 실행파일

실행파일 경로:

```text
bin/<GOOS>-<GOARCH>/
```

예:

```text
bin/linux-amd64/
bin/linux-arm64/
bin/windows-amd64/
```

### RUN-003

Host는 현재 플랫폼과 일치하는 디렉터리의 실행파일을 선택해야 한다.

### RUN-004

비 Windows 실행파일은 실행 권한을 가져야 한다.

### RUN-005

Symbolic Link 형태의 실행파일은 허용하지 않는다.

---

# 13. 실행파일 선택 규칙

하나의 플랫폼 디렉터리에 여러 실행파일이 있을 수 있다.

권장 선택 우선순위:

```text
1. extension id와 동일한 이름
2. extension id + OS 실행 확장자
3. 명시적 entrypoint 지원 시 Manifest 선언값
4. 그 외는 오류 또는 명확한 결정 규칙 적용
```

향후 Manifest에 명시적 entrypoint를 도입하는 것을 권장한다.

예:

```yaml
backend:
  enabled: true
  kind: process
  entry: ableops-ai-extension
```

---

# 14. Extension Runtime Protocol

Process Extension은 Host와 표준 Runtime Protocol을 따라야 한다.

필수 기능:

```text
Startup Handshake
Health Check
Graceful Shutdown
Host Identity
Extension Identity
Call Authentication
Protocol Version
```

---

# 15. Startup Handshake

Extension은 기동 후 Host에 실제 Listen 주소 및 Protocol 정보를 전달해야 한다.

권장 형태:

```text
ABLEOPS_EXT_READY {json}
```

### RUN-006

Extension은 외부 네트워크가 아닌 Loopback 주소에만 Listen해야 한다.

허용:

```text
127.0.0.1
::1
localhost
```

금지:

```text
0.0.0.0
공인 IP
LAN IP
```

---

# 16. Host ↔ Extension 인증

Host와 Extension 간 호출은 인증되어야 한다.

권장 방식:

```text
Short-lived Call Token
또는
Process Startup 시 생성된 Secret
```

### SEC-001

Host의 DB Password, Kafka Password, Master Key 등의 전체 환경변수를 Extension에 상속해서는 안 된다.

### SEC-002

Extension에는 실행에 필요한 최소한의 환경변수만 전달해야 한다.

---

# 17. Capability

Extension이 Host API 기능을 사용해야 하는 경우 Capability를 선언한다.

예:

```yaml
capabilities:
  - kafka.read
  - cluster.read
```

### PERM-001

Host는 Manifest에 선언되지 않은 Capability를 Extension에 제공해서는 안 된다.

---

# 18. Permission

Extension Route 또는 기능별 권한이 필요한 경우 Permission을 선언한다.

```yaml
permissions:
  - extension.example.read
  - extension.example.manage
```

### PERM-002

Permission은 최소권한 원칙에 따라 선언한다.

---

# 19. Public Route

Extension이 Host를 통해 외부 API를 제공해야 할 경우 Route를 선언한다.

예:

```yaml
routes:
  - path: /api/example
    permission: extension.example.read
```

### ROUTE-001

Manifest에 선언되지 않은 Route는 Host Public Proxy를 통해 노출해서는 안 된다.

### ROUTE-002

Host 내부 통신용 API는 Public Route로 선언하지 않는 것을 원칙으로 한다.

---

# 20. Frontend Extension

Frontend가 필요한 경우:

```yaml
frontend:
  enabled: true
  entry: web/index.html
```

### UI-001

Frontend 자산은 `web/` 아래에 위치해야 한다.

### UI-002

Extension Frontend는 Host가 제공하는 인증·권한·테마·Navigation 계약을 따라야 한다.

---

# 21. Menu Contribution

Extension이 Host 메뉴를 추가할 경우 Manifest에 선언한다.

예:

```yaml
menus:
  - id: example
    label: Example
    path: /extensions/example
```

### UI-003

메뉴는 설치 여부와 Enable 상태에 따라 동적으로 표시되어야 한다.

---

# 22. Extension Lifecycle

공통 Lifecycle 상태는 다음을 권장한다.

```text
DISCOVERED
INSTALLED
STARTING
RUNNING
STOPPING
STOPPED
DEGRADED
FAILED
INCOMPATIBLE
UPDATING
ROLLING_BACK
UNINSTALLING
```

상태 전이 예:

```text
INSTALLED
   ↓
STARTING
   ↓
RUNNING
   ↓
STOPPING
   ↓
STOPPED
```

장애:

```text
STARTING → FAILED
RUNNING  → DEGRADED
```

호환성:

```text
INSTALLED → INCOMPATIBLE
```

---

# 23. Runtime State와 Enabled State 분리

Extension의 **현재 프로세스 상태**와 **운영 사용 여부**는 서로 다른 개념이다.

예:

```text
state = STOPPED
enabled = true
```

의미:

```text
사용하도록 설정되어 있으나 현재 프로세스가 정지됨
```

또는:

```text
state = STOPPED
enabled = false
```

의미:

```text
운영자가 사용하지 않도록 Disable함
```

### LIFE-001

`state`와 `enabled`는 별도 필드로 관리해야 한다.

### LIFE-002

`enabled` 값은 Host 재기동 후에도 유지되어야 한다.

---

# 24. Enable

Enable 동작:

```text
enabled=true 저장
        ↓
호환성 확인
        ↓
현재 STOPPED이면 Start
```

### LIFE-003

Enable은 단순 UI 상태가 아니라 영속 운영 상태여야 한다.

---

# 25. Disable

Disable 동작:

```text
enabled=false 영속화
        ↓
현재 RUNNING이면 Stop
```

### LIFE-004

Disable 상태의 Extension은 Host 재기동 시 자동 기동하지 않아야 한다.

---

# 26. Host Startup 복구

Host 기동 시 Extension Manager는 설치된 Extension을 스캔해야 한다.

동작:

```text
설치 목록 확인
       ↓
Manifest 검증
       ↓
호환성 검증
       ↓
enabled 확인
       ↓
enabled=true → Start
enabled=false → STOPPED 유지
```

---

# 27. 설치 절차

표준 설치 절차:

```text
Upload
 ↓
Package Size Check
 ↓
ZIP Validation
 ↓
Entry Security Validation
 ↓
Signature Verification
 ↓
Manifest Parse
 ↓
Manifest Contract Validation
 ↓
Compatibility Check
 ↓
Permission/Capability Validation
 ↓
Extract to Temporary Directory
 ↓
Binary Validation
 ↓
Atomic Install
 ↓
Persist Install Metadata
 ↓
Start if enabled
 ↓
Audit
```

---

# 28. Temporary Install

### INST-001

패키지를 최종 설치 디렉터리에 직접 해제해서는 안 된다.

권장:

```text
extensions/.staging/<id>-<uuid>/
```

검증 완료 후:

```text
atomic rename
```

으로 최종 경로에 반영한다.

---

# 29. 설치 디렉터리

권장 구조:

```text
extensions/
  <extension-id>/
    current/
```

또는 버전 기반 구조:

```text
extensions/
  <extension-id>/
    versions/
      1.0.0/
      1.1.0/
    current -> 1.1.0
```

단, Symlink 자체를 설치 계약에서 금지하는 운영체제 정책이 있을 경우 current 포인터는 DB 또는 metadata로 관리한다.

---

# 30. Persistent State 저장

Extension 운영 상태는 Extension 배포 디렉터리 밖에 보관하는 것을 권장한다.

예:

```text
extensions/
  .state/
    <extension-id>.json
```

이유:

```text
Update 시 Extension 디렉터리 교체
Rollback 시 Version 디렉터리 교체
재설치
```

과 관계없이 운영 상태를 유지하기 위함이다.

### LIFE-005

Update/Rollback 과정에서 `enabled` 상태가 유실되어서는 안 된다.

---

# 31. Update

Update는 기존 Extension을 새 패키지로 교체하는 작업이다.

표준 절차:

```text
신규 패키지 검증
 ↓
현재 버전 확인
 ↓
Update 가능 여부 판단
 ↓
신규 버전 staging 설치
 ↓
현재 Extension Stop
 ↓
신규 버전 Activate
 ↓
Start
 ↓
Health Verification
 ↓
성공 → 이전 버전 보관
실패 → Rollback
```

### UPDATE-001

신규 패키지 검증은 기존 Extension Stop 전에 최대한 수행해야 한다.

### UPDATE-002

호환성 검증 실패 패키지로 Update해서는 안 된다.

---

# 32. Version 정책

다음 정책을 권장한다.

```text
Upgrade:
1.0.0 → 1.1.0 허용

Same Version:
1.1.0 → 1.1.0 기본 거부
관리자 Force 옵션에서만 허용 가능

Downgrade:
1.1.0 → 1.0.0 기본 Update 경로에서는 거부
Rollback 기능으로 수행
```

---

# 33. Rollback

Rollback은 이전에 정상 설치되었던 Version으로 복구하는 기능이다.

표준 절차:

```text
현재 Extension Stop
 ↓
이전 Version Activate
 ↓
Start
 ↓
Health Verification
 ↓
Audit
```

### ROLLBACK-001

Rollback 대상으로 검증되지 않은 임의 패키지를 사용해서는 안 된다.

### ROLLBACK-002

Rollback 대상은 과거 성공적으로 설치되었던 Version이어야 한다.

---

# 34. Migration과 Rollback

DB Migration이 포함된 Extension은 Rollback 시 특별한 정책이 필요하다.

Manifest에 향후 다음과 같은 정보를 추가하는 것을 권장한다.

```yaml
migrations:
  enabled: true
  rollbackSupported: false
```

### MIG-001

비가역 Migration이 적용된 경우 단순 Binary Rollback을 자동 수행해서는 안 된다.

---

# 35. Uninstall

Uninstall 표준 절차:

```text
Disable
 ↓
Stop
 ↓
사용 중 여부 확인
 ↓
Extension Files 제거
 ↓
Metadata 제거 또는 Archived 처리
 ↓
Audit
```

### UNINST-001

기본적으로 Uninstall이 Extension 생성 데이터까지 자동 삭제해서는 안 된다.

데이터 삭제는 별도 명시적 작업으로 분리하는 것을 권장한다.

---

# 36. Start

Start 가능 조건:

```text
Installed
Compatible
Enabled
Platform Binary 존재
Signature Policy 만족
필수 설정 만족
```

### LIFE-006

조건을 만족하지 못하면 Start를 수행하지 않고 명확한 상태와 오류 사유를 기록해야 한다.

---

# 37. Stop

Stop은 Graceful Shutdown을 우선한다.

권장:

```text
Stop Request
 ↓
Grace Period
 ↓
정상 종료 확인
 ↓
Timeout 시 Force Kill
```

---

# 38. Restart

Restart:

```text
Stop
 ↓
Start
```

단, `enabled=false`인 Extension에 Restart를 허용할지 정책을 명확히 해야 한다.

권장:

```text
enabled=false → Restart 거부
```

---

# 39. Health Check

Extension은 표준 Health Endpoint를 제공해야 한다.

권장:

```text
/health
```

응답 예:

```json
{
  "status": "ok"
}
```

Host는 다음을 관리한다.

```text
Health Interval
Health Timeout
Consecutive Failure Count
Last Healthy Time
Last Error
```

---

# 40. DEGRADED

일시적인 Health 실패가 즉시 `FAILED`를 의미하지는 않는다.

예:

```text
RUNNING
 ↓
Health 연속 실패
 ↓
DEGRADED
```

Health가 회복되면:

```text
DEGRADED → RUNNING
```

---

# 41. Crash Restart

Extension Process가 비정상 종료된 경우 Host는 제한적으로 자동 Restart할 수 있다.

권장:

```text
Exponential Backoff
Crash Window
Restart Limit
```

예:

```text
1 sec
2 sec
4 sec
8 sec
...
max 30 sec
```

### LIFE-007

Crash Loop 발생 시 무한 Restart해서는 안 된다.

상태:

```text
FAILED
```

로 전환하고 운영자 개입을 요구해야 한다.

---

# 42. INCOMPATIBLE

다음 경우 `INCOMPATIBLE` 상태를 사용한다.

```text
Host API Version 불일치
Runtime Protocol Version 불일치
필수 Host Capability 미지원
Manifest API Version 미지원
```

`INCOMPATIBLE`은 일반 `FAILED`와 구분한다.

이유:

```text
Restart로 해결되지 않기 때문
```

---

# 43. Package Security

Host는 Extension 패키지를 신뢰할 수 없는 입력으로 취급해야 한다.

필수 방어:

```text
ZIP Slip
Absolute Path
Drive Path
UNC Path
Backslash Path
..
Symbolic Link
Duplicate Entry
Case-insensitive Duplicate
ZIP Bomb
Huge File
Huge Entry Count
Executable Outside bin/
Unknown Top-level
```

---

# 44. Package Limits

제품별 설정으로 다음 제한을 둘 수 있다.

```text
Max Package Size
Max Entry Count
Max File Size
Max Expanded Size
Max Compression Ratio
```

기본값은 Host 공통 라이브러리에서 제공하는 것을 권장한다.

---

# 45. Secret Protection

Extension 패키지에 다음을 포함해서는 안 된다.

```text
Password
API Key
Access Token
Private Key
Credential File
.env
개인 인증 저장소
운영 DB 접속정보
Host Master Key
```

### SEC-003

Extension 설치 패키지는 Secret Delivery 수단으로 사용해서는 안 된다.

---

# 46. Extension Configuration

운영 설정은 패키지와 분리한다.

권장:

```text
Host Extension Configuration Store
```

예:

```text
DB
Secret Store
Encrypted Config Store
```

Manifest에는 Secret 값을 넣지 않는다.

---

# 47. Signature

표준 서명 파일:

```text
META-INF/ableops-signature.json
```

표준 알고리즘:

```text
Ed25519
```

정책:

```text
off
warn
require
```

---

# 48. Signature 정책

```text
off
  unsigned         허용
  invalid signed   검증 생략 가능

warn
  unsigned         허용 + 경고
  invalid signed   거부
  valid signed     허용

require
  unsigned         거부
  invalid signed   거부
  valid signed     허용
```

### SIGN-001

`warn`에서 잘못된 서명이 있는 패키지를 단순 경고로 설치해서는 안 된다.

---

# 49. Trusted Publisher

Host는 Trusted Publisher를 관리할 수 있어야 한다.

권장 데이터:

```text
keyId
publisher
algorithm
publicKey
enabled
createdAt
updatedAt
```

신뢰 판정 기준은 Publisher 이름이 아니라:

```text
keyId + Public Key
```

이어야 한다.

---

# 50. Signature Downgrade 방지

기존 설치 Version이 서명된 경우 신규 Update가 미서명으로 내려가는 것을 제한해야 한다.

### SIGN-002

서명된 Extension을 미서명 Extension으로 Update하는 경우 별도 정책 또는 관리자 승인을 요구해야 한다.

---

# 51. Extension 관리 API

각 AbleOps 제품은 최소한 다음 논리 API를 제공해야 한다.

```text
GET    /api/extensions
GET    /api/extensions/{id}

POST   /api/extensions/install

POST   /api/extensions/{id}/start
POST   /api/extensions/{id}/stop
POST   /api/extensions/{id}/restart

POST   /api/extensions/{id}/enable
POST   /api/extensions/{id}/disable

POST   /api/extensions/{id}/update
POST   /api/extensions/{id}/rollback

DELETE /api/extensions/{id}
```

실제 URL 구조는 제품별 API 표준에 맞게 조정할 수 있으나 의미 계약은 유지해야 한다.

---

# 52. Extension 조회 정보

Extension 상세 조회는 최소 다음 정보를 제공해야 한다.

```json
{
  "id": "ableops-example",
  "name": "AbleOps Example",
  "version": "1.0.0",
  "publisher": "AbleOps",

  "enabled": true,
  "state": "RUNNING",

  "compatible": true,

  "installedAt": "...",
  "updatedAt": "...",

  "lastStartedAt": "...",
  "lastStoppedAt": "...",
  "lastHealthyAt": "...",

  "lastError": null
}
```

---

# 53. UI 요구사항

모든 AbleOps 제품은 가능한 한 동일한 UX를 제공한다.

기본 메뉴:

```text
시스템 관리
  └─ 확장 기능
```

---

# 54. Extension 목록 화면

표시 권장 항목:

```text
이름
ID
Version
Publisher
Enabled
State
Compatibility
Signature
설치일
최근 오류
```

---

# 55. Extension 상세 화면

권장 탭:

```text
개요
상태
Manifest
설정
권한
버전
로그
감사 이력
```

---

# 56. 관리 동작

UI에서 다음 동작을 제공한다.

```text
설치
Enable
Disable
Start
Stop
Restart
Update
Rollback
Uninstall
```

각 작업은 현재 상태에 따라 허용/비허용을 명확히 표시해야 한다.

---

# 57. 사용자 확인이 필요한 작업

다음 작업은 확인 Dialog를 권장한다.

```text
Disable
Stop
Update
Rollback
Uninstall
```

특히 데이터 또는 서비스 영향 가능성을 설명해야 한다.

---

# 58. Audit

다음 작업은 반드시 감사 이력을 남겨야 한다.

```text
INSTALL
UPDATE
ROLLBACK
UNINSTALL

ENABLE
DISABLE

START
STOP
RESTART

SIGNATURE_REJECT
COMPATIBILITY_REJECT
START_FAILED
CRASH_LOOP
```

---

# 59. Audit 항목

권장 필드:

```text
timestamp
actor
extensionId
extensionVersion
action
result
previousState
newState
reason
requestId
```

Secret은 감사 로그에 기록하지 않는다.

---

# 60. 로그

Extension stdout/stderr는 Host가 수집할 수 있다.

원칙:

```text
Protocol stdout와 일반 로그를 혼합하지 않음
Secret Redaction
Line Length 제한
Retention 적용
Extension ID 식별
```

---

# 61. 오류 정보

`lastError`에는 운영자가 조치할 수 있는 수준의 오류를 제공한다.

예:

```text
이 플랫폼용 실행파일 없음
Host 버전 호환되지 않음
Startup handshake timeout
Health check failed
Signature verification failed
Crash loop detected
```

Secret이 포함될 가능성이 있는 원문 오류는 Redaction 후 저장한다.

---

# 62. 제품별 Extension Capability

공통 Extension Host 위에 각 제품은 자체 Capability를 정의할 수 있다.

예:

AbleOps Kafka:

```text
kafka.read
kafka.manage
cluster.read
```

AbleOps Flink:

```text
flink.job.read
flink.job.manage
flink.savepoint
```

AbleOps MCP Gateway:

```text
mcp.server.read
mcp.server.manage
mcp.tool.invoke
```

단, Capability 전달·검증 방식은 공통 규격을 따라야 한다.

---

# 63. 공통 SDK

AbleOps 제품군은 가능한 한 공통 Extension SDK를 사용한다.

권장 모듈:

```text
github.com/heartblast/ableops-sdk/extension/v1
```

SDK 책임:

```text
Manifest Parsing
Manifest Validation
Runtime Protocol
Startup Handshake
Health
Call Token
Graceful Shutdown
Host Context
Route Contract
Test Kit
```

---

# 64. 공통 Packaging Tool

장기적으로 Extension 프로젝트별 `build-extension.sh` 복제를 최소화하는 것을 권장한다.

공통 도구 예:

```text
ableops extension build
ableops extension verify
ableops extension sign
ableops extension inspect
```

예:

```bash
ableops extension build \
  --manifest internal/extension/manifest.yaml \
  --command ./cmd/my-extension \
  --targets linux/amd64,windows/amd64
```

---

# 65. 공통 Validator

다음 검증 로직은 제품별로 중복 구현하지 않고 공통 라이브러리화하는 것을 권장한다.

```text
ZIP Validation
Entry Path Validation
Manifest Validation
Version Validation
Compatibility Validation
Signature Validation
Package Limits
Binary Resolution
```

---

# 66. 제품별 구현 금지사항

각 AbleOps 제품이 자체적으로 다음을 변형 구현해서는 안 된다.

```text
서로 다른 .ableops-ext 구조
서로 다른 Manifest 필수 필드
서로 다른 플랫폼 경로 규칙
서로 다른 Signature 파일 위치
서로 다른 Lifecycle 의미
서로 다른 Enable/Disable 의미
서로 다른 기본 보안 검증 규칙
```

제품 고유 기능은 Extension capability 또는 optional Manifest 영역으로 확장해야 한다.

---

# 67. API Versioning

본 규격은 다음 API Version을 기준으로 한다.

```text
ableops.io/extension/v1
```

Breaking Change가 필요한 경우:

```text
ableops.io/extension/v2
```

와 같이 새 버전을 도입한다.

기존 v1 Extension의 호환성은 가능한 한 유지한다.

---

# 68. Manifest 확장 규칙

제품별 설정이 필요한 경우 공통 필드를 변형하지 말고 Namespace 확장을 권장한다.

예:

```yaml
x-ableops-kafka:
  feature: value
```

또는 표준화된 `extensions` 영역을 향후 도입할 수 있다.

---

# 69. Install State와 Runtime State

제품 구현 시 다음 정보를 분리한다.

```text
Installed Version
Active Version
Enabled
Runtime State
Health State
Compatibility State
Signature State
```

하나의 `status` 문자열에 모든 의미를 넣지 않는다.

---

# 70. 권장 내부 데이터 모델

예:

```text
ExtensionRecord
--------------
id
name
publisher
installedVersion
activeVersion
enabled
runtimeState
compatible
signatureState
installPath
installedAt
updatedAt
lastStartedAt
lastStoppedAt
lastHealthyAt
lastError
```

Version History:

```text
ExtensionVersion
----------------
extensionId
version
installPath
signatureState
installedAt
active
rollbackAvailable
```

---

# 71. 동시성 제어

Extension 관리 작업은 동일 Extension에 대해 직렬화해야 한다.

동시에 다음이 실행되어서는 안 된다.

```text
Update + Start
Rollback + Uninstall
Enable + Disable
Start + Stop
```

### OPS-001

Extension ID 단위 Lock 또는 Transaction Guard를 사용해야 한다.

---

# 72. Idempotency

가능한 작업은 Idempotent하게 설계한다.

예:

```text
이미 enabled=true 상태에서 Enable
→ 성공 또는 no-op

이미 STOPPED 상태에서 Stop
→ 성공 또는 no-op
```

불필요한 오류로 처리하지 않는 것을 권장한다.

---

# 73. 장애 격리

Extension 하나의 실패가 다음에 영향을 주어서는 안 된다.

```text
Host 전체
다른 Extension
관리 UI
Extension Manager
```

### OPS-002

Extension 장애는 Extension ID 단위로 격리해야 한다.

---

# 74. 설치 실패 복구

설치가 중간 실패한 경우:

```text
최종 설치 디렉터리 오염 금지
기존 Version 유지
staging 정리
실패 이력 기록
```

---

# 75. Update 실패 복구

Update 실패 시 원칙:

```text
기존 정상 Version으로 복구
enabled 상태 유지
실패 원인 기록
```

---

# 76. Host 재기동 중 복구

Host가 Extension Update 도중 종료될 수 있다.

따라서 다음 상태를 복구할 수 있어야 한다.

```text
staging 존재
UPDATING 상태
ROLLING_BACK 상태
새 Version 설치 완료/Activation 미완료
```

제품 수준에서는 atomic operation 또는 recovery marker를 사용한다.

---

# 77. Extension 설정 관리

Extension 설정은 설치 패키지와 별도로 관리한다.

권장 API:

```text
GET /api/extensions/{id}/config
PUT /api/extensions/{id}/config
```

민감 설정은 Secret Store를 이용한다.

---

# 78. Config Schema

향후 Extension이 설정 Schema를 제공하도록 확장할 수 있다.

예:

```yaml
config:
  schema: config/schema.json
```

이를 이용하면 Host가 공통 설정 UI를 자동 생성할 수 있다.

---

# 79. Extension Dependencies

향후 Extension 간 Dependency가 필요한 경우 Manifest에서 명시적으로 선언한다.

예:

```yaml
dependencies:
  - id: ableops-common-auth
    version: ">=1.0.0 <2.0.0"
```

현재는 Extension 간 암묵적 의존성을 만들지 않는 것을 권장한다.

---

# 80. 설치 전 체크리스트

Host는 설치 전에 다음을 확인해야 한다.

```text
[ ] 파일 크기 제한
[ ] ZIP 형식
[ ] Entry 수 제한
[ ] Path traversal 없음
[ ] Symlink 없음
[ ] 허용 Top-level만 존재
[ ] Manifest 존재
[ ] Manifest Parse 성공
[ ] Manifest API 지원
[ ] ID 유효
[ ] Version 유효
[ ] Host 호환
[ ] Signature 정책 만족
[ ] 현재 플랫폼 binary 존재
[ ] 실행파일 안전성
[ ] Capability/Permission 유효
```

---

# 81. 설치 후 체크리스트

```text
[ ] 설치 Metadata 저장
[ ] enabled 기본값 적용
[ ] Start 결과 확인
[ ] Health 확인
[ ] 상태 업데이트
[ ] 감사 로그 기록
[ ] UI 반영
```

---

# 82. CI 요구사항

Extension 프로젝트 CI:

```text
Unit Test
Manifest Contract Test
Build
Race Test
Package Build
Package Verify
Dependency Scan
```

Host 제품 CI:

```text
Package Security Test
Install Test
Start/Stop Test
Enable/Disable Test
Update Test
Rollback Test
Uninstall Test
Crash Recovery Test
Signature Policy Test
Compatibility Test
```

---

# 83. 공통 테스트 시나리오

최소 E2E:

```text
1. 정상 패키지 설치
2. 잘못된 Manifest 거부
3. 호환되지 않는 Version 거부
4. 잘못된 ZIP 거부
5. Zip Slip 거부
6. Symlink 거부
7. 실행파일 없음 처리
8. Enable/Disable 영속성
9. Start/Stop
10. Crash Restart
11. Crash Loop 차단
12. Update 성공
13. Update 실패 후 복구
14. Rollback
15. Uninstall
16. unsigned warn
17. unsigned require 거부
18. invalid signature 거부
```

---

# 84. 관리 화면 공통 UX

AbleOps 제품군은 사용자가 제품을 바꾸어도 동일한 Extension 관리 경험을 제공하는 것이 목표이다.

권장:

```text
[확장 기능]

설치됨
┌───────────────────────────────────────────────┐
│ Name │ Version │ Enabled │ State │ Actions    │
└───────────────────────────────────────────────┘
```

Actions:

```text
상세
Enable / Disable
Start / Stop
Update
Rollback
삭제
```

---

# 85. 공통 구현 계층

권장 아키텍처:

```text
              AbleOps Product
                    │
        ┌───────────▼───────────┐
        │ Extension Manager     │
        ├───────────────────────┤
        │ Install               │
        │ Lifecycle             │
        │ Update / Rollback     │
        │ State                 │
        │ Audit                 │
        └───────────┬───────────┘
                    │
        ┌───────────▼───────────┐
        │ Extension Host        │
        ├───────────────────────┤
        │ Runtime Protocol      │
        │ Process Supervisor    │
        │ Route Proxy           │
        │ Frontend Assets       │
        └───────────┬───────────┘
                    │
        ┌───────────▼───────────┐
        │ Common SDK / Contract │
        └───────────┬───────────┘
                    │
             .ableops-ext
```

---

# 86. 공통 모듈화 권고

장기적으로 다음 모듈을 AbleOps 공용 라이브러리로 분리하는 것을 권장한다.

```text
ableops-sdk/extension
ableops-extension-package
ableops-extension-host
ableops-extension-signing
ableops-extension-testkit
```

제품은 공통 구현 위에 다음만 추가한다.

```text
제품 Capability
제품 Permission
제품 전용 Route
제품 전용 UI Integration
```

---

# 87. 제품별 차이 허용 범위

허용:

```text
Capability 종류
Permission 종류
Extension이 호출할 Host API
Menu 위치
제품 전용 UI
제품 전용 설정
```

공통 유지:

```text
Package 형식
Manifest 기본 계약
Install
Lifecycle
Enable/Disable
Update/Rollback
Signature
Security
Audit
State model
Runtime Protocol
```

---

# 88. 표준 요구사항 요약

| ID | 요구사항 | 수준 |
|---|---|---|
| PKG-001 | 파일명은 Manifest ID/Version 기준 | 필수 |
| PKG-002 | 허용된 Top-level만 패키징 | 필수 |
| MAN-001 | 제품군 전역적으로 안정적인 Extension ID 사용 | 필수 |
| MAN-002 | Semantic Versioning 권고 | 권고 |
| MAN-003 | Version 일치 | 필수 |
| COMP-001 | 설치 전 Host 호환성 검증 | 필수 |
| COMP-002 | 비호환 상태를 INCOMPATIBLE로 분리 | 필수 |
| RUN-001 | Process Extension 별도 프로세스 실행 | 필수 |
| RUN-002 | Extension 장애 격리 | 필수 |
| RUN-003 | 플랫폼별 Binary 선택 | 필수 |
| RUN-004 | Unix 실행 권한 | 필수 |
| RUN-005 | Symlink Binary 금지 | 필수 |
| RUN-006 | Loopback Listener만 허용 | 필수 |
| SEC-001 | Host 전체 환경변수 상속 금지 | 필수 |
| SEC-002 | 최소 환경만 전달 | 필수 |
| SEC-003 | Package를 Secret 전달수단으로 사용 금지 | 필수 |
| PERM-001 | 선언하지 않은 Capability 제공 금지 | 필수 |
| PERM-002 | 최소권한 | 필수 |
| ROUTE-001 | 선언 Route만 공개 | 필수 |
| ROUTE-002 | 내부 API Public Route 노출 금지 | 권고 |
| LIFE-001 | state/enabled 분리 | 필수 |
| LIFE-002 | enabled 영속화 | 필수 |
| LIFE-003 | Enable 시 필요하면 Start | 필수 |
| LIFE-004 | Disable 시 재기동 자동 Start 금지 | 필수 |
| LIFE-005 | Update/Rollback 후 enabled 유지 | 필수 |
| LIFE-006 | Start 전 필수조건 검증 | 필수 |
| LIFE-007 | Crash Loop 제한 | 필수 |
| INST-001 | Staging 후 Atomic Install | 필수 |
| UPDATE-001 | 기존 Stop 전 신규 패키지 최대 검증 | 필수 |
| UPDATE-002 | 비호환 Update 금지 | 필수 |
| ROLLBACK-001 | 임의 미검증 패키지 Rollback 금지 | 필수 |
| ROLLBACK-002 | 설치 이력 기반 Rollback | 필수 |
| MIG-001 | 비가역 Migration의 자동 Rollback 금지 | 필수 |
| SIGN-001 | warn에서도 invalid signature 거부 | 필수 |
| SIGN-002 | Signature Downgrade 통제 | 필수 |
| OPS-001 | Extension 단위 동시성 제어 | 필수 |
| OPS-002 | Extension 장애 격리 | 필수 |

---

# 89. 최종 표준 정의

AbleOps 제품군의 Extension 기능은 다음 세 가지를 공통 플랫폼 계약으로 본다.

```text
1. Extension Package Contract
2. Extension Runtime Contract
3. Extension Management Contract
```

전체 구조:

```text
                    .ableops-ext
                         │
            ┌────────────▼────────────┐
            │ Package Contract        │
            │ Manifest / Bin / Web    │
            │ Migration / Signature   │
            └────────────┬────────────┘
                         │
            ┌────────────▼────────────┐
            │ Runtime Contract        │
            │ Process / Handshake     │
            │ Health / Auth / Stop    │
            └────────────┬────────────┘
                         │
            ┌────────────▼────────────┐
            │ Management Contract     │
            │ Install / Enable        │
            │ Update / Rollback       │
            │ State / Audit / UI      │
            └────────────┬────────────┘
                         │
                 AbleOps Product
```

---

# 90. 적용 원칙

향후 새로운 AbleOps 제품에서 “확장기능” 기능을 구현할 경우 다음 원칙을 적용한다.

> 새로운 Extension 관리체계를 별도로 설계하지 않는다.

대신:

```text
AbleOps Extension 공통 규격
        +
제품별 Capability / Permission / Integration
```

방식으로 구현한다.

즉:

```text
ableops-kafka
ableops-flink
ableops-mcp-gateway
향후 AbleOps 제품
```

은 Extension 설치·관리 엔진을 가능한 한 공유하고, 제품별 차이는 Extension이 사용할 수 있는 기능 범위에서만 발생하도록 한다.

이렇게 해야 AbleOps 제품군 전체에서 동일한 `.ableops-ext` 생태계, 동일한 관리 UX, 동일한 보안 정책, 동일한 Lifecycle 모델을 유지할 수 있다.
