package pricing

import (
	"errors"
	"fmt"
)

// CalcElecUsage 电费月度阶梯反向推算。monthUsed 为当月此前累计电量。
func CalcElecUsage(cost, monthUsed float64, cfg *PricingConfig) float64 {
	if cfg == nil {
		return 0
	}
	return cfg.ReverseCalc(cost, monthUsed)
}

// CalcGasUsage 燃气年度阶梯反向推算。yearlyUsed 为本年此前累计用气量。
//
// 修正说明：旧版签名 calcGasUsage(cost, cfg) 忽略年度累计，
// 现改为显式传入 yearlyUsed，跨年重置由调用方按 stat_year 分组维护。
func CalcGasUsage(cost, yearlyUsed float64, cfg *PricingConfig) float64 {
	if cfg == nil {
		return 0
	}
	return cfg.ReverseCalc(cost, yearlyUsed)
}

// CalcWaterUsage 水费反向推算（预留接口）。
//
// TODO(deferred): 水费阶梯规则尚未确认，暂返回 0 并标注，
// 待用户提供当地水价阶梯后填充 Tiers 与 cycle（通常为月度/季度）。
func CalcWaterUsage(cost, cumulative float64, cfg *PricingConfig) float64 {
	if cfg == nil || len(cfg.Tiers) == 0 {
		return 0
	}
	return cfg.ReverseCalc(cost, cumulative)
}

// ForwardCost 正向计费：给定用量与累计，算出花费。主要用于校验与展示。
func (p *PricingConfig) ForwardCost(amount, cumulative float64) float64 {
	if amount <= 0 || len(p.Tiers) == 0 {
		return 0
	}
	p.normalize()
	cost := 0.0
	remaining := amount
	prevLimit := 0.0
	for _, t := range p.Tiers {
		if t.Price <= 0 {
			continue
		}
		bandCapacity := t.Limit - prevLimit
		avail := bandCapacity - cumulative
		if avail < 0 {
			avail = 0
		}
		if remaining <= avail {
			cost += remaining * t.Price
			return cost
		}
		cost += avail * t.Price
		remaining -= avail
		prevLimit = t.Limit
		cumulative -= bandCapacity
		if cumulative < 0 {
			cumulative = 0
		}
	}
	last := p.Tiers[len(p.Tiers)-1]
	cost += remaining * last.Price
	return cost
}

// Validate 校验配置合法性，供 web 保存与文件导入时调用。
func (p *PricingConfig) Validate() error {
	if p == nil {
		return errors.New("配置为空")
	}
	switch p.UtilityType {
	case Electricity, Gas, Water:
	default:
		return fmt.Errorf("未知能源类型 %q", p.UtilityType)
	}
	switch p.Cycle {
	case CycleMonthly, CycleYearly:
	default:
		return fmt.Errorf("未知计费周期 %q", p.Cycle)
	}
	if len(p.Tiers) == 0 {
		return errors.New("至少需要一档")
	}
	prev := -1.0
	for i, t := range p.Tiers {
		if t.Price < 0 {
			return fmt.Errorf("第 %d 档单价不能为负", i+1)
		}
		if i > 0 && t.Limit <= prev {
			return fmt.Errorf("第 %d 档上限必须递增", i+1)
		}
		prev = t.Limit
	}
	return nil
}
