package pricing

import (
	"math"
	"testing"
)

// 默认长沙电价（月度）：0-180 @0.588，181-450 @0.638，>450 @0.888
func elecCfg() *PricingConfig {
	return &PricingConfig{
		UtilityType: Electricity,
		Cycle:       CycleMonthly,
		Tiers: []Tier{
			{Limit: 180, Price: 0.588},
			{Limit: 450, Price: 0.638},
			{Limit: 1e9, Price: 0.888}, // 三档无上限
		},
	}
}

// 默认长沙燃气（年度）：0-390 @2.65，391-600 @2.92，>600 @3.75
func gasCfg() *PricingConfig {
	return &PricingConfig{
		UtilityType: Gas,
		Cycle:       CycleYearly,
		Tiers: []Tier{
			{Limit: 390, Price: 2.65},
			{Limit: 600, Price: 2.92},
			{Limit: 1e9, Price: 3.75},
		},
	}
}

func approx(a, b float64) bool {
	return math.Abs(a-b) < 0.01
}

func TestReverseCalc_ElecTier1(t *testing.T) {
	cfg := elecCfg()
	// 月初未用，花费恰好覆盖 100 度一档电量
	got := CalcElecUsage(100*0.588, 0, cfg)
	if !approx(got, 100) {
		t.Fatalf("一档推算期望 100，实际 %.4f", got)
	}
}

func TestReverseCalc_ElecCrossTier(t *testing.T) {
	cfg := elecCfg()
	// 花费 = 180度一档 + 100度二档 = 180*0.588 + 100*0.638
	cost := 180*0.588 + 100*0.638
	got := CalcElecUsage(cost, 0, cfg)
	if !approx(got, 280) {
		t.Fatalf("跨档推算期望 280，实际 %.4f", got)
	}
}

func TestReverseCalc_ElecWithCumulative(t *testing.T) {
	cfg := elecCfg()
	// 当月已用 100 度，再花 80*0.588，应得 80 度（仍在剩余一档）
	got := CalcElecUsage(80*0.588, 100, cfg)
	if !approx(got, 80) {
		t.Fatalf("带累计一档推算期望 80，实际 %.4f", got)
	}
}

func TestReverseCalc_ElecCumulativeExhaustsTier1(t *testing.T) {
	cfg := elecCfg()
	// 当月已用 150 度，一档剩 30 度
	// 再花 30*0.588 + 50*0.638，应得 80 度
	cost := 30*0.588 + 50*0.638
	got := CalcElecUsage(cost, 150, cfg)
	if !approx(got, 80) {
		t.Fatalf("累计耗尽一档推算期望 80，实际 %.4f", got)
	}
}

func TestReverseCalc_ZeroCost(t *testing.T) {
	cfg := elecCfg()
	if got := CalcElecUsage(0, 0, cfg); got != 0 {
		t.Fatalf("零花费应返回 0，实际 %.4f", got)
	}
	if got := CalcElecUsage(-5, 0, cfg); got != 0 {
		t.Fatalf("负花费应返回 0，实际 %.4f", got)
	}
}

func TestReverseCalc_NilCfg(t *testing.T) {
	if got := CalcElecUsage(10, 0, nil); got != 0 {
		t.Fatalf("nil 配置应返回 0，实际 %.4f", got)
	}
}

// 验证旧版 bug 已修复：燃气年度累计跨档
func TestReverseCalc_GasYearlyCumulative(t *testing.T) {
	cfg := gasCfg()
	// 年度已用 380 方（接近一档上限 390），再花：
	//   一档剩 10 方 * 2.65 = 26.5
	//   二档 20 方 * 2.92 = 58.4
	//   共 84.9，应得 30 方
	cost := 10*2.65 + 20*2.92
	got := CalcGasUsage(cost, 380, cfg)
	if !approx(got, 30) {
		t.Fatalf("燃气年度累计跨档推算期望 30，实际 %.4f", got)
	}
}

// 旧 bug 复现对照：忽略累计会低估
func TestReverseCalc_GasCumulativeChangesResult(t *testing.T) {
	cfg := gasCfg()
	cost := 10*2.65 + 20*2.92
	withCum := CalcGasUsage(cost, 380, cfg)
	withoutCum := CalcGasUsage(cost, 0, cfg)
	// 不传累计时花费全部落在一档，推算出更多方数（因为单价低）
	if withoutCum <= withCum {
		t.Fatalf("累计应使推算方数变小：有累计=%.4f 无累计=%.4f", withCum, withoutCum)
	}
}

// 正向计费与反向推算互逆
func TestForwardReverseInverse(t *testing.T) {
	cfg := elecCfg()
	amount := 123.4
	cumulative := 50.0
	cost := cfg.ForwardCost(amount, cumulative)
	back := cfg.ReverseCalc(cost, cumulative)
	if !approx(back, amount) {
		t.Fatalf("正反互逆失败：用量 %.4f -> 花费 %.4f -> 推回 %.4f", amount, cost, back)
	}
}

func TestValidate(t *testing.T) {
	if err := elecCfg().Validate(); err != nil {
		t.Fatalf("合法配置不应报错：%v", err)
	}
	bad := &PricingConfig{UtilityType: "x", Cycle: CycleMonthly, Tiers: []Tier{{Limit: 10, Price: 1}}}
	if err := bad.Validate(); err == nil {
		t.Fatal("未知能源类型应报错")
	}
	ord := &PricingConfig{UtilityType: Electricity, Cycle: CycleMonthly, Tiers: []Tier{
		{Limit: 100, Price: 1},
		{Limit: 50, Price: 1}, // 上限倒序
	}}
	if err := ord.Validate(); err == nil {
		t.Fatal("档位上限非递增应报错")
	}
}
