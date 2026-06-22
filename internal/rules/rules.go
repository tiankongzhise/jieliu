// Package rules 实现可配置的日期提取规则。
//
// 设计要点（README 要求）：
//   - 时间规则抽象成独立层，水电气可各自配置不同规则
//   - 对文件名 / 图片内容使用不同规则
//   - 规则带 priority 权重，数值大者优先生效
//   - content 命中整体优先于 filename（图片内日期更可信）
package rules

// Source 规则作用对象。
type Source string

const (
	SourceFilename Source = "filename"
	SourceContent  Source = "content"
)

// DateRule 单条日期提取规则。
type DateRule struct {
	ID       int64  `json:"id"`
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	Source   Source `json:"source"`
	Pattern  string `json:"pattern"`  // 正则；前 3 个捕获组依次为 年/月/日
	Priority int    `json:"priority"` // 数值大者优先
	Enabled  bool   `json:"enabled"`
}

// ExtractResult 提取结果，保留来源证据供前端展示与人工确认。
type ExtractResult struct {
	Date        string `json:"date"`         // yyyy-MM-dd，空表示未命中
	Source      Source `json:"source"`       // 实际命中的来源
	MatchedRule string `json:"matched_rule"` // 命中规则名
	Confident   bool   `json:"confident"`    // 是否高可信（content 命中）
}
