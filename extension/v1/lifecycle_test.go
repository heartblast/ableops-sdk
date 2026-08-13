package extensionv1

// 생애주기 상태 정규화·전이 허용표 테스트.

import "testing"

func TestState_Valid_IsHealthy(t *testing.T) {
	if len(AllStates()) != 9 {
		t.Fatalf("상태는 9종이어야 한다: %d종", len(AllStates()))
	}
	for _, s := range AllStates() {
		if !s.Valid() {
			t.Errorf("%s.Valid() = false", s)
		}
	}
	if State("RUNNING_MAYBE").Valid() {
		t.Error("알 수 없는 상태를 유효로 판정했다")
	}
	// IsHealthy 는 RUNNING 만 true — DEGRADED 를 정상으로 세면 장애가 화면에서 사라진다.
	for _, s := range AllStates() {
		want := s == StateRunning
		if s.IsHealthy() != want {
			t.Errorf("%s.IsHealthy() = %v, 기대 %v", s, s.IsHealthy(), want)
		}
	}
}

func TestNormalizeState_알수없는값은_FAILED(t *testing.T) {
	if got := NormalizeState("running"); got != StateRunning {
		t.Errorf("소문자 정규화 실패: %v", got)
	}
	if got := NormalizeState("  Degraded  "); got != StateDegraded {
		t.Errorf("공백/대소문자 정규화 실패: %v", got)
	}
	for _, in := range []string{"", "  ", "정상", "OK", "ACTIVE", "UNKNOWN_STATE"} {
		if got := NormalizeState(in); got != StateFailed {
			t.Errorf("NormalizeState(%q) = %v, 기대 FAILED(상태 불명은 정상으로 읽지 않는다)", in, got)
		}
	}
}

func TestInstallPhase(t *testing.T) {
	phases := []InstallPhase{PhaseStaging, PhaseValidating, PhaseInstalling, PhaseStarting, PhaseVerifying, PhaseInstalled, PhaseRollback, PhaseFailed}
	for _, p := range phases {
		if !p.Valid() {
			t.Errorf("%s.Valid() = false", p)
		}
	}
	if InstallPhase("DONE").Valid() {
		t.Error("알 수 없는 설치 단계를 유효로 판정했다")
	}
	if !PhaseInstalled.IsTerminal() || !PhaseFailed.IsTerminal() {
		t.Error("종료 단계 판정 오류")
	}
	if PhaseStaging.IsTerminal() || PhaseRollback.IsTerminal() {
		t.Error("진행 단계를 종료로 판정했다")
	}
}

func TestCanTransition(t *testing.T) {
	allowed := [][2]State{
		{StateNotInstalled, StateInstalled},
		{StateInstalled, StateStarting},
		{StateStarting, StateRunning},
		{StateStarting, StateDegraded},
		{StateStarting, StateFailed},
		{StateRunning, StateDisabled},
		{StateRunning, StateDegraded},
		{StateDegraded, StateRunning},
		{StateDisabled, StateStarting},
		{StateFailed, StateStarting},
		{StateIncompatible, StateUpdating},
		{StateUpdating, StateRunning},
		{StateRunning, StateRunning}, // 동일 상태 재보고는 허용
	}
	for _, tc := range allowed {
		if !CanTransition(tc[0], tc[1]) {
			t.Errorf("CanTransition(%s, %s) = false (허용해야 함)", tc[0], tc[1])
		}
	}

	denied := [][2]State{
		{StateNotInstalled, StateRunning},  // 설치 없이 실행 불가
		{StateNotInstalled, StateStarting}, //
		{StateFailed, StateRunning},        // 검증 없이 정상으로 점프 금지
		{StateDisabled, StateRunning},      // 활성화는 STARTING 을 거친다
		{StateInstalled, StateRunning},     //
		{StateRunning, State("WAT")},       // 알 수 없는 상태
		{State("WAT"), StateRunning},       //
		{StateNotInstalled, StateDisabled}, // 미설치를 비활성으로 만들 수 없다
	}
	for _, tc := range denied {
		if CanTransition(tc[0], tc[1]) {
			t.Errorf("CanTransition(%s, %s) = true (거부해야 함)", tc[0], tc[1])
		}
	}
}
