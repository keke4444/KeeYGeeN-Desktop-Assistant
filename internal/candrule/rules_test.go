package candrule

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadTXT(t *testing.T) {
	p := writeTemp(t, "rules.txt", "# 注释\n+exe\n-dll\nsetup\n")
	r, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Excluded("foo.dll") {
		t.Fatal("dll 应被排除")
	}
	if r.Excluded("foo.exe") {
		t.Fatal("exe 不应被排除")
	}
	if r.Rank("setup.msi") != 1 {
		t.Fatalf("setup 优先级应为 1，实际 %d", r.Rank("setup.msi"))
	}
	if r.Rank("other.txt") != math.MaxInt {
		t.Fatal("未命中优先规则的名称排序值应为 MaxInt")
	}
}

func TestLoadJSON(t *testing.T) {
	p := writeTemp(t, "rules.json", `{"exclude":["tmp"],"priority":["报告","截图"]}`)
	r, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Excluded("a.tmp") {
		t.Fatal("tmp 应被排除")
	}
	if r.Rank("年度报告.docx") != 0 || r.Rank("截图 2026.png") != 1 {
		t.Fatal("优先顺序不正确")
	}
}

func TestLoadJSONArray(t *testing.T) {
	p := writeTemp(t, "arr.json", `["+zip","-bak"]`)
	r, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Excluded("x.bak") || r.Rank("a.zip") != 0 {
		t.Fatal("数组格式解析不正确")
	}
}

func TestLoadCSV(t *testing.T) {
	p := writeTemp(t, "rules.csv", "keyword,action\npsd,exclude\n合同,priority\n")
	r, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Excluded("设计.psd") {
		t.Fatal("psd 应被排除")
	}
	if r.Rank("采购合同.pdf") != 0 {
		t.Fatal("合同应优先")
	}
}

func TestLoadINI(t *testing.T) {
	p := writeTemp(t, "rules.ini", "[rules]\nexclude = tmp, bak\npriority = 报告, 截图\n")
	r, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Excluded("x.bak") {
		t.Fatal("bak 应被排除")
	}
	if r.Rank("月报 报告.xlsx") != 0 {
		t.Fatal("报告应优先")
	}
}

func TestLoadTOML(t *testing.T) {
	p := writeTemp(t, "rules.toml", "exclude = [\"log\"]\npriority = [\"发票\"]\n")
	r, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Excluded("a.log") || r.Rank("发票.pdf") != 0 {
		t.Fatal("TOML 规则不正确")
	}
}

func TestEmptyRules(t *testing.T) {
	var r *Rules
	if r.Excluded("x") {
		t.Fatal("空规则不应排除")
	}
	if r.Rank("x") == 0 {
		t.Fatal("空规则排序值应为最大值")
	}
}
