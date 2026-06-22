package main

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ===================== 全局常量配置 =====================
const (
	ScanDir        = "./baidupan"
	DBFile         = "./data.db"
	ServerAddr     = ":8080"
	ScanInterval   = 5 // 分钟
	AdminPassword  = "admin123"
)

// 模型定义
type FileRecord struct {
	ID          int
	FileMD5     string
	FileName    string
	FilePath    string
	ScanTime    string
	IsProcessed int
	IsAbnormal  int // 1=无日期待人工补录
}

type BalanceRecord struct {
	ID          int
	FileID      int
	BillDate    string
	ElecBalance float64
	GasBalance  float64
	OCRText     string
	ManualFix   int
}

type DailyConsume struct {
	ID         int
	StatDate   string
	ElecAmount float64
	GasAmount  float64
	ElecCost   float64
	GasCost    float64
	IsManual   int
	FixNote    string
}

type BillConfig struct {
	ID               int
	ElecTier1Limit   float64 // 月度一档电量上限
	ElecTier1Price   float64
	ElecTier2Limit   float64 // 月度二档累计上限
	ElecTier2Price   float64
	ElecTier3Price   float64
	GasTier1Limit    float64
	GasTier1Price    float64
	GasTier2Limit    float64
	GasTier2Price    float64
	GasTier3Price    float64
	UpdateTime       string
}

type FixLog struct {
	ID        int
	TargetDay string
	OptType   string // balance / consume
	OldVal    string
	NewVal    string
	Note      string
	OptTime   string
}

var db *sql.DB
var tpl *template.Template

func main() {
	// 初始化数据库、目录、模板
	initDB()
	initTemplate()
	_ = os.MkdirAll(ScanDir, 0755)

	// 初始化默认计费配置
	initDefaultConfig()

	// 后台定时扫描协程
	go scanLoop()

	// Web服务启动
	http.HandleFunc("/login", pageLogin)
	http.HandleFunc("/login/submit", apiLogin)
	http.HandleFunc("/", authWrap(pageIndex))
	http.HandleFunc("/bill/list", authWrap(pageBillList))
	http.HandleFunc("/bill/edit", authWrap(pageBillEdit))
	http.HandleFunc("/bill/save", authWrap(apiBillSave))
	http.HandleFunc("/stat/day", authWrap(pageDayStat))
	http.HandleFunc("/stat/month", authWrap(pageMonthChart))
	http.HandleFunc("/stat/save", authWrap(apiConsumeSave))
	http.HandleFunc("/config", authWrap(pageConfig))
	http.HandleFunc("/config/save", authWrap(apiConfigSave))
	http.HandleFunc("/log", authWrap(pageFixLog))
	http.HandleFunc("/scan/manual", authWrap(apiManualScan))

	fmt.Printf("服务启动完成 http://127.0.0.1%s 密码:%s\n", ServerAddr, AdminPassword)
	_ = http.ListenAndServe(ServerAddr, nil)
}

// ===================== 数据库初始化 =====================
func initDB() {
	var err error
	db, err = sql.Open("sqlite3", DBFile+"?_journal=WAL&cache=shared")
	if err != nil {
		panic("数据库打开失败:" + err.Error())
	}
	db.SetMaxOpenConns(10)

	createSQL := `
	CREATE TABLE IF NOT EXISTS file_record (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_md5 TEXT UNIQUE NOT NULL,
		file_name TEXT,
		file_path TEXT,
		scan_time TEXT,
		is_processed INTEGER DEFAULT 0,
		is_abnormal INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS balance_record (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id INTEGER,
		bill_date TEXT NOT NULL,
		elec_balance REAL DEFAULT 0,
		gas_balance REAL DEFAULT 0,
		ocr_text TEXT,
		manual_fix INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS daily_consume (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		stat_date TEXT UNIQUE NOT NULL,
		elec_amount REAL DEFAULT 0,
		gas_amount REAL DEFAULT 0,
		elec_cost REAL DEFAULT 0,
		gas_cost REAL DEFAULT 0,
		is_manual INTEGER DEFAULT 0,
		fix_note TEXT
	);

	CREATE TABLE IF NOT EXISTS bill_config (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		elec_tier1_limit REAL,
		elec_tier1_price REAL,
		elec_tier2_limit REAL,
		elec_tier2_price REAL,
		elec_tier3_price REAL,
		gas_tier1_limit REAL,
		gas_tier1_price REAL,
		gas_tier2_limit REAL,
		gas_tier2_price REAL,
		gas_tier3_price REAL,
		update_time TEXT
	);

	CREATE TABLE IF NOT EXISTS fix_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		target_day TEXT,
		opt_type TEXT,
		old_val TEXT,
		new_val TEXT,
		note TEXT,
		opt_time TEXT
	);
	`
	_, err = db.Exec(createSQL)
	if err != nil {
		panic("建表失败:" + err.Error())
	}
}

