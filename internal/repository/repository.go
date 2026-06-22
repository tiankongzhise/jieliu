// Package repository 定义数据访问接口与 SQLite 实现。
//
// 接口化便于将来替换存储或注入 mock 做单测。
// 所有方法接受 context.Context，便于超时与取消。
package repository

import (
	"context"
	"time"

	"github.com/jieliu/jieliu/internal/models"
	"github.com/jieliu/jieliu/internal/pricing"
	"github.com/jieliu/jieliu/internal/rules"
)

// Repository 数据访问接口。
type Repository interface {
	// 用户
	CreateUser(ctx context.Context, username, passwordHash string) (models.User, error)
	GetUserByName(ctx context.Context, username string) (models.User, error)
	GetUserByID(ctx context.Context, id int64) (models.User, error)

	// 文件
	HasFile(ctx context.Context, userID int64, md5 string) (bool, error)
	InsertFile(ctx context.Context, f models.FileRecord) (int64, error)
	MarkFileProcessed(ctx context.Context, fileID int64) error
	MarkFileAbnormal(ctx context.Context, fileID int64, note string) error

	// 账单
	InsertBalance(ctx context.Context, b models.BalanceRecord) error
	ListBills(ctx context.Context, userID int64) ([]BillItem, error)
	GetBill(ctx context.Context, id int64) (models.BalanceRecord, string, error) // 返回 record + filename
	UpdateBill(ctx context.Context, b models.BalanceRecord) error
	ListBalancesByUser(ctx context.Context, userID int64) ([]models.BalanceRecord, error)

	// 每日用量
	UpsertDailyConsume(ctx context.Context, d models.DailyConsume) error
	GetDailyManual(ctx context.Context, userID int64, date string) (bool, error)
	ListDailyConsume(ctx context.Context, userID int64) ([]models.DailyConsume, error)
	ListDailyForChart(ctx context.Context, userID int64) ([]ChartRow, error)

	// 计费配置
	GetPricing(ctx context.Context, userID int64, ut pricing.UtilityType) (*pricing.PricingConfig, error)
	UpsertPricing(ctx context.Context, cfg pricing.PricingConfig) error
	SeedDefaultPricing(ctx context.Context, userID int64) error

	// 日期规则
	ListRules(ctx context.Context, userID int64) ([]rules.DateRule, error)
	UpsertRule(ctx context.Context, r rules.DateRule) error
	DeleteRule(ctx context.Context, userID, ruleID int64) error
	SeedDefaultRules(ctx context.Context, userID int64) error

	// 网盘绑定
	UpsertBinding(ctx context.Context, b models.BaiduBinding) error
	GetBinding(ctx context.Context, userID int64) (*models.BaiduBinding, error)
	DeleteBinding(ctx context.Context, userID int64) error
	ListExpiredTempBindings(ctx context.Context, now time.Time) ([]models.BaiduBinding, error)

	// 日志
	InsertFixLog(ctx context.Context, l models.FixLog) error
	ListFixLogs(ctx context.Context, userID int64) ([]models.FixLog, error)
}

// BillItem 账单列表展示项（含文件名）。
type BillItem struct {
	ID          int64
	BillDate    string
	ElecBalance float64
	GasBalance  float64
	WaterBalance float64
	ManualFix   int
	FileName    string
	Source      string
}

// ChartRow 月度图表数据行。
type ChartRow struct {
	Day string  `json:"day"`
	Ele float64 `json:"ele"`
	Gas float64 `json:"gas"`
}
