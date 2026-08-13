package extensionv1

// semver 파싱·제약 판정 테스트.

import "testing"

func TestParseSemver_정상(t *testing.T) {
	cases := []struct {
		in                  string
		major, minor, patch int
	}{
		{"1.2.3", 1, 2, 3},
		{"0.0.0", 0, 0, 0},
		{"v1.3.3", 1, 3, 3},
		{" 1.3.3 ", 1, 3, 3},
		{"1.2.3-beta.1", 1, 2, 3},   // prerelease 는 허용하되 무시
		{"1.2.3+20260813", 1, 2, 3}, // build 메타데이터도 무시
		{"10.20.30", 10, 20, 30},
	}
	for _, tc := range cases {
		ma, mi, pa, err := ParseSemver(tc.in)
		if err != nil {
			t.Errorf("ParseSemver(%q) 오류: %v", tc.in, err)
			continue
		}
		if ma != tc.major || mi != tc.minor || pa != tc.patch {
			t.Errorf("ParseSemver(%q) = %d.%d.%d, 기대 %d.%d.%d", tc.in, ma, mi, pa, tc.major, tc.minor, tc.patch)
		}
	}
}

func TestParseSemver_실패(t *testing.T) {
	// "dev"/"unknown"/"" 은 여기서 특별대우하지 않는다 — Core 가 먼저 개발 빌드를 분기해야 한다.
	for _, in := range []string{"", "   ", "dev", "unknown", "1", "1.2", "1.2.3.4", "1.2.x", "a.b.c", "-1.0.0", "1..3"} {
		if _, _, _, err := ParseSemver(in); err == nil {
			t.Errorf("ParseSemver(%q) 가 통과했다(거부해야 함)", in)
		}
	}
}

func TestSatisfiesRange_빈제약은_무조건_통과(t *testing.T) {
	for _, c := range []string{"", "   ", "\t"} {
		ok, err := SatisfiesRange("1.3.3", c)
		if err != nil || !ok {
			t.Errorf("빈 제약 %q: ok=%v err=%v (무조건 true 여야 함)", c, ok, err)
		}
	}
}

func TestSatisfiesRange_연산자와_AND결합(t *testing.T) {
	cases := []struct {
		version    string
		constraint string
		want       bool
	}{
		{"1.3.3", ">=1.3.0", true},
		{"1.2.9", ">=1.3.0", false},
		{"1.3.0", ">=1.3.0", true},
		{"1.3.1", ">1.3.0", true},
		{"1.3.0", ">1.3.0", false},
		{"1.9.9", "<2.0.0", true},
		{"2.0.0", "<2.0.0", false},
		{"2.0.0", "<=2.0.0", true},
		{"2.0.1", "<=2.0.0", false},
		{"1.3.3", "=1.3.3", true},
		{"1.3.3", "1.3.3", true},  // 연산자 없는 맨 버전 = 동등
		{"1.3.4", "1.3.3", false}, //
		{"1.3.3", ">=1.3.0 <2.0.0", true},
		{"2.0.0", ">=1.3.0 <2.0.0", false},
		{"1.2.0", ">=1.3.0 <2.0.0", false},
		{"1.3.3", ">=1.3.0, <2.0.0", true}, // 쉼표 구분도 허용
		{"1.3.3", ">=1.3 <2", true},        // 부분 버전 제약(빠진 요소는 0)
		{"1.3.3", ">1.3.3 <2.0.0", false},
	}
	for _, tc := range cases {
		got, err := SatisfiesRange(tc.version, tc.constraint)
		if err != nil {
			t.Errorf("SatisfiesRange(%q, %q) 오류: %v", tc.version, tc.constraint, err)
			continue
		}
		if got != tc.want {
			t.Errorf("SatisfiesRange(%q, %q) = %v, 기대 %v", tc.version, tc.constraint, got, tc.want)
		}
	}
}

func TestSatisfiesRange_오류(t *testing.T) {
	cases := []struct{ version, constraint string }{
		{"dev", ">=1.3.0"},     // 개발 빌드는 Core 가 먼저 분기해야 한다 — 여기서는 오류
		{"1.3.3", "^1.3.0"},    // 미지원 연산자
		{"1.3.3", "~1.3.0"},    // 미지원 연산자
		{"1.3.3", ">=abc"},     // 버전 파싱 실패
		{"1.3.3", ">="},        // 버전 없음
		{"1.3.3", ">=1.2.3.4"}, // 요소 과다
	}
	for _, tc := range cases {
		if ok, err := SatisfiesRange(tc.version, tc.constraint); err == nil {
			t.Errorf("SatisfiesRange(%q, %q) 가 오류 없이 %v 를 돌려줬다", tc.version, tc.constraint, ok)
		}
	}
}