// 初始化长沙月度阶梯默认配置（居民用电月度阶梯）
func initDefaultConfig() {
	var cnt int
	_ = db.QueryRow("SELECT COUNT(*) FROM bill_config").Scan(&cnt)
	if cnt > 0 {
		return
	}
	// 长沙居民月度阶梯电价（每月重置额度）
	// 一档：0-180度 0.588；二档181-450 0.638；三档451以上 0.888
	// 燃气年度阶梯不变
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.Exec(`INSERT INTO bill_config
	(elec_tier1_limit,elec_tier1_price,elec_tier2_limit,elec_tier2_price,elec_tier3_price,
	gas_tier1_limit,gas_tier1_price,gas_tier2_limit,gas_tier2_price,gas_tier3_price,update_time)
	VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		180, 0.588, 450, 0.638, 0.888,
		390, 2.65, 600, 2.92, 3.75, now)
	if err != nil {
		fmt.Println("默认配置初始化失败", err)
	}
}

func getConfig() (cfg BillConfig) {
	_ = db.QueryRow(`SELECT * FROM bill_config ORDER BY id DESC LIMIT 1`).Scan(
		&cfg.ID,
		&cfg.ElecTier1Limit, &cfg.ElecTier1Price,
		&cfg.ElecTier2Limit, &cfg.ElecTier2Price, &cfg.ElecTier3Price,
		&cfg.GasTier1Limit, &cfg.GasTier1Price,
		&cfg.GasTier2Limit, &cfg.GasTier2Price, &cfg.GasTier3Price,
		&cfg.UpdateTime,
	)
	return cfg
}

// ===================== 文件扫描增量逻辑 =====================
func scanLoop() {
	ticker := time.NewTicker(ScanInterval * time.Minute)
	defer ticker.Stop()
	for {
		fmt.Println("定时启动文件扫描")
		scanFiles()
		<-ticker.C
	}
}

func apiManualScan(w http.ResponseWriter, r *http.Request) {
	scanFiles()
	http.Redirect(w, r, "/bill/list", 302)
}

func scanFiles() {
	var allImg []string
	jpg, _ := filepath.Glob(filepath.Join(ScanDir, "*.jpg"))
	png, _ := filepath.Glob(filepath.Join(ScanDir, "*.png"))
	allImg = append(allImg, jpg...)
	allImg = append(allImg, png...)

	for _, fpath := range allImg {
		md5Str := fileMD5(fpath)
		var exist int
		_ = db.QueryRow(`SELECT 1 FROM file_record WHERE file_md5=?`, md5Str).Scan(&exist)
		if exist == 1 {
			continue
		}
		// 新增文件入库
		baseName := filepath.Base(fpath)
		now := time.Now().Format("2006-01-02 15:04:05")
		res, err := db.Exec(`INSERT INTO file_record
			(file_md5,file_name,file_path,scan_time,is_processed,is_abnormal)
			VALUES (?,?,?,?,0,0)`, md5Str, baseName, fpath, now)
		if err != nil {
			continue
		}
		fid, _ := res.LastInsertId()
		processImage(fpath, md5Str, int(fid))
	}
	fmt.Println("文件扫描完成")
}

func fileMD5(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := md5.New()
	_, _ = f.ReadTo(h)
	return hex.EncodeToString(h.Sum(nil))
}

// 图片解析：OCR日期优先，无OCR则文件名日期
func processImage(fpath, md5Str string, fid int) {
	fileName := filepath.Base(fpath)
	// 1. 文件名提取日期
	nameDate := extractDate(fileName)
	// 2. OCR识别（此处为占位，接入PaddleOCR后替换返回真实文本+ocrDate）
	ocrText, ocrDate := mockOCR(fpath)

	// 日期优先级：OCR识别日期 > 文件名日期
	var billDate string
	if isValidDate(ocrDate) {
		billDate = ocrDate
	} else if isValidDate(nameDate) {
		billDate = nameDate
	} else {
		// 无有效日期标记异常，等待人工编辑
		_, _ = db.Exec(`UPDATE file_record SET is_abnormal=1 WHERE id=?`, fid)
		fmt.Printf("文件 %s 未识别到有效日期，标记异常\n", fileName)
		return
	}

	// 模拟OCR余额，正式版替换为OCR数字正则提取
	elecBal := 186.32
	gasBal := 92.75

	// 写入账单余额记录
	_, err := db.Exec(`INSERT INTO balance_record
	(file_id,bill_date,elec_balance,gas_balance,ocr_text,manual_fix)
	VALUES (?,?,?,?,?,0)`, fid, billDate, elecBal, gasBal, ocrText)
	if err != nil {
		fmt.Println("账单入库失败", err)
		return
	}
	// 更新文件已处理
	_, _ = db.Exec(`UPDATE file_record SET is_processed=1 WHERE id=?`, fid)
	// 重新计算每日用量
	calcAllDailyConsume()
}

// 模拟OCR函数，正式替换为真实图片OCR调用
func mockOCR(path string) (text, date string) {
	// 示例：图片内识别到日期则返回，否则空
	// 真实逻辑：调用paddleocr识别图片文字，正则提取日期、金额
	return "", ""
}

// 正则提取文件名 20xx-xx-xx / 20xx_xx_xx
func extractDate(s string) string {
	reg := regexp.MustCompile(`(\d{4})[-_]?(\d{1,2})[-_]?(\d{1,2})`)
	m := reg.FindStringSubmatch(s)
	if len(m) < 4 {
		return ""
	}
	return fmt.Sprintf("%s-%02s-%02s", m[1], m[2], m[3])
}

func isValidDate(d string) bool {
	_, err := time.Parse("2006-01-02", d)
	return err == nil
}

// ===================== 计费核心（月度阶梯电价修正） =====================
// 根据扣费金额反向推算月度阶梯用电量（每月档位额度重置）
func calcElecUsage(totalCost float64, monthUsage float64, cfg BillConfig) float64 {
	if totalCost <= 0 {
		return 0
	}
	// 当月已用基础电量，本次新增消耗
	targetCost := totalCost
	tier1Limit := cfg.ElecTier1Limit
	tier2Limit := cfg.ElecTier2Limit
	p1, p2, p3 := cfg.ElecTier1Price, cfg.ElecTier2Price, cfg.ElecTier3Price

	var addAmount float64
	// 先扣除当月已有电量占用的档位
	remainTier1 := tier1Limit - monthUsage
	if remainTier1 < 0 {
		remainTier1 = 0
	}
	remainTier2 := tier2Limit - monthUsage
	if remainTier2 < remainTier1 {
		remainTier2 = remainTier1
	}

	// 一档消耗
	if targetCost <= remainTier1*p1 {
		addAmount = targetCost / p1
		return addAmount
	}
	addAmount += remainTier1
	targetCost -= remainTier1 * p1

	// 二档消耗
	tier2Total := (tier2Limit - tier1Limit) * p2
	if targetCost <= tier2Total {
		addAmount += targetCost / p2
		return addAmount
	}
	addAmount += tier2Limit - tier1Limit
	targetCost -= tier2Total

	// 三档剩余
	addAmount += targetCost / p3
	return addAmount
}

// 燃气年度阶梯推算不变
func calcGasUsage(cost float64, cfg BillConfig) float64 {
	if cost <= 0 {
		return 0
	}
	t1, t2, t3 := cfg.GasTier1Limit, cfg.GasTier2Limit, 0.0
	p1, p2, p3 := cfg.GasTier1Price, cfg.GasTier2Price, cfg.GasTier3Price
	var amt float64
	max1 := t1 * p1
	max2 := (t2 - t1) * p2
	if cost <= max1 {
		amt = cost / p1
	} else if cost <= max1+max2 {
		amt = t1 + (cost-max1)/p2
	} else {
		amt = t2 + (cost-max1-max2)/p3
	}
	return amt
}

// 批量计算所有日用量，人工修正数据不覆盖
func calcAllDailyConsume() {
	rows, err := db.Query(`SELECT bill_date,elec_balance,gas_balance FROM balance_record ORDER BY bill_date ASC`)
	if err != nil {
		return
	}
	defer rows.Close()
	var list []BalanceRecord
	for rows.Next() {
		var b BalanceRecord
		_ = rows.Scan(&b.BillDate, &b.ElecBalance, &b.GasBalance)
		list = append(list, b)
	}
	if len(list) < 2 {
		return
	}
	cfg := getConfig()
	// 按月累计电量
	monthAcc := make(map[string]float64)

	for i := 1; i < len(list); i++ {
		prev := list[i-1]
		curr := list[i]
		statDay := prev.BillDate
		ym := statDay[:7] // yyyy-MM 按月分组

		elecCost := prev.ElecBalance - curr.ElecBalance
		gasCost := prev.GasBalance - curr.GasBalance

		// 获取当月累计电量
		used := monthAcc[ym]
		elecAdd := calcElecUsage(elecCost, used, cfg)
		gasAdd := calcGasUsage(gasCost, cfg)
		monthAcc[ym] += elecAdd

		// 查询是否人工修正
		var manual int
		_ = db.QueryRow(`SELECT is_manual FROM daily_consume WHERE stat_date=?`, statDay).Scan(&manual)
		if manual == 1 {
			continue // 人工修正数据跳过自动覆盖
		}
		// 更新/插入统计数据
		_, _ = db.Exec(`REPLACE INTO daily_consume
		(stat_date,elec_amount,gas_amount,elec_cost,gas_cost,is_manual,fix_note)
		VALUES (?,?,?,?,?,0,"")`, statDay, elecAdd, gasAdd, elecCost, gasCost)
	}
}

// ===================== Web 模板与鉴权 =====================
func initTemplate() {
	tpl = template.New("page")
	// 内嵌所有页面HTML模板
	tpl.Parse(`
{{define "login"}}
<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>登录</title></head>
<body style="max-width:400px;margin:100px auto">
<h2>系统登录</h2>
<form action="/login/submit" method="post">
<input name="pwd" type="password" placeholder="输入密码" style="width:300px;padding:8px;margin:10px 0">
<button type="submit" style="padding:8px 20px">登录</button>
</form>
</body>
</html>
{{end}}

{{define "index"}}
<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>电气燃气统计首页</title></head>
<body style="padding:30px;font-size:16px">
<h1>电费燃气统计管理系统</h1>
<div style="margin:20px 0;line-height:2.2">
<a href="/bill/list" style="font-size:18px;margin-right:20px">账单记录管理</a>
<a href="/stat/day" style="font-size:18px;margin-right:20px">每日用量统计(可修正)</a>
<a href="/stat/month" style="font-size:18px;margin-right:20px">月度图表统计</a>
<a href="/config" style="font-size:18px;margin-right:20px">阶梯计费配置</a>
<a href="/log" style="font-size:18px">修改操作日志</a>
</div>
<div>
<form action="/scan/manual" method="post">
<button type="submit" style="padding:8px 16px">手动触发网盘文件扫描</button>
</form>
</div>
</body>
</html>
{{end}}

{{define "billList"}}
<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>账单列表</title></head>
<body style="padding:20px">
<a href="/">← 返回首页</a>
<h2>账单余额记录</h2>
<table border="1" cellpadding="6" cellspacing="0" width="100%">
<tr>
<th>账单日期</th><th>电费余额</th><th>燃气余额</th><th>文件名</th><th>是否人工修改</th><th>操作</th>
</tr>
{{range .List}}
<tr>
<td>{{.BillDate}}</td>
<td>{{printf "%.2f" .ElecBalance}}</td>
<td>{{printf "%.2f" .GasBalance}}</td>
<td>{{.FileName}}</td>
<td>{{if eq .ManualFix 1}}是{{else}}否{{end}}</td>
<td><a href="/bill/edit?id={{.ID}}">编辑修正</a></td>
</tr>
{{end}}
</table>
</body>
</html>
{{end}}

{{define "dayStat"}}
<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>每日用量统计</title></head>
<body style="padding:20px">
<a href="/">← 返回首页</a>
<h2>每日用电用气统计（可人工修正）</h2>
<table border="1" cellpadding="6" cellspacing="0" width="100%">
<tr>
<th>统计日期</th><th>用电量(度)</th><th>用气量(方)</th><th>电费(元)</th><th>燃气费(元)</th><th>人工修正</th><th>编辑</th>
</tr>
{{range .List}}
<tr>
<td>{{.StatDate}}</td>
<td>{{printf "%.2f" .ElecAmount}}</td>
<td>{{printf "%.2f" .GasAmount}}</td>
<td>{{printf "%.2f" .ElecCost}}</td>
<td>{{printf "%.2f" .GasCost}}</td>
<td>{{if eq .IsManual 1}}是{{else}}否{{end}}</td>
<td>
<form action="/stat/save" method="post">
<input hidden name="date" value="{{.StatDate}}">
用电量:<input name="ele" step="0.01" value="{{printf "%.2f" .ElecAmount}}" style="width:80px"><br>
用气量:<input name="gas" step="0.01" value="{{printf "%.2f" .GasAmount}}" style="width:80px"><br>
备注:<input name="note" value="{{.FixNote}}"><br>
<button type="submit">保存修正</button>
</form>
</td>
</tr>
{{end}}
</table>
</body>
</html>
{{end}}

{{define "config"}}
<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>计费阶梯配置</title></head>
<body style="padding:20px">
<a href="/">← 返回首页</a>
<h2>长沙月度电费阶梯+燃气阶梯配置</h2>
<form action="/config/save" method="post">
<h3>月度电费阶梯（每月额度重置）</h3>
一档上限(度):<input name="et1l" value="{{.ElecTier1Limit}}"> 单价:<input name="et1p" value="{{.ElecTier1Price}}"><br>
二档累计上限(度):<input name="et2l" value="{{.ElecTier2Limit}}"> 单价:<input name="et2p" value="{{.ElecTier2Price}}"><br>
三档单价:<input name="et3p" value="{{.ElecTier3Price}}"><br>
<h3>燃气年度阶梯</h3>
一档上限(方):<input name="gt1l" value="{{.GasTier1Limit}}"> 单价:<input name="gt1p" value="{{.GasTier1Price}}"><br>
二档累计上限(方):<input name="gt2l" value="{{.GasTier2Limit}}"> 单价:<input name="gt2p" value="{{.GasTier2Price}}"><br>
三档单价:<input name="gt3p" value="{{.GasTier3Price}}"><br>
<button type="submit" style="margin-top:10px">保存配置并重新计算</button>
</form>
</body>
</html>
{{end}}

{{define "monthChart"}}
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>月度统计图表</title>
<script src="https://cdn.jsdelivr.net/npm/echarts@5.4.3/dist/echarts.min.js"></script>
</head>
<body style="padding:20px">
<a href="/">← 返回首页</a>
<h2>月度用电用气趋势图</h2>
<div id="chart" style="width:100%;height:450px;"></div>
<script>
var data = {{.JsonData}};
var xAxis = data.map(item=>item.day);
var elec = data.map(item=>item.ele);
var gas = data.map(item=>item.gas);
var chartDom = document.getElementById('chart');
var myChart = echarts.init(chartDom);
var opt = {
  tooltip:{},
  legend:{data:["用电量(度)","用气量(方)"]},
  xAxis:{type:'category',data:xAxis},
  yAxis:{type:'value'},
  series:[
    {name:"用电量(度)",type:"line",data:elec},
    {name:"用气量(方)",type:"line",data:gas}
  ]
};
myChart.setOption(opt);
</script>
</body>
</html>
{{end}}

{{define "log"}}
<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>修改日志</title></head>
<body style="padding:20px">
<a href="/">← 返回首页</a>
<h2>人工修改操作日志</h2>
<table border="1" cellpadding="6" cellspacing="0" width="100%">
<tr><th>日期</th><th>修改目标</th><th>类型</th><th>原值</th><th>新值</th><th>备注</th></tr>
{{range .List}}
<tr>
<td>{{.OptTime}}</td>
<td>{{.TargetDay}}</td>
<td>{{.OptType}}</td>
<td>{{.OldVal}}</td>
<td>{{.NewVal}}</td>
<td>{{.Note}}</td>
</tr>
{{end}}
</table>
</body>
</html>
{{end}}
`)
}

// 登录鉴权中间件
func authWrap(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("auth")
		if err != nil || c.Value != "ok" {
			tpl.ExecuteTemplate(w, "login", nil)
			return
		}
		h(w, r)
	}
}

func pageLogin(w http.ResponseWriter, r *http.Request) {
	tpl.ExecuteTemplate(w, "login", nil)
}

func apiLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	pwd := r.Form.Get("pwd")
	if pwd == AdminPassword {
		http.SetCookie(w, &http.Cookie{Name: "auth", Value: "ok", Path: "/", MaxAge: 86400})
		http.Redirect(w, r, "/", 302)
		return
	}
	fmt.Fprintln(w, "密码错误，返回<a href='/login'>登录</a>")
}

// ===================== 页面路由实现 =====================
func pageIndex(w http.ResponseWriter, r *http.Request) {
	tpl.ExecuteTemplate(w, "index", nil)
}

func pageBillList(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query(`
	SELECT br.id,br.bill_date,br.elec_balance,br.gas_balance,br.manual_fix,fr.file_name
	FROM balance_record br
	LEFT JOIN file_record fr ON br.file_id=fr.id
	ORDER BY br.bill_date DESC
	`)
	defer rows.Close()
	type billItem struct {
		ID          int
		BillDate    string
		ElecBalance float64
		GasBalance  float64
		ManualFix   int
		FileName    string
	}
	var list []billItem
	for rows.Next() {
		var i billItem
		_ = rows.Scan(&i.ID, &i.BillDate, &i.ElecBalance, &i.GasBalance, &i.ManualFix, &i.FileName)
		list = append(list, i)
	}
	tpl.ExecuteTemplate(w, "billList", map[string]any{"List": list})
}

func pageDayStat(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query(`SELECT * FROM daily_consume ORDER BY stat_date DESC`)
	defer rows.Close()
	var list []DailyConsume
	for rows.Next() {
		var d DailyConsume
		_ = rows.Scan(&d.ID, &d.StatDate, &d.ElecAmount, &d.GasAmount, &d.ElecCost, &d.GasCost, &d.IsManual, &d.FixNote)
		list = append(list, d)
	}
	tpl.ExecuteTemplate(w, "dayStat", map[string]any{"List": list})
}

func pageMonthChart(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query(`SELECT stat_date,elec_amount,gas_amount FROM daily_consume ORDER BY stat_date ASC`)
	defer rows.Close()
	type chartRow struct {
		day string
		ele float64
		gas float64
	}
	var data []chartRow
	for rows.Next() {
		var cr chartRow
		_ = rows.Scan(&cr.day, &cr.ele, &cr.gas)
		data = append(data, cr)
	}
	json, _ := json.Marshal(data)
	tpl.ExecuteTemplate(w, "monthChart", map[string]any{"JsonData": template.JS(json)})
}

func pageConfig(w http.ResponseWriter, r *http.Request) {
	cfg := getConfig()
	tpl.ExecuteTemplate(w, "config", cfg)
}

func pageFixLog(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query(`SELECT * FROM fix_log ORDER BY opt_time DESC`)
	defer rows.Close()
	var list []FixLog
	for rows.Next() {
		var l FixLog
		_ = rows.Scan(&l.ID, &l.TargetDay, &l.OptType, &l.OldVal, &l.NewVal, &l.Note, &l.OptTime)
		list = append(list, l)
	}
	tpl.ExecuteTemplate(w, "log", map[string]any{"List": list})
}

// ===================== 保存接口 =====================
func apiConfigSave(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	now := time.Now().Format("2006-01-02 15:04:05")
	parseFloat := func(s string) float64 {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	_, err := db.Exec(`UPDATE bill_config SET
	elec_tier1_limit=?,elec_tier1_price=?,elec_tier2_limit=?,elec_tier2_price=?,elec_tier3_price=?,
	gas_tier1_limit=?,gas_tier1_price=?,gas_tier2_limit=?,gas_tier2_price=?,gas_tier3_price=?,update_time=?`,
		parseFloat(r.Form.Get("et1l")), parseFloat(r.Form.Get("et1p")),
		parseFloat(r.Form.Get("et2l")), parseFloat(r.Form.Get("et2p")), parseFloat(r.Form.Get("et3p")),
		parseFloat(r.Form.Get("gt1l")), parseFloat(r.Form.Get("gt1p")),
		parseFloat(r.Form.Get("gt2l")), parseFloat(r.Form.Get("gt2p")), parseFloat(r.Form.Get("gt3p")), now)
	if err == nil {
		calcAllDailyConsume()
	}
	http.Redirect(w, r, "/config", 302)
}

func apiConsumeSave(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	day := r.Form.Get("date")
	eleStr := r.Form.Get("ele")
	gasStr := r.Form.Get("gas")
	note := r.Form.Get("note")
	ele, _ := strconv.ParseFloat(eleStr, 64)
	gas, _ := strconv.ParseFloat(gasStr, 64)
	// 记录修改日志
	var oldEle, oldGas float64
	_ = db.QueryRow(`SELECT elec_amount,gas_amount FROM daily_consume WHERE stat_date=?`, day).Scan(&oldEle, &oldGas)
	now := time.Now().Format("2006-01-02 15:04:05")
	_, _ = db.Exec(`INSERT INTO fix_log(target_day,opt_type,old_val,new_val,note,opt_time)
	VALUES (?,?,?,?,?,?)`, day, "每日用量", fmt.Sprintf("电%.2f气%.2f", oldEle, oldGas), fmt.Sprintf("电%.2f气%.2f", ele, gas), note, now)
	// 更新并标记人工修正
	_, _ = db.Exec(`REPLACE INTO daily_consume
	(stat_date,elec_amount,gas_amount,is_manual,fix_note)
	VALUES (?,?,?,1,?)`, day, ele, gas, note)
	http.Redirect(w, r, "/stat/day", 302)
}

func pageBillEdit(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.Atoi(idStr)
	var br BalanceRecord
	var fname string
	_ = db.QueryRow(`
	SELECT br.id,br.bill_date,br.elec_balance,br.gas_balance,fr.file_name
	FROM balance_record br LEFT JOIN file_record fr ON br.file_id=fr.id WHERE br.id=?`, id).
		Scan(&br.ID, &br.BillDate, &br.ElecBalance, &br.GasBalance, &fname)
	html := fmt.Sprintf(`
	<a href="/bill/list">返回</a>
	<h3>编辑账单 %s</h3>
	<form action="/bill/save" method="post">
	<input hidden name="id" value="%d">
	账单日期:<input name="date" value="%s"><br>
	电费余额:<input name="ele" value="%.2f"><br>
	燃气余额:<input name="gas" value="%.2f"><br>
	<button type="submit">保存修改</button>
	</form>
	`, fname, id, br.BillDate, br.ElecBalance, br.GasBalance)
	w.Write([]byte(html))
}

func apiBillSave(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id, _ := strconv.Atoi(r.Form.Get("id"))
	newDate := r.Form.Get("date")
	ele, _ := strconv.ParseFloat(r.Form.Get("ele"), 64)
	gas, _ := strconv.ParseFloat(r.Form.Get("gas"), 64)
	// 日志
	var oldDate string
	var oldE, oldG float64
	_ = db.QueryRow(`SELECT bill_date,elec_balance,gas_balance FROM balance_record WHERE id=?`, id).Scan(&oldDate, &oldE, &oldG)
	now := time.Now().Format("2006-01-02 15:04:05")
	_, _ = db.Exec(`INSERT INTO fix_log(target_day,opt_type,old_val,new_val,note,opt_time)
	VALUES (?,?,?,?,?,?)`, oldDate, "账单余额", fmt.Sprintf("date:%s 电%.2f气%.2f", oldDate, oldE, oldG), fmt.Sprintf("date:%s 电%.2f气%.2f", newDate, ele, gas), "人工修改账单", now)
	_, _ = db.Exec(`UPDATE balance_record SET bill_date=?,elec_balance=?,gas_balance=?,manual_fix=1 WHERE id=?`, newDate, ele, gas, id)
	calcAllDailyConsume()
	http.Redirect(w, r, "/bill/list", 302)
}
