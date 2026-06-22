// Package pricing 实现电/气/水的阶梯计费与反向推算。
//
// 核心模型：
//   - 用量差额由相邻两日"余额"相减得到（先扣费后推用量）
//   - 阶梯按 cycle 周期重置：electricity=月度、gas/water=年度
//   - 推算时需要"周期内已用量"，因此 calcAllDailyConsume 维护按月/按年的累计器
//
// 修正历史：旧 calcGasUsage 忽略年度累计，每次从一档 0 起算，
// 导致跨档后单价偏低。现统一为带 cumulative 用量的逆向阶梯算法。
package pricing

import "sort"

// UtilityType 能源类型。
type UtilityType string

const (
	Electricity UtilityType = "electricity"
	Gas         UtilityType = "gas"
	Water       UtilityType = "water" // 预留，规则待补
)

// Cycle 计费周期。
type Cycle string

const (
	CycleMonthly Cycle = "monthly"
	CycleYearly  Cycle = "yearly"
)

// Tier 单个阶梯档位。Limit 为"累计上限"（含本档及之前）。
type Tier struct {
	Limit float64 `json:"limit"`
	Price float64 `json:"price"`
}

// PricingConfig 单种能源的阶梯配置。
type PricingConfig struct {
	UserID      int64        `json:"user_id"`
	UtilityType UtilityType  `json:"utility_type"`
	Cycle       Cycle        `json:"cycle"`
	Tiers       []Tier       `json:"tiers"`      // 调用前按 Limit 升序排序
	UpdateTime  string       `json:"update_time"`
}

// normalize 按 Limit 升序排序，保证档位顺序正确。
func (p *PricingConfig) normalize() {
	sort.SliceStable(p.Tiers, func(i, j int) bool {
		return p.Tiers[i].Limit < p.Tiers[j].Limit
	})
}

// ReverseCalc 根据扣费金额与周期内已用量，逆向推算本次用量。
//
// 算法：
//   - 从第一档起，扣除"已用"后剩余的档位容量
//   - 在剩余容量内按档位单价消费金额，逐档递推
//   - cost<=0 直接返回 0（余额未降）
//
// cost: 本次扣费金额；cumulative: 当前周期内此前累计用量。
func (p *PricingConfig) ReverseCalc(cost, cumulative float64) float64 {
	if cost <= 0 || len(p.Tiers) == 0 {
		return 0
	}
	p.normalize()

	amount := 0.0
	remaining := cost
	prevLimit := 0.0

	for i, t := range p.Tiers {
		if t.Price <= 0 {
			continue
		}
		// 本档总容量 = 本档上限 - 上一档上限
		bandCapacity := t.Limit - prevLimit
		if bandCapacity < 0 {
			bandCapacity = 0
		}
		// 扣除已用量在本档占用的部分
		avail := bandCapacity - cumulative
		if avail < 0 {
			avail = 0
		}
		// 本档满档花费
		bandFullCost := avail * t.Price

		if remaining <= bandFullCost {
			amount += remaining / t.Price
			return amount
		}
		amount += avail
		remaining -= bandFullCost
		prevLimit = t.Limit
		cumulative -= bandCapacity // 已用量随档位前进而消耗
		if cumulative < 0 {
			cumulative = 0
		}
		_ = i
	}

	// 超过最高档：用最后一档单价继续推算
	last := p.Tiers[len(p.Tiers)-1]
	if last.Price > 0 {
		amount += remaining / last.Price
	}
	return amount
}
