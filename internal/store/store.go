package store

// 本地 SQLite 数据库（modernc.org/sqlite，纯 Go）。目前存操作记录；
// 表结构用 PRAGMA user_version 做迁移。

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	_ "modernc.org/sqlite"
)

// DB 是打开的本地数据库，并发安全。
type DB struct {
	db *sql.DB
}

// migrations 按顺序执行，第 i 条把 user_version 从 i 升到 i+1。
// 已发布的迁移不能改，只能追加。
var migrations = []string{
	`CREATE TABLE calls (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		request_id    TEXT    NOT NULL,
		role          TEXT    NOT NULL,            -- target 被控端 / source 控制端
		operator_type TEXT    NOT NULL,            -- ai / human
		via           TEXT    NOT NULL DEFAULT '', -- cli / mcp / ui
		session       TEXT    NOT NULL DEFAULT '',
		caller_owner  TEXT    NOT NULL DEFAULT '',
		caller_device TEXT    NOT NULL DEFAULT '',
		caller_kind   TEXT    NOT NULL DEFAULT '',
		device        TEXT    NOT NULL DEFAULT '', -- 被控设备（设备 ID 或名称）
		tool          TEXT    NOT NULL,
		args          TEXT    NOT NULL DEFAULT '', -- 已脱敏的参数摘要
		status        TEXT    NOT NULL,            -- running / ok / 错误码 / interrupted
		started_at    INTEGER NOT NULL,            -- Unix 毫秒
		ended_at      INTEGER NOT NULL DEFAULT 0,
		duration_ms   INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX calls_started ON calls(started_at);
	CREATE INDEX calls_request ON calls(request_id);`,
}

// 状态取值之外的错误码就是协议错误码（cancelled、timeout 等）。
const (
	StatusRunning     = "running"
	StatusOK          = "ok"
	StatusInterrupted = "interrupted" // 节点退出时调用还在执行
)

// 记录的角色。同一次调用在被控端和控制端各有一条，request_id 相同。
const (
	RoleTarget = protocol.RoleTarget
	RoleSource = protocol.RoleSource
)

// retention 是记录的保留时间，超过的在打开数据库时删除。
const retention = 90 * 24 * time.Hour

// Open 打开（必要时创建）path 处的数据库并迁移到最新结构。
func Open(path string) (*DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库 %s 失败: %w", path, err)
	}
	// 单连接：写入串行，避免 SQLITE_BUSY；记录写入量很小。
	db.SetMaxOpenConns(1)
	d := &DB{db: db}
	if err := d.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`DELETE FROM calls WHERE started_at < ?`, time.Now().Add(-retention).UnixMilli()); err != nil {
		db.Close()
		return nil, fmt.Errorf("清理过期记录失败: %w", err)
	}
	return d, nil
}

// Close 关闭数据库。
func (d *DB) Close() error { return d.db.Close() }

func (d *DB) migrate() error {
	var ver int
	if err := d.db.QueryRow(`PRAGMA user_version`).Scan(&ver); err != nil {
		return fmt.Errorf("读取数据库版本失败: %w", err)
	}
	if ver > len(migrations) {
		return fmt.Errorf("数据库版本 %d 比本程序（%d）新，请升级 DeskBridge", ver, len(migrations))
	}
	for i := ver; i < len(migrations); i++ {
		tx, err := d.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("数据库迁移 %d 失败: %w", i+1, err)
		}
		// PRAGMA 不支持参数绑定，版本号是程序内常量。
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Record 是一条操作记录，字段即跨设备查询接口的线上格式（protocol.Record）。
type Record = protocol.Record

// Begin 写入一条状态为 running 的记录，返回行号。
func (d *DB) Begin(r Record) (int64, error) {
	if r.StartedAt == 0 {
		r.StartedAt = time.Now().UnixMilli()
	}
	res, err := d.db.Exec(`INSERT INTO calls
		(request_id, role, operator_type, via, session, caller_owner, caller_device, caller_kind,
		 device, tool, args, status, started_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.RequestID, r.Role, r.OperatorType, r.Via, r.Session, r.CallerOwner, r.CallerDevice, r.CallerKind,
		r.Device, r.Tool, r.Args, StatusRunning, r.StartedAt)
	if err != nil {
		return 0, fmt.Errorf("写入操作记录失败: %w", err)
	}
	return res.LastInsertId()
}

// Finish 结束一条记录，写入结果状态和耗时。
func (d *DB) Finish(id int64, status string, endedAt time.Time, durationMS int64) error {
	_, err := d.db.Exec(`UPDATE calls SET status=?, ended_at=?, duration_ms=? WHERE id=?`,
		status, endedAt.UnixMilli(), durationMS, id)
	if err != nil {
		return fmt.Errorf("更新操作记录失败: %w", err)
	}
	return nil
}

// MarkInterrupted 把仍是 running 的记录标为 interrupted。节点启动时调用：
// 上次运行中途退出（崩溃、被结束）时，那些调用不会再有结果。
func (d *DB) MarkInterrupted() (int64, error) {
	res, err := d.db.Exec(`UPDATE calls SET status=?, ended_at=? WHERE status=?`,
		StatusInterrupted, time.Now().UnixMilli(), StatusRunning)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Filter 是查询条件，零值字段不参与筛选。结果按行号从新到旧排列。
type Filter struct {
	Since        int64  // started_at >= Since（Unix 毫秒）
	Until        int64  // started_at < Until
	OperatorType string // ai / human
	Tool         string // 精确匹配；以 * 结尾为前缀匹配，如 fs.*
	Status       string // running / ok / failed（除这三者以及 cancelled 之外的结果）/ 具体错误码
	Role         string // target / source
	RequestID    string
	BeforeID     int64 // 只返回行号小于它的记录，用于翻页
	Limit        int   // 默认 100，最大 1000
}

// 查询条数。
const (
	DefaultLimit = 100
	MaxLimit     = 1000
)

// Query 按条件查询记录。
func (d *DB) Query(f Filter) ([]Record, error) {
	var where []string
	var args []any
	add := func(cond string, v ...any) {
		where = append(where, cond)
		args = append(args, v...)
	}
	if f.Since > 0 {
		add("started_at >= ?", f.Since)
	}
	if f.Until > 0 {
		add("started_at < ?", f.Until)
	}
	if f.OperatorType != "" {
		add("operator_type = ?", f.OperatorType)
	}
	if f.Tool != "" {
		if prefix, ok := strings.CutSuffix(f.Tool, "*"); ok {
			add("substr(tool, 1, ?) = ?", len(prefix), prefix)
		} else {
			add("tool = ?", f.Tool)
		}
	}
	switch f.Status {
	case "":
	case "failed":
		add("status NOT IN (?,?,?,?)", StatusRunning, StatusOK, "cancelled", StatusInterrupted)
	default:
		add("status = ?", f.Status)
	}
	if f.Role != "" {
		add("role = ?", f.Role)
	}
	if f.RequestID != "" {
		add("request_id = ?", f.RequestID)
	}
	if f.BeforeID > 0 {
		add("id < ?", f.BeforeID)
	}
	limit := f.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	q := `SELECT id, request_id, role, operator_type, via, session, caller_owner, caller_device, caller_kind,
		device, tool, args, status, started_at, ended_at, duration_ms FROM calls`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := d.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("查询操作记录失败: %w", err)
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.RequestID, &r.Role, &r.OperatorType, &r.Via, &r.Session,
			&r.CallerOwner, &r.CallerDevice, &r.CallerKind, &r.Device, &r.Tool, &r.Args,
			&r.Status, &r.StartedAt, &r.EndedAt, &r.DurationMS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
