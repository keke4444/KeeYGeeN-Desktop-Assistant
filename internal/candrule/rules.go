// Package candrule 解析搜索候选规则文件，对候选结果做过滤与优先排序。
//
// 规则只有两类：
//   - 排除：名称包含该词的候选直接不显示（-词 / exclude）
//   - 优先：名称包含该词的候选排在前面，按规则出现顺序决定优先级（+词 / priority）
//
// 支持 txt / json / csv / tsv / ini / toml 多种格式。
package candrule

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	maxFileBytes = 2 << 20 // 规则文件上限 2 MB
	maxRules     = 500     // 每类规则上限
)

// Rules 是解析后的候选规则。
type Rules struct {
	Exclude  []string // 命中即排除
	Priority []string // 顺序即优先级
}

// Load 按扩展名解析规则文件。
func Load(path string) (*Rules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileBytes {
		return nil, fmt.Errorf("规则文件超过 2 MB")
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return parseJSON(data)
	case ".csv":
		return parseTable(data, ',')
	case ".tsv":
		return parseTable(data, '\t')
	case ".ini", ".conf":
		return parseINI(data)
	case ".toml":
		return parseTOML(data)
	default:
		return parseLines(data)
	}
}

// Excluded 判断名称是否命中排除规则（不区分大小写）。
func (r *Rules) Excluded(name string) bool {
	if r == nil {
		return false
	}
	n := strings.ToLower(name)
	for _, w := range r.Exclude {
		if w != "" && strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// Rank 返回名称的优先级序号，越小越靠前；未命中任何优先规则返回 MaxInt。
func (r *Rules) Rank(name string) int {
	if r == nil {
		return math.MaxInt
	}
	n := strings.ToLower(name)
	for i, w := range r.Priority {
		if w != "" && strings.Contains(n, w) {
			return i
		}
	}
	return math.MaxInt
}

func (r *Rules) add(word, action string) {
	word = strings.ToLower(strings.TrimSpace(word))
	word = strings.Trim(word, `"'`)
	if word == "" {
		return
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "-", "exclude", "ex", "ban", "black", "blacklist", "deny", "排除", "禁用", "屏蔽":
		r.Exclude = appendUnique(r.Exclude, word)
	case "+", "include", "inc", "priority", "prior", "boost", "white", "whitelist", "优先", "置顶":
		r.Priority = appendUnique(r.Priority, word)
	default:
		r.Priority = appendUnique(r.Priority, word)
	}
}

func appendUnique(list []string, v string) []string {
	if len(list) >= maxRules {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// splitAction 从 "词" 或 "+词" / "-词" 中拆出动作。
func splitAction(s string) (word, action string) {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, "+"):
		return s[1:], "+"
	case strings.HasPrefix(s, "-"):
		return s[1:], "-"
	}
	return s, ""
}

func parseLines(data []byte) (*Rules, error) {
	r := &Rules{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, ";") {
			continue
		}
		action := ""
		// 也兼容 "词<TAB>动作" 的行内写法
		if i := strings.IndexAny(line, "\t,"); i >= 0 {
			head := strings.TrimSpace(line[:i])
			tail := strings.TrimSpace(line[i+1:])
			if isAction(tail) {
				word, act := splitAction(head)
				action = act
				if action == "" {
					action = tail
				}
				r.add(word, action)
				continue
			}
		}
		word, act := splitAction(line)
		if act != "" {
			action = act
		}
		r.add(word, action)
	}
	return r, sc.Err()
}

func isAction(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "+", "-", "exclude", "ex", "ban", "black", "blacklist", "deny", "include", "inc",
		"priority", "prior", "boost", "white", "whitelist", "排除", "禁用", "屏蔽", "优先", "置顶":
		return true
	}
	return false
}

type structuredRules struct {
	Exclude  []string `json:"exclude" toml:"exclude"`
	Include  []string `json:"include" toml:"include"`
	Priority []string `json:"priority" toml:"priority"`
	Rules    []string `json:"rules" toml:"rules"`
}

func (s structuredRules) applyTo(r *Rules) {
	for _, w := range s.Exclude {
		word, act := splitAction(w)
		if act == "" {
			act = "-"
		}
		r.add(word, act)
	}
	for _, w := range s.Include {
		word, act := splitAction(w)
		if act == "" {
			act = "+"
		}
		r.add(word, act)
	}
	for _, w := range s.Priority {
		word, act := splitAction(w)
		if act == "" {
			act = "+"
		}
		r.add(word, act)
	}
	for _, w := range s.Rules {
		word, act := splitAction(w)
		r.add(word, act)
	}
}

func parseJSON(data []byte) (*Rules, error) {
	// 支持 ["+exe", "-tmp"] 这种简写数组
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		r := &Rules{}
		for _, item := range arr {
			word, act := splitAction(item)
			r.add(word, act)
		}
		return r, nil
	}

	var obj structuredRules
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	r := &Rules{}
	obj.applyTo(r)
	return r, nil
}

func parseTOML(data []byte) (*Rules, error) {
	var obj structuredRules
	if err := toml.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("TOML 解析失败: %w", err)
	}
	r := &Rules{}
	obj.applyTo(r)
	return r, nil
}

func parseTable(data []byte, sep rune) (*Rules, error) {
	r := &Rules{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		fields := strings.Split(line, string(sep))
		for i := range fields {
			fields[i] = strings.Trim(strings.TrimSpace(fields[i]), `"'`)
		}
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		// 跳过表头
		switch strings.ToLower(fields[0]) {
		case "keyword", "name", "word", "词", "关键词", "名称":
			continue
		}
		word, act := splitAction(fields[0])
		if len(fields) > 1 && fields[1] != "" {
			if isAction(fields[1]) {
				act = strings.ToLower(fields[1])
			} else if strings.TrimSpace(fields[1]) != "" {
				// 第二列不是动作时，默认把首列当优先词
				act = "+"
			}
		}
		r.add(word, act)
	}
	return r, sc.Err()
}

func parseINI(data []byte) (*Rules, error) {
	r := &Rules{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "[") {
			continue
		}
		i := strings.IndexAny(line, "=:")
		if i < 0 {
			// 没有等号时按普通行处理
			word, act := splitAction(line)
			r.add(word, act)
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:i]))
		val := strings.TrimSpace(line[i+1:])
		isExclude := strings.Contains(key, "exclude") || strings.Contains(key, "black") ||
			strings.Contains(key, "deny") || strings.Contains(key, "排除") || strings.Contains(key, "禁用")
		for _, part := range strings.FieldsFunc(val, func(c rune) bool {
			return c == ',' || c == ';' || c == ' ' || c == '\t'
		}) {
			word, act := splitAction(part)
			if act == "" {
				if isExclude {
					act = "-"
				} else {
					act = "+"
				}
			}
			r.add(word, act)
		}
	}
	return r, sc.Err()
}
