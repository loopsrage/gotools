package gclib

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/loopsrage/gotools/lib/sqllib"
)

// GcpSqlConfig configures Cloud SQL (PostgreSQL / ODBC) connection parameters.
type GcpSqlConfig struct {
	Driver                 string
	Server                 string
	Database               string
	Username               string
	Password               string
	InstanceConnectionName string
}

// ConStr formats the ODBC connection string matching toolset's GcpSqlConfig.con_str().
func (c GcpSqlConfig) ConStr() string {
	return fmt.Sprintf(
		"Driver=%s;Server=tcp:%s;Database=%s;UID=%s;PWD=%s;Connection Timeout=30;",
		c.Driver,
		c.Server,
		c.Database,
		c.Username,
		c.Password,
	)
}

// PostgresDSN builds a standard PostgreSQL URI DSN for github.com/jackc/pgx/v5/stdlib ("pgx").
func (c GcpSqlConfig) PostgresDSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.Username, c.Password),
		Host:   c.Server,
		Path:   c.Database,
	}
	q := u.Query()
	q.Set("connect_timeout", "30")
	u.RawQuery = q.Encode()
	return u.String()
}

// GcpSql wraps a database/sql.DB handle (backed by pgx/v5/stdlib by default) for Cloud SQL inspection and queries.
type GcpSql struct {
	DB     *sql.DB
	Config GcpSqlConfig
}

// NewGcpSql opens a database/sql handle using driverName (default "pgx") and dsn (default cfg.PostgresDSN()).
func NewGcpSql(driverName, dsn string, cfg GcpSqlConfig) (*GcpSql, error) {
	if driverName == "" {
		driverName = "pgx"
	}
	if dsn == "" {
		dsn = cfg.PostgresDSN()
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	return &GcpSql{DB: db, Config: cfg}, nil
}

// NewGcpSqlWithDB wraps an existing *sql.DB connection pool.
func NewGcpSqlWithDB(db *sql.DB, cfg GcpSqlConfig) *GcpSql {
	return &GcpSql{DB: db, Config: cfg}
}

func (g *GcpSql) InspectTables(ctx context.Context, schema string) ([]string, error) {
	return sqllib.InspectDatabaseTables(ctx, g.DB, sqllib.DialectPostgres, schema)
}

func (g *GcpSql) InspectColumns(ctx context.Context, table, schema string) ([]sqllib.ColumnInfo, error) {
	return sqllib.InspectDatabaseColumns(ctx, g.DB, sqllib.DialectPostgres, table, schema)
}

func (g *GcpSql) TableMetadata(ctx context.Context, table, schema string) (*sqllib.TableMetadata, error) {
	cols, err := g.InspectColumns(ctx, table, schema)
	if err != nil {
		return nil, err
	}
	return &sqllib.TableMetadata{
		Name:    table,
		Schema:  schema,
		Columns: cols,
	}, nil
}

func (g *GcpSql) BuildSearchQuery(opts sqllib.SearchOptions) (string, []interface{}, error) {
	dialect := sqllib.DetectDialect(g.DB, sqllib.DialectPostgres)
	return sqllib.BuildCloudSearchQuery(dialect, false, opts)
}

func (g *GcpSql) Search(ctx context.Context, opts sqllib.SearchOptions) ([]map[string]interface{}, error) {
	query, args, err := g.BuildSearchQuery(opts)
	if err != nil {
		return nil, err
	}
	return sqllib.ExecuteCloudSearch(ctx, g.DB, query, args...)
}

func (g *GcpSql) Close() error {
	if g.DB != nil {
		return g.DB.Close()
	}
	return nil
}
