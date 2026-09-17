---
description: SDK 릴리스 절차. SDKVersion 상수 · CHANGELOG 제목 · git 태그 셋을 한 커밋에서 맞추고 verify.sh release 로 확인한 뒤, Core 저장소에서 할 후속 작업을 사용자에게 넘긴다.
---

# release — SDK 버전 올리기

상세와 근거는 [릴리스.md](../context/릴리스.md). 이 커맨드는 순서와 보고 형식만 정한다.

## 실행 절차

1. `git diff v<직전태그>..HEAD -- extension/v1/testdata/api-v1.golden` 으로 **Patch/Minor 를 판정**한다(`$ARGUMENTS` 에 버전이 오면 판정과 맞는지 확인만).
   `-` 줄이나 `imethod` 추가가 보이면 릴리스를 멈추고 보고한다.
2. `extension/v1/compat.go` 의 `SDKVersion` 과 `CHANGELOG.md` 맨 위 `## vX.Y.Z — YYYY-MM-DD` 를 **같은 값**으로 쓴다. `## 미출시` 내용을 옮긴다.
3. `frontend/v1` 변경이 포함됐으면 CHANGELOG 에 "Core `web/src/extensions/contract/` 재복사 필요" 를 적는다.
4. 커밋 메시지: `chore(release): vX.Y.Z — <한 줄 요약>`.
5. `bash scripts/verify.sh release` — 전부 통과해야 한다. 실패하면 태그를 만들지 않는다.
6. 태그와 push 는 **사용자 확인 후**(settings 가 ask 로 묻는다): `git tag vX.Y.Z && git push origin main vX.Y.Z`.

## 최종 보고 형식

- 버전과 SemVer 근거(골든 추가 N줄 / 삭제 0줄)
- `verify.sh release` 결과 표
- **Core 저장소에서 할 일** 체크리스트([릴리스.md](../context/릴리스.md) §3 그대로, 해당 없는 항목은 지운다)
- 실행하지 않은 것(태그·push·프록시 확인)은 "미실행" 으로 명시
