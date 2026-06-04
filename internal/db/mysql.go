package db

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

func NewMySQL(databaseURL string) (*sqlx.DB, error) {
	dsn, err := normalizeMySQLDSN(databaseURL)
	if err != nil {
		return nil, err
	}

	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	return db, nil
}

func normalizeMySQLDSN(rawDSN string) (string, error) {
	if rawDSN == "" {
		return "", fmt.Errorf("empty DSN")
	}

	if strings.Contains(rawDSN, "@tcp(") {
		return rawDSN, nil
	}

	parsed, err := url.Parse(rawDSN)
	if err != nil {
		return "", err
	}

	switch parsed.Scheme {
	case "mysql":
	default:
		return "", fmt.Errorf("unsupported database scheme: %s", parsed.Scheme)
	}

	user := parsed.User.Username()
	password, _ := parsed.User.Password()
	dbName := strings.TrimPrefix(parsed.Path, "/")
	if user == "" || parsed.Host == "" || dbName == "" {
		return "", fmt.Errorf("invalid DSN: %s", rawDSN)
	}

	mysqlCfg := mysql.Config{
		User:                 user,
		Passwd:               password,
		Net:                  "tcp",
		Addr:                 parsed.Host,
		DBName:               dbName,
		Loc:                  time.Local,
		ParseTime:            true,
		AllowNativePasswords: true,
		Params: map[string]string{
			"charset": "utf8mb4",
		},
	}

	return mysqlCfg.FormatDSN(), nil
}
