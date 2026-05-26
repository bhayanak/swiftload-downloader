package gui

import "testing"

func TestIsNewer(t *testing.T) {
	tests := []struct {
		remote string
		local  string
		want   bool
	}{
		{"v2.1.0", "2.0.0", true},
		{"v3.0.0", "2.9.9", true},
		{"v2.0.1", "2.0.0", true},
		{"v2.0.0", "2.0.0", false},
		{"v1.9.0", "2.0.0", false},
		{"v2.0.0-beta.1", "2.0.0", false}, // pre-release ignored
		{"invalid", "2.0.0", false},
		{"v2.0.0", "invalid", false},
	}
	for _, tt := range tests {
		got := isNewer(tt.remote, tt.local)
		if got != tt.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tt.remote, tt.local, got, tt.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  []int
	}{
		{"v2.1.3", []int{2, 1, 3}},
		{"1.0.0", []int{1, 0, 0}},
		{"v0.9.12", []int{0, 9, 12}},
		{"v1.0.0-rc.1", nil},
		{"bad", nil},
		{"1.2", nil},
	}
	for _, tt := range tests {
		got := parseVersion(tt.input)
		if tt.want == nil {
			if got != nil {
				t.Errorf("parseVersion(%q) = %v, want nil", tt.input, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("parseVersion(%q) = nil, want %v", tt.input, tt.want)
			continue
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Errorf("parseVersion(%q)[%d] = %d, want %d", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}
