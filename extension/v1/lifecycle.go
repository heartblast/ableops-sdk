package extensionv1

// Extension 생애주기 — 런타임 상태 9종과 설치 트랜잭션 단계 8종.
//
// 상태는 Core(Registry)가 소유한다. Extension 이 스스로 상태를 바꾸지 않으며,
// Extension 은 Health 로 자기 건강 상태만 보고한다(extension.go 참조).

import "strings"

// State 는 Extension 의 런타임 상태다.
type State string

const (
	StateNotInstalled State = "NOT_INSTALLED" // 설치되지 않음(미설치 또는 삭제 완료)
	StateInstalled    State = "INSTALLED"     // 설치 완료, 아직 시작 전
	StateDisabled     State = "DISABLED"      // 관리자가 비활성화(라우트는 등록되고 핸들러가 503)
	StateStarting     State = "STARTING"      // 백엔드 시작 중
	StateRunning      State = "RUNNING"       // 정상 동작(요구B §4 의 검증 조건 전부 통과)
	StateDegraded     State = "DEGRADED"      // 부분 동작(예: 백엔드 정상 + 프론트 기여 실패)
	StateFailed       State = "FAILED"        // 시작·검증 실패
	StateIncompatible State = "INCOMPATIBLE"  // Core 버전 제약 불충족
	StateUpdating     State = "UPDATING"      // 버전 업데이트 진행 중
)

// InstallPhase 는 설치 트랜잭션의 진행 단계다(요구B §9·§10 — 설치 이력에 그대로 남는다).
type InstallPhase string

const (
	PhaseStaging    InstallPhase = "STAGING"    // 패키지 업로드·임시 전개
	PhaseValidating InstallPhase = "VALIDATING" // 패키지 구조·Manifest·호환성 검증
	PhaseInstalling InstallPhase = "INSTALLING" // 설치 디렉터리 배치·메타 등록·마이그레이션
	PhaseStarting   InstallPhase = "STARTING"   // 백엔드 시작
	PhaseVerifying  InstallPhase = "VERIFYING"  // Health·라우트/메뉴/권한 등록 확인
	PhaseInstalled  InstallPhase = "INSTALLED"  // 설치 성공(종료)
	PhaseRollback   InstallPhase = "ROLLBACK"   // 실패 후 되돌리는 중
	PhaseFailed     InstallPhase = "FAILED"     // 설치 실패(종료, 복구 정보 보존)
)

// Valid 는 v1 이 정의한 상태인지 판정한다.
func (s State) Valid() bool {
	switch s {
	case StateNotInstalled, StateInstalled, StateDisabled, StateStarting,
		StateRunning, StateDegraded, StateFailed, StateIncompatible, StateUpdating:
		return true
	}
	return false
}

// IsHealthy 는 "지금 정상적으로 서비스 중"인지 판정한다.
// **RUNNING 만 true** 다 — DEGRADED 는 일부만 동작하는 상태이므로 정상으로 셈하지 않는다.
func (s State) IsHealthy() bool {
	return s == StateRunning
}

// AllStates 는 v1 이 정의하는 상태 전체를 돌려준다.
func AllStates() []State {
	return []State{
		StateNotInstalled, StateInstalled, StateDisabled, StateStarting,
		StateRunning, StateDegraded, StateFailed, StateIncompatible, StateUpdating,
	}
}

// NormalizeState 는 외부(DB·API·구버전 패키지)에서 들어온 상태 문자열을 State 로 정규화한다.
//
// 공백 트림 + 대문자화 후 알려진 값이면 그대로, **알 수 없는 값은 FAILED** 로 정규화한다.
// 빈 문자열도 FAILED 다. 이유: 상태를 읽지 못했다는 것은 "정상 여부를 확인할 수 없다"는 뜻인데,
// 이를 RUNNING/INSTALLED 같은 정상 상태로 읽으면 실제로 죽은 Extension 이 화면에 정상으로 표시되고
// 관리자가 장애를 인지하지 못한다. 판정 불가는 안전한 쪽(실패)으로 접는다.
func NormalizeState(v string) State {
	s := State(strings.ToUpper(strings.TrimSpace(v)))
	if s.Valid() {
		return s
	}
	return StateFailed
}

// Valid 는 v1 이 정의한 설치 단계인지 판정한다.
func (p InstallPhase) Valid() bool {
	switch p {
	case PhaseStaging, PhaseValidating, PhaseInstalling, PhaseStarting,
		PhaseVerifying, PhaseInstalled, PhaseRollback, PhaseFailed:
		return true
	}
	return false
}

// IsTerminal 은 더 이상 진행하지 않는 종료 단계인지 판정한다.
func (p InstallPhase) IsTerminal() bool {
	return p == PhaseInstalled || p == PhaseFailed
}

// allowedTransitions 는 상태 전이 허용표다(from → 허용 to 집합).
// 표에 없는 전이는 Core 가 거부하고 그 시도 자체를 설치 이력/감사에 남긴다.
var allowedTransitions = map[State][]State{
	// 설치·재설치
	StateNotInstalled: {StateInstalled, StateFailed, StateIncompatible},
	// 설치 직후: 시작하거나, 비활성으로 두거나, 업데이트·삭제
	StateInstalled: {StateStarting, StateDisabled, StateUpdating, StateNotInstalled, StateFailed, StateIncompatible},
	// 비활성: 활성화(→STARTING), 업데이트, 삭제
	StateDisabled: {StateStarting, StateInstalled, StateUpdating, StateNotInstalled, StateFailed},
	// 시작 중: 성공(RUNNING) / 부분성공(DEGRADED) / 실패 / 시작 도중 비활성화
	StateStarting: {StateRunning, StateDegraded, StateFailed, StateDisabled},
	// 동작 중
	StateRunning: {StateDegraded, StateDisabled, StateFailed, StateUpdating, StateIncompatible},
	// 부분 동작: 회복하거나 악화하거나
	StateDegraded: {StateRunning, StateStarting, StateDisabled, StateFailed, StateUpdating},
	// 실패: 재시작·비활성·업데이트·삭제로만 빠져나간다(바로 RUNNING 으로 점프 금지 — 검증을 거쳐야 한다)
	StateFailed: {StateStarting, StateInstalled, StateDisabled, StateUpdating, StateNotInstalled},
	// 호환 불가: Core 업그레이드나 Extension 업데이트 후 재판정
	StateIncompatible: {StateUpdating, StateInstalled, StateDisabled, StateNotInstalled, StateFailed},
	// 업데이트 중
	StateUpdating: {StateInstalled, StateStarting, StateRunning, StateDegraded, StateDisabled, StateFailed, StateIncompatible},
}

// CanTransition 은 from → to 상태 전이가 허용되는지 판정한다.
// 같은 상태로의 전이(상태 재보고)는 허용한다. 알 수 없는 상태가 하나라도 있으면 거부한다.
func CanTransition(from, to State) bool {
	if !from.Valid() || !to.Valid() {
		return false
	}
	if from == to {
		return true
	}
	for _, allowed := range allowedTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}
