-- 初始 schema：用户、文件、账单、统计、配置、规则、网盘绑定、操作日志
-- 启用外键（由连接字符串或 PRAGMA 控制，这里补充约束）

CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS file_record (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL,
    file_md5     TEXT NOT NULL,
    file_name    TEXT,
    file_path    TEXT,
    scan_time    TEXT,
    is_processed INTEGER DEFAULT 0,
    is_abnormal  INTEGER DEFAULT 0,
    source       TEXT DEFAULT 'local',
    err_note     TEXT,
    UNIQUE (user_id, file_md5)
);
CREATE INDEX IF NOT EXISTS idx_file_user ON file_record(user_id);
CREATE INDEX IF NOT EXISTS idx_file_abnormal ON file_record(is_abnormal);

CREATE TABLE IF NOT EXISTS balance_record (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL,
    file_id       INTEGER,
    bill_date     TEXT NOT NULL,
    elec_balance  REAL DEFAULT 0,
    gas_balance   REAL DEFAULT 0,
    water_balance REAL DEFAULT 0,
    ocr_text      TEXT,
    manual_fix    INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_balance_user_date ON balance_record(user_id, bill_date);

CREATE TABLE IF NOT EXISTS daily_consume (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL,
    stat_date    TEXT NOT NULL,
    elec_amount  REAL DEFAULT 0,
    gas_amount   REAL DEFAULT 0,
    water_amount REAL DEFAULT 0,
    elec_cost    REAL DEFAULT 0,
    gas_cost     REAL DEFAULT 0,
    water_cost   REAL DEFAULT 0,
    is_manual    INTEGER DEFAULT 0,
    fix_note     TEXT,
    UNIQUE (user_id, stat_date)
);
CREATE INDEX IF NOT EXISTS idx_consume_user_date ON daily_consume(user_id, stat_date);

-- 阶梯计费配置：每个用户每种能源一行，tiers 以 JSON 存储便于扩展（含用水预留）
CREATE TABLE IF NOT EXISTS pricing_config (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL,
    utility_type TEXT NOT NULL,        -- electricity | gas | water
    cycle        TEXT NOT NULL,        -- monthly | yearly
    tiers_json   TEXT NOT NULL,        -- [{"limit":..,"price":..},...]
    update_time  TEXT NOT NULL,
    UNIQUE (user_id, utility_type)
);

-- 日期提取规则：对文件名 / 图片内容分别配置，带权重
CREATE TABLE IF NOT EXISTS date_rule (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id  INTEGER NOT NULL,
    name     TEXT NOT NULL,
    source   TEXT NOT NULL,  -- filename | content
    pattern  TEXT NOT NULL,  -- 正则，前 3 个捕获组为 年/月/日
    priority INTEGER DEFAULT 0,
    enabled  INTEGER DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_rule_user ON date_rule(user_id);

-- 百度网盘绑定：token 加密存储，临时授权到期自动清理
CREATE TABLE IF NOT EXISTS baidu_binding (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id           INTEGER NOT NULL UNIQUE,
    baidu_uid         TEXT,
    access_token_enc  BLOB,
    refresh_token_enc BLOB,
    expire_at         TEXT,
    auth_type         TEXT DEFAULT 'permanent', -- permanent | temporary
    temp_until        TEXT,                     -- 临时授权到期时间
    created_at        TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS fix_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL,
    target_day TEXT,
    opt_type   TEXT,
    old_val    TEXT,
    new_val    TEXT,
    note       TEXT,
    opt_time   TEXT
);
CREATE INDEX IF NOT EXISTS idx_log_user_time ON fix_log(user_id, opt_time);
