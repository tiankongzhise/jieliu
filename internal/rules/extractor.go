package rules

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Extractor 按规则集从文件名与 OCR 文本中提取日期。
//
// 提取策略：
//  1. content 规则整体优先于 filename（图片内日期更可信）
//  2. 同一 source 内，按 priority 降序尝试，命中即止
//  3. 全部未命中返回空 Date，由调用方标记异常
type Extractor struct {
	rules []compiledRule
}

type compiledRule struct {
	rule   DateRule
	regexp *regexp.Regexp
}

// NewExtractor 编译规则集。无效正则会被跳过并记录到返回的错误列表，
// 但不阻止其它规则生效（避免一条坏规则拖垮整个扫描）。
func NewExtractor(rules []DateRule) (*Extractor, []error) {
	var errs []error
	e := &Extractor{}
	// 先按 source 分组优先级：content 优先，再 priority 降序
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Source != rules[j].Source {
			return rules[i].Source == SourceContent // content 排前
		}
		return rules[i].Priority > rules[j].Priority
	})
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			errs = append(errs, fmt.Errorf("规则 %q 正则无效: %w", r.Name, err))
			continue
		}
		e.rules = append(e.rules, compiledRule{rule: r, regexp: re})
	}
	return e, errs
}

// Extract 执行提取。filename 为文件名，ocrText 为图片识别文本（可空）。
func (e *Extractor) Extract(filename, ocrText string) ExtractResult {
	// 先扫 content 规则（已排在前面），命中即返回
	for _, cr := range e.rules {
		if cr.rule.Source != SourceContent {
			continue
		}
		if d, ok := tryMatch(cr, ocrText); ok {
			return ExtractResult{Date: d, Source: SourceContent, MatchedRule: cr.rule.Name, Confident: true}
		}
	}
	// 再扫 filename 规则
	for _, cr := range e.rules {
		if cr.rule.Source != SourceFilename {
			continue
		}
		if d, ok := tryMatch(cr, filename); ok {
			return ExtractResult{Date: d, Source: SourceFilename, MatchedRule: cr.rule.Name, Confident: false}
		}
	}
	return ExtractResult{Source: "", Confident: false}
}

func tryMatch(cr compiledRule, input string) (string, bool) {
	if input == "" {
		return "", false
	}
	m := cr.regexp.FindStringSubmatch(input)
	if len(m) < 4 {
		return "", false
	}
	year, month, day := strings.TrimSpace(m[1]), strings.TrimSpace(m[2]), strings.TrimSpace(m[3])
	date := fmt.Sprintf("%s-%02s-%02s", year, month, day)
	if !validDate(date) {
		return "", false
	}
	return date, true
}

func validDate(d string) bool {
	_, err := time.Parse("2006-01-02", d)
	return err == nil
}

// DefaultRules 返回内置默认规则（注册新用户时种子）。
//   - content 优先级 100：匹配标准 yyyy-MM-dd
//   - filename 优先级 10：兼容 yyyy-MM-dd / yyyy_MM_dd / yyyyMMdd
func DefaultRules(userID int64) []DateRule {
	return []DateRule{
		{
			UserID: userID, Name: "内容-标准日期", Source: SourceContent,
			Pattern: `(\d{4})-(\d{1,2})-(\d{1,2})`, Priority: 100, Enabled: true,
		},
		{
			UserID: userID, Name: "内容-紧凑日期", Source: SourceContent,
			Pattern: `(\d{4})(\d{2})(\d{2})`, Priority: 90, Enabled: true,
		},
		{
			UserID: userID, Name: "文件名-常见日期", Source: SourceFilename,
			Pattern: `(\d{4})[-_]?(\d{1,2})[-_]?(\d{1,2})`, Priority: 10, Enabled: true,
		},
	}
}
