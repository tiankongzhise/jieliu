// Package models 定义全服务共享的数据结构。
//
// 这些结构对应数据库表行，跨 repository / pricing / scan / server 各层传递，
// 因此集中放在独立包内，避免循环依赖。
package models

import "time"

// FileRecord 文件扫描记录。每张账单图片对应一行，按 file_md5 去重。
type FileRecord struct {
	ID          int64   `json:"id"`
	FileMD5     string  `json:"file_md5"`
	FileName    string  `json:"file_name"`
	FilePath    string  `json:"file_path"`
	ScanTime    string  `json:"scan_time"`
	IsProcessed int     `json:"is_processed"` // 1=已成功解析入库
	IsAbnormal  int     `json:"is_abnormal"`  // 1=无日期/解析失败，待人工补录
	UserID      int64   `json:"user_id"`      // 归属用户
	Source      string  `json:"source"`       // "local" | "baidu"
	ErrNote     *string `json:"err_note"`     // 异常时的原因，前端展示
}

// BalanceRecord 账单余额记录，由文件解析得到。
type BalanceRecord struct {
	ID          int64   `json:"id"`
	FileID      *int64  `json:"file_id"`
	BillDate    string  `json:"bill_date"`     // yyyy-MM-dd
	ElecBalance float64 `json:"elec_balance"`  // 电费余额（元）
	GasBalance  float64 `json:"gas_balance"`   // 燃气余额（元）
	WaterBalance float64 `json:"water_balance"` // 水费余额（元，预留）
	OCRText     string  `json:"ocr_text"`
	ManualFix   int     `json:"manual_fix"` // 1=人工修正过
	UserID      int64   `json:"user_id"`
}

// DailyConsume 每日用量统计，由相邻两日余额差额反推。
type DailyConsume struct {
	ID         int64   `json:"id"`
	StatDate   string  `json:"stat_date"` // 统计日（前一日账单的日期）
	ElecAmount float64 `json:"elec_amount"`
	GasAmount  float64 `json:"gas_amount"`
	WaterAmount float64 `json:"water_amount"` // 预留
	ElecCost   float64 `json:"elec_cost"`
	GasCost    float64 `json:"gas_cost"`
	WaterCost  float64 `json:"water_cost"` // 预留
	IsManual   int     `json:"is_manual"`  // 1=人工修正，自动重算时跳过
	FixNote    string  `json:"fix_note"`
	UserID     int64   `json:"user_id"`
}

// User 注册用户。
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"` // 不序列化
	CreatedAt    time.Time `json:"created_at"`
}

// BaiduBinding 用户绑定的百度网盘授权。token 字段为加密后的密文。
type BaiduBinding struct {
	ID              int64     `json:"id"`
	UserID          int64     `json:"user_id"`
	BaiduUID        string    `json:"baidu_uid"`
	AccessTokenEnc  []byte    `json:"-"` // AES-GCM 密文（含 salt+nonce），不序列化
	RefreshTokenEnc []byte    `json:"-"`
	ExpireAt        time.Time `json:"expire_at"`
	AuthType        string    `json:"auth_type"` // "permanent" | "temporary"
	TempUntil       *time.Time `json:"temp_until"` // 临时授权到期时间，永久授权为 nil
	CreatedAt       time.Time `json:"created_at"`
}

// FixLog 人工修正操作日志，保留来源证据。
type FixLog struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	TargetDay string `json:"target_day"`
	OptType   string `json:"opt_type"` // "balance" | "consume"
	OldVal    string `json:"old_val"`
	NewVal    string `json:"new_val"`
	Note      string `json:"note"`
	OptTime   string `json:"opt_time"`
}
