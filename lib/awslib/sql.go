package awslib

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "github.com/microsoft/go-mssqldb"

	"github.com/loopsrage/gotools/lib/sqllib"
)

// AwsSqlConfig configures AWS RDS / SQL Server connection parameters and region metadata.
type AwsSqlConfig struct {
	Driver     string
	Server     string
	Database   string
	Username   string
	Password   string
	RegionName string
}

// ConStr formats the ODBC connection string matching toolset's AwsSqlConfig.con_str().
func (c AwsSqlConfig) ConStr() string {
	return fmt.Sprintf(
		"Driver=%s;Server=tcp:%s,1433;Database=%s;UID=%s;PWD=%s;TrustServerCertificate=yes;Connection Timeout=30;",
		c.Driver,
		c.Server,
		c.Database,
		c.Username,
		c.Password,
	)
}

// SQLServerDSN builds a standard Go sqlserver:// URI DSN for github.com/microsoft/go-mssqldb.
func (c AwsSqlConfig) SQLServerDSN() string {
	u := &url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword(c.Username, c.Password),
		Host:   fmt.Sprintf("%s:1433", c.Server),
	}
	q := u.Query()
	q.Set("database", c.Database)
	q.Set("TrustServerCertificate", "true")
	q.Set("connection timeout", "30")
	u.RawQuery = q.Encode()
	return u.String()
}

// AwsSql wraps a database/sql.DB handle (backed by go-mssqldb by default) for AWS RDS schema inspection and queries.
type AwsSql struct {
	DB     *sql.DB
	Config AwsSqlConfig
}

// NewAwsSql opens a database/sql handle using driverName (default "sqlserver") and dsn (default cfg.SQLServerDSN()).
func NewAwsSql(driverName, dsn string, cfg AwsSqlConfig) (*AwsSql, error) {
	if driverName == "" {
		driverName = "sqlserver"
	}
	if dsn == "" {
		dsn = cfg.SQLServerDSN()
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	return &AwsSql{DB: db, Config: cfg}, nil
}

// NewAwsSqlWithDB wraps an existing *sql.DB connection pool.
func NewAwsSqlWithDB(db *sql.DB, cfg AwsSqlConfig) *AwsSql {
	return &AwsSql{DB: db, Config: cfg}
}

func (a *AwsSql) InspectTables(ctx context.Context, schema string) ([]string, error) {
	return sqllib.InspectDatabaseTables(ctx, a.DB, sqllib.DialectMSSQL, schema)
}

func (a *AwsSql) InspectColumns(ctx context.Context, table, schema string) ([]sqllib.ColumnInfo, error) {
	return sqllib.InspectDatabaseColumns(ctx, a.DB, sqllib.DialectMSSQL, table, schema)
}

func (a *AwsSql) TableMetadata(ctx context.Context, table, schema string) (*sqllib.TableMetadata, error) {
	cols, err := a.InspectColumns(ctx, table, schema)
	if err != nil {
		return nil, err
	}
	return &sqllib.TableMetadata{
		Name:    table,
		Schema:  schema,
		Columns: cols,
	}, nil
}

func (a *AwsSql) BuildSearchQuery(opts sqllib.SearchOptions) (string, []interface{}, error) {
	dialect := sqllib.DetectDialect(a.DB, sqllib.DialectMSSQL)
	return sqllib.BuildCloudSearchQuery(dialect, true, opts)
}

func (a *AwsSql) Search(ctx context.Context, opts sqllib.SearchOptions) ([]map[string]interface{}, error) {
	query, args, err := a.BuildSearchQuery(opts)
	if err != nil {
		return nil, err
	}
	return sqllib.ExecuteCloudSearch(ctx, a.DB, query, args...)
}

func (a *AwsSql) Close() error {
	if a.DB != nil {
		return a.DB.Close()
	}
	return nil
}
