package textarea

import (
	"testing"

	"charm.land/bubbles/v2/key"
)

func TestWrapCJK(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		width  int
		expect int // expected number of visual lines
	}{
		{
			name:   "CJK wraps at character boundary",
			input:  "你好世界测试",
			width:  6,
			expect: 2,
		},
		{
			name:   "CJK with space wraps normally",
			input:  "你好 世界",
			width:  8,
			expect: 2,
		},
		{
			name:   "CJK fits exactly",
			input:  "你好",
			width:  4,
			expect: 1,
		},
		{
			name:   "Mixed CJK and Latin wraps correctly",
			input:  "Hello你好World",
			width:  10,
			expect: 2,
		},
		{
			name:   "Latin word wrapping preserved",
			input:  "Hello World",
			width:  8,
			expect: 2,
		},
		{
			name:   "Empty input",
			input:  "",
			width:  10,
			expect: 1,
		},
		{
			name:   "Single CJK char",
			input:  "你",
			width:  10,
			expect: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrap([]rune(tt.input), tt.width)
			if len(result) != tt.expect {
				t.Errorf("wrap(%q, %d) returned %d lines, expected %d",
					tt.input, tt.width, len(result), tt.expect)
				for i, line := range result {
					t.Errorf("  line %d: %q", i, string(line))
				}
			}
			// Every line should end with a trailing space
			for i, line := range result {
				if len(line) == 0 {
					t.Errorf("line %d is empty", i)
				} else if line[len(line)-1] != ' ' {
					t.Errorf("line %d doesn't end with trailing space: %q", i, string(line))
				}
			}
		})
	}
}

func TestWrapCJKNoSpaceHardWrap(t *testing.T) {
	// CJK text with a space should NOT hard-wrap at the space position
	// when the content fits on the line.
	input := "你好 世"
	width := 10
	result := wrap([]rune(input), width)
	if len(result) != 1 {
		t.Errorf("wrap(%q, %d) returned %d lines, expected 1",
			input, width, len(result))
		for i, line := range result {
			t.Errorf("  line %d: %q", i, string(line))
		}
	}
}

func TestWordNavigationCJK(t *testing.T) {
	// Indices: H(0)e(1)l(2)l(3)o(4) ' '(5) 你(6)好(7) W(8)o(9)r(10)l(11)d(12) 测(13)试(14) ' '(15) e(16)n(17)d(18)
	m := New()
	m.SetWidth(40)
	m.SetValue("Hello 你好World测试 end")

	tests := []struct {
		name     string
		startCol int
		expected int
		forward  bool
	}{
		{"right: skip Hello", 0, 5, true},
		{"right: skip space+你(CJK)", 5, 7, true},
		{"right: skip 好(CJK)", 7, 8, true},
		{"right: skip World", 8, 13, true},
		{"right: skip 测(CJK)", 13, 14, true},
		{"right: skip 试(CJK)", 14, 15, true},
		{"right: skip space+end", 15, 19, true},
		{"right: at end stays", 19, 19, true},
		{"left: skip end", 19, 16, false},
		{"left: skip space+试(CJK)", 16, 14, false},
		{"left: skip 测(CJK)", 14, 13, false},
		{"left: skip World", 13, 8, false},
		{"left: skip 好(CJK)", 8, 7, false},
		{"left: skip 你(CJK)", 7, 6, false},
		{"left: skip space+Hello", 6, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.SetCursorColumn(tt.startCol)
			if tt.forward {
				m.wordRight()
			} else {
				m.wordLeft()
			}
			if m.col != tt.expected {
				t.Errorf("from col %d, %s → col %d, expected %d",
					tt.startCol,
					map[bool]string{true: "wordRight", false: "wordLeft"}[tt.forward],
					m.col, tt.expected)
			}
		})
	}
}

func TestDeleteWordCJK(t *testing.T) {
	m := New()
	m.SetWidth(40)

	m.SetValue("Hello你好测试")
	m.SetCursorColumn(len("Hello你好测试"))

	// Delete 试 (CJK → one char)
	m.deleteWordLeft()
	if got := m.Value(); got != "Hello你好测" {
		t.Errorf("after deleteWordLeft: got %q, want %q", got, "Hello你好测")
	}

	// Delete 测
	m.deleteWordLeft()
	if got := m.Value(); got != "Hello你好" {
		t.Errorf("after deleteWordLeft: got %q, want %q", got, "Hello你好")
	}

	// Delete 好 (CJK)
	m.deleteWordLeft()
	if got := m.Value(); got != "Hello你" {
		t.Errorf("after deleteWordLeft: got %q, want %q", got, "Hello你")
	}

	// Delete 你 (CJK)
	m.deleteWordLeft()
	if got := m.Value(); got != "Hello" {
		t.Errorf("after deleteWordLeft: got %q, want %q", got, "Hello")
	}

	// Delete "Hello" (Latin word)
	m.deleteWordLeft()
	if got := m.Value(); got != "" {
		t.Errorf("after deleteWordLeft: got %q, want %q", got, "")
	}
}

func TestCtrlArrowKeyBindings(t *testing.T) {
	km := DefaultKeyMap()

	assertHasKey := func(t *testing.T, binding key.Binding, want string) {
		t.Helper()
		for _, k := range binding.Keys() {
			if k == want {
				return
			}
		}
		t.Errorf("binding keys %v should include %q", binding.Keys(), want)
	}

	assertHasKey(t, km.WordForward, "ctrl+right")
	assertHasKey(t, km.WordBackward, "ctrl+left")
	// Original alt bindings should still work
	assertHasKey(t, km.WordForward, "alt+right")
	assertHasKey(t, km.WordBackward, "alt+left")
}
