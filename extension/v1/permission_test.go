package extensionv1

// 권한 키 조립·검증·파싱(왕복) 테스트.

import "testing"

func TestPermissionKey_왕복(t *testing.T) {
	cases := []struct{ extID, action string }{
		{"sample", "view"},
		{"sample", "manage"},
		{"flink-sql", "job.run"},
		{"data-workspace", "worksheet-edit"},
	}
	for _, tc := range cases {
		key := PermissionKey(tc.extID, tc.action)
		id, action, ok := ParsePermissionKey(key)
		if !ok {
			t.Errorf("ParsePermissionKey(%q) 실패", key)
			continue
		}
		if id != tc.extID || action != tc.action {
			t.Errorf("왕복 불일치: %q → (%q, %q), 기대 (%q, %q)", key, id, action, tc.extID, tc.action)
		}
		if !ValidPermissionKey(tc.extID, key) {
			t.Errorf("ValidPermissionKey(%q, %q) = false", tc.extID, key)
		}
	}
}

func TestParsePermissionKey_거부(t *testing.T) {
	for _, key := range []string{
		"",
		"view",
		"ext.sample",            // action 없음
		"extension.sample.view", // 접두 오타
		"EXT.sample.view",       // 대문자 접두
		"ext.Sample.view",       // ID 대문자
		"ext.sample-.view",      // ID 형식 위반
		"ext.ab.view",           // ID 너무 짧음
		"ext.sample.View",       // action 대문자
		"ext.sample.",           // action 없음
		"topic.create",          // Core 권한
	} {
		if _, _, ok := ParsePermissionKey(key); ok {
			t.Errorf("ParsePermissionKey(%q) 가 통과했다(거부해야 함)", key)
		}
	}
}

func TestValidPermissionKey_다른Extension_거부(t *testing.T) {
	if ValidPermissionKey("sample", "ext.other.view") {
		t.Error("다른 Extension 의 권한 키를 통과시켰다")
	}
	if ValidPermissionKey("sample", "topic.create") {
		t.Error("Core 권한 키를 Extension 권한으로 통과시켰다")
	}
}

func TestValidExtensionID(t *testing.T) {
	valid := []string{"sample", "flink-sql", "abc", "a1b2c3", "data-workspace-x1"}
	for _, id := range valid {
		if !ValidExtensionID(id) {
			t.Errorf("ValidExtensionID(%q) = false (통과해야 함)", id)
		}
	}
	invalid := []string{"", "ab", "Sample", "1sample", "sample-", "-sample", "sample_x", "sample.x", "샘플"}
	for _, id := range invalid {
		if ValidExtensionID(id) {
			t.Errorf("ValidExtensionID(%q) = true (거부해야 함)", id)
		}
	}
}
