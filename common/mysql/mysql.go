package mysql

import (
	"fmt"
	"log"
	"my-AIchat/config"
	"my-AIchat/model"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// ExternalDB 自然语言查库用的「外部数据库」独立连接。
// 与业务库 DB 完全解耦；只读账号从连接层杜绝写入；绝不执行 AutoMigrate。
var ExternalDB *gorm.DB

func InitDB() error {
	host := config.GetConfig().MysqlHost
	port := config.GetConfig().MysqlPort
	dbname := config.GetConfig().MysqlDatabaseName
	username := config.GetConfig().MysqlUser
	password := config.GetConfig().MysqlPassword
	charset := config.GetConfig().MysqlCharset

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		username, password, host, port, dbname, charset)

	var log logger.Interface
	if gin.Mode() == "debug" {
		log = logger.Default.LogMode(logger.Info)
	} else {
		log = logger.Default
	}

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       dsn,
		DefaultStringSize:         256,
		DisableDatetimePrecision:  true,
		DontSupportRenameIndex:    true,
		DontSupportRenameColumn:   true,
		SkipInitializeWithVersion: false,
	}), &gorm.Config{
		Logger: log,
	})
	if err != nil {
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	DB = db

	return migrate()
}

func migrate() error {
	err := DB.AutoMigrate(
		&model.User{},
		&model.Session{},
		&model.Message{},
		&model.UserMemory{},
	)
	if err != nil {
		return err
	}
	return nil
}

// InitExternalDB 初始化外部数据库（自然语言查库目标库）的独立连接。
// 与 InitDB 的区别：不执行任何 AutoMigrate（绝不往外部库写结构），
// 连接失败仅返回 error 由调用方决定不阻断主流程。
func InitExternalDB() error {
	conf := config.GetConfig().ExternalDBConfig
	dsn := externalDBDSN(conf.Dsn, conf.Schema)
	if dsn == "" {
		log.Println("[mysql] externalDB dsn 为空，跳过外部库初始化")
		return nil
	}

	idle := conf.MaxIdleConns
	if idle <= 0 {
		idle = 5
	}
	open := conf.MaxOpenConns
	if open <= 0 {
		open = 20
	}

	var gormLog logger.Interface
	if gin.Mode() == "debug" {
		gormLog = logger.Default.LogMode(logger.Info)
	} else {
		gormLog = logger.Default
	}

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       dsn,
		DefaultStringSize:         256,
		DisableDatetimePrecision:  true,
		DontSupportRenameIndex:    true,
		DontSupportRenameColumn:   true,
		SkipInitializeWithVersion: false,
	}), &gorm.Config{
		Logger: gormLog,
	})
	if err != nil {
		return fmt.Errorf("externalDB connect failed: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("externalDB get sql.DB failed: %v", err)
	}
	sqlDB.SetMaxIdleConns(idle)
	sqlDB.SetMaxOpenConns(open)
	sqlDB.SetConnMaxLifetime(time.Hour)

	ExternalDB = db
	log.Println("[mysql] externalDB init success")
	return nil
}

// externalDBDSN 把配置的 Schema（库名）拼进 DSN，使连接默认选中目标库。
// 仅用 DSN 而不带库名会导致执行 SELECT 时报 "Error 1046 No database selected"。
// 规则：
//   - Schema 为空：原样返回 DSN；
//   - DSN 末尾已是 "/"：直接追加 Schema（幂等，已为 "/Schema" 则不重复）；
//   - DSN 已带库名但与 Schema 不同：统一替换为 Schema，保证与读目录用的库一致；
//   - 保留 DSN 中已有的查询参数（?...）。
func externalDBDSN(dsn, schema string) string {
	if schema == "" {
		return dsn
	}
	base := dsn
	var qs string
	if i := strings.Index(dsn, "?"); i >= 0 {
		base, qs = dsn[:i], dsn[i:]
	}
	if !strings.HasSuffix(base, "/") {
		// 已带库名（如 /otherdb）：整体替换为 Schema
		if idx := strings.LastIndex(base, "/"); idx >= 0 {
			base = base[:idx+1] + schema
			return base + qs
		}
		base += "/"
	}
	if !strings.HasSuffix(base, "/"+schema) {
		base += schema
	}
	return base + qs
}
