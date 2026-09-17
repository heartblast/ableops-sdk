---
description: 검증 수준(fast/full/release)을 고르고 scripts/verify.sh 를 실행한다. 어떤 범위로 검증할지 판단이 필요할 때, 그리고 작업을 "완료"라고 보고하기 직전에 사용한다.
---

# verify — 검증 수준 선택

## 목적

**검증을 없애지 않고 시점을 옮긴다.** 개발 중에는 fast, 작업 종료 시 full 1회, 태그 직전 release 1회.
매 수정마다 `go test ./...` 를 통째로 반복하지 않기 위한 라우터다. 단계 내용은 [검증.md](../context/검증.md) §2.

## 수준 선택

| 상황 | 수준 |
| --- | --- |
| 코드 한 덩어리를 방금 고쳤다 | `bash scripts/verify.sh fast` |
| **작업을 끝냈다 / PR 을 올린다** / 공개 심볼·go.mod·CHANGELOG·게이트 테스트를 건드렸다 | `bash scripts/verify.sh full` |
| 릴리스 태그를 붙인다 | `bash scripts/verify.sh release` |

`$ARGUMENTS` 에 수준이 오면 그대로 쓴다. 없으면 위 표로 고르고, 애매하면 **한 단계 위**.

## 절대 규칙

- **완료 보고 전 `full` 1회는 필수다.** fast 통과만으로 "완료"라고 말하지 않는다.
- 실패를 통과로 보고하지 않는다. 실행하지 않은 단계는 **"미검증"** 으로 명시한다.
- 실패 시 전체 로그를 붙이지 말고 **핵심 오류 라인**과 `verify.sh` 마지막 결과 표만 요약한다.
- `frontend/v1` 변경은 여기서 검증되지 않는다 — "Core `npm run check:bridge` 미검증" 이라고 쓴다.

## 실패 시

[검증.md](../context/검증.md) §4 의 실패 모드를 먼저 대조한다(consumer 캐시 · 골든 · CHANGELOG 제목 · CRLF · GOWORK).
게이트 테스트를 **약화·삭제·SKIP 으로 우회하지 않는다.** 게이트가 깨졌으면 코드가 틀린 것이다.
