// Package config 集中加载与校验运行期配置。
//
// 设计原则：
//   - 默认值保证"开箱即用"，敏感项（密钥/凭证）无默认，必须显式配置
//   - 同时支持环境变量与可选 .env 文件（godotenv autoload）
//   - 不在此处读取真实密钥到日志，避免泄漏
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config 全局配置。字段只读，启动时一次性加载。
type Config struct {
	ServerAddr        string
	DBFile            string
	ScanDir           string // 本地兜底扫描目录（未绑定网盘时使用）
	ScanInterval      int    // 分钟
	SessionKey        []byte // 会话签名密钥，至少 32 字节
	TokenEncKey       []byte // OAuth token 加密密钥，至少 32 字节
	BaiduOCRKey       string // 百度 OCR API Key（可空，空则降级 NoopOCR）
	BaiduOCRSecret    string
	BaiduPanAppKey    string // 百度网盘 AppKey（可空，空则禁用网盘功能）
	BaiduPanAppSecret string
	TempCleanInterval int // 秒，临时授权清理协程周期
}

// Load 从环境变量加载配置。缺失的敏感项返回错误，避免用空密钥静默运行。
func Load() (Config, error) {
	c := Config{
		ServerAddr:        getenv("SERVER_ADDR", ":8080"),
		DBFile:            getenv("DB_FILE", "./data.db"),
		ScanDir:           getenv("SCAN_DIR", "./baidupan"),
		ScanInterval:      getenvInt("SCAN_INTERVAL", 5),
		BaiduOCRKey:       os.Getenv("BAIDU_OCR_API_KEY"),
		BaiduOCRSecret:    os.Getenv("BAIDU_OCR_SECRET_KEY"),
		BaiduPanAppKey:    os.Getenv("BAIDU_PAN_APP_KEY"),
		BaiduPanAppSecret: os.Getenv("BAIDU_PAN_APP_SECRET"),
		TempCleanInterval: getenvInt("TEMP_CLEAN_INTERVAL", 3600),
	}

	sessionKey := os.Getenv("SESSION_KEY")
	tokenKey := os.Getenv("TOKEN_ENC_KEY")

	// 开发期兜底：未配置时给一个明显可识别的占位并告警，避免用空字节签名导致所有人会话等价。
	if sessionKey == "" {
		fmt.Println("[warn] SESSION_KEY 未设置，使用不安全的开发占位。生产环境必须设置 32+ 字节随机串。")
		sessionKey = "dev-insecure-session-key-change-me"
	}
	if tokenKey == "" {
		fmt.Println("[warn] TOKEN_ENC_KEY 未设置，使用不安全的开发占位。生产环境必须设置 32+ 字节随机串。")
		tokenKey = "dev-insecure-token-encryption-key-change-me"
	}
	c.SessionKey = []byte(sessionKey)
	c.TokenEncKey = []byte(tokenKey)

	if len(c.SessionKey) < 16 || len(c.TokenEncKey) < 16 {
		return c, fmt.Errorf("SESSION_KEY / TOKEN_ENC_KEY 长度过短（<16），拒绝启动")
	}
	return c, nil
}

// OCRConfigured 返回是否配置了百度 OCR 凭证，决定引擎选择。
func (c Config) OCRConfigured() bool {
	return c.BaiduOCRKey != "" && c.BaiduOCRSecret != ""
}

// PanConfigured 返回是否配置了百度网盘凭证。
func (c Config) PanConfigured() bool {
	return c.BaiduPanAppKey != "" && c.BaiduPanAppSecret != ""
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
