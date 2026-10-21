//go:build windows

package hotkey

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		spec string
		ok   bool
	}{
		{"Ctrl+Alt+D", true},
		{"ctrl+shift+f5", true},
		{"Win+Space", true},
		{"Alt+Enter", true},
		{"Ctrl+Alt+", false},
		{"Ctrl", false},
		{"D", false},
		{"Ctrl+Alt+ABC", false},
		{"Ctrl+Alt+UnknownKey", false},
	}
	for _, c := range cases {
		_, _, err := Parse(c.spec)
		if c.ok && err != nil {
			t.Errorf("%q 应合法，得到错误: %v", c.spec, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%q 应非法，但解析通过了", c.spec)
		}
	}
}

func TestParseKeyCodes(t *testing.T) {
	mods, vk, err := Parse("Ctrl+Alt+D")
	if err != nil {
		t.Fatal(err)
	}
	if mods != modControl|modAlt {
		t.Fatalf("修饰键不正确: %d", mods)
	}
	if vk != 'D' {
		t.Fatalf("虚拟键码不正确: %d", vk)
	}

	_, vk, err = Parse("Shift+F5")
	if err != nil {
		t.Fatal(err)
	}
	if vk != 0x74 {
		t.Fatalf("F5 键码不正确: 0x%x", vk)
	}
}
