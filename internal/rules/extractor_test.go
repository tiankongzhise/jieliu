package rules

import "testing"

func TestExtract_ContentBeatsFilename(t *testing.T) {
	e, errs := NewExtractor(DefaultRules(1))
	if len(errs) != 0 {
		t.Fatalf("默认规则编译出错: %v", errs)
	}
	// 文件名是 2024-06-22，内容是 2024-06-23，应以内容为准
	r := e.Extract("bill_2024-06-22.jpg", "日期 2024-06-23 余额 100")
	if r.Date != "2024-06-23" || r.Source != SourceContent {
		t.Fatalf("应以内容日期为准，实际 date=%s source=%s", r.Date, r.Source)
	}
	if !r.Confident {
		t.Fatal("content 命中应为高可信")
	}
}

func TestExtract_FallbackToFilename(t *testing.T) {
	e, _ := NewExtractor(DefaultRules(1))
	r := e.Extract("bill_2024-06-22.jpg", "")
	if r.Date != "2024-06-22" || r.Source != SourceFilename {
		t.Fatalf("无内容时应回退文件名，实际 date=%s source=%s", r.Date, r.Source)
	}
}

func TestExtract_PriorityOrder(t *testing.T) {
	// 高优先级规则匹配失败时回退低优先级
	rules := []DateRule{
		{UserID: 1, Name: "高标准", Source: SourceFilename, Pattern: `D(\d{4})(\d{2})(\d{2})`, Priority: 100, Enabled: true},
		{UserID: 1, Name: "低宽松", Source: SourceFilename, Pattern: `(\d{4})[-_](\d{1,2})[-_](\d{1,2})`, Priority: 10, Enabled: true},
	}
	e, _ := NewExtractor(rules)
	// 输入不含 D 前缀，高标准不命中，应命中低宽松
	r := e.Extract("2024-06-22", "")
	if r.Date != "2024-06-22" || r.MatchedRule != "低宽松" {
		t.Fatalf("应回退低优先级规则，实际 date=%s rule=%s", r.Date, r.MatchedRule)
	}
}

func TestExtract_NoMatch(t *testing.T) {
	e, _ := NewExtractor(DefaultRules(1))
	r := e.Extract("no_date_here.jpg", "")
	if r.Date != "" {
		t.Fatalf("未命中应返回空日期，实际 %s", r.Date)
	}
}

func TestExtract_InvalidRegexSkipped(t *testing.T) {
	rules := []DateRule{
		{UserID: 1, Name: "坏正则", Source: SourceFilename, Pattern: `(`, Priority: 100, Enabled: true},
		{UserID: 1, Name: "好正则", Source: SourceFilename, Pattern: `(\d{4})-(\d{2})-(\d{2})`, Priority: 10, Enabled: true},
	}
	e, errs := NewExtractor(rules)
	if len(errs) == 0 {
		t.Fatal("坏正则应产生编译错误")
	}
	r := e.Extract("2024-06-22", "")
	if r.Date != "2024-06-22" {
		t.Fatalf("坏规则不应阻断好规则，实际 %s", r.Date)
	}
}
