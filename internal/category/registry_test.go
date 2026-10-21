package category

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateName(t *testing.T) {
	valid := []string{"工具", "编程", "repwn", "re-pwn", "AI", "取证 2026"}
	for _, n := range valid {
		if err := ValidateName(n); err != nil {
			t.Errorf("期望 %q 合法，得到错误: %v", n, err)
		}
	}

	invalid := []string{
		"",
		"re/pwn",
		`工具\编程`,
		"工具?",
		"a:b",
		"a*b",
		`a"b`,
		"a<b",
		"a>b",
		"a|b",
		"结尾空格 ",
		"结尾点.",
		"CON",
		"con",
		"nul.txt",
		"COM1",
	}
	for _, n := range invalid {
		if err := ValidateName(n); err == nil {
			t.Errorf("期望 %q 非法，但校验通过了", n)
		}
	}
}

func TestBuildFromRootAndConfig(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "工具", "编程", ".hidden", "临时")

	reg, err := Build(root, true, []string{"取证", "工具"}, []string{"临时"})
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}
	names := reg.Names()
	want := []string{"工具", "编程", "取证"}
	if len(names) != len(want) {
		t.Fatalf("期望 %v，得到 %v", want, names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("期望 %v，得到 %v", want, names)
		}
	}
}

func TestBuildMissingRootIsEmpty(t *testing.T) {
	reg, err := Build(filepath.Join(t.TempDir(), "home"), true, nil, nil)
	if err != nil {
		t.Fatalf("根目录不存在时不应报错: %v", err)
	}
	if len(reg.Categories) != 0 {
		t.Fatalf("期望空分类，得到 %v", reg.Names())
	}
}

func TestBuildRejectsIllegalConfiguredName(t *testing.T) {
	root := t.TempDir()
	if _, err := Build(root, true, []string{"re/pwn"}, nil); err == nil {
		t.Fatalf("非法分类名必须报错")
	}
}

func mkdirs(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(root, n), 0o755); err != nil {
			t.Fatalf("创建目录 %s 失败: %v", n, err)
		}
	}
}
