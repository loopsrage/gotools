package azurelib

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "github.com/microsoft/go-mssqldb"

	"github.com/loopsrage/gotools/lib/sqllib"
)

// AzureSqlConfig configures Azure SQL / SQL Server connection parameters.
type AzureSqlConfig struct {
	Driver   string
	Server   string
	Database string
	Username string
	Password string
}

// ConStr formats the ODBC connection string matching toolset's AzureSqlConfig.con_str().
func (c AzureSqlConfig) ConStr() string {
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
func (c AzureSqlConfig) SQLServerDSN() string {
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

// AzureSql wraps a database/sql.DB handle (backed by go-mssqldb by default) for Azure SQL schema inspection and queries.
type AzureSql struct {
	DB     *sql.DB
	Config AzureSqlConfig
}

// NewAzureSql opens a database/sql handle using driverName (default "sqlserver") and dsn (default cfg.SQLServerDSN()).
func NewAzureSql(driverName, dsn string, cfg AzureSqlConfig) (*AzureSql, error) {
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
	return &AzureSql{DB: db, Config: cfg}, nil
}

// NewAzureSqlWithDB wraps an existing *sql.DB connection pool.
func NewAzureSqlWithDB(db *sql.DB, cfg AzureSqlConfig) *AzureSql {
	return &AzureSql{DB: db, Config: cfg}
}

func (a *AzureSql) InspectTables(ctx context.Context, schema string) ([]string, error) {
	return sqllib.InspectDatabaseTables(ctx, a.DB, sqllib.DialectMSSQL, schema)
}

func (a *AzureSql) InspectColumns(ctx context.Context, table, schema string) ([]sqllib.ColumnInfo, error) {
	return sqllib.InspectDatabaseColumns(ctx, a.DB, sqllib.DialectMSSQL, table, schema)
}

func (a *AzureSql) TableMetadata(ctx context.Context, table, schema string) (*sqllib.TableMetadata, error) {
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

func (a *AzureSql) BuildSearchQuery(opts sqllib.SearchOptions) (string, []interface{}, error) {
	dialect := sqllib.DetectDialect(a.DB, sqllib.DialectMSSQL)
	return sqllib.BuildCloudSearchQuery(dialect, true, opts)
}

func (a *AzureSql) Search(ctx context.Context, opts sqllib.SearchOptions) ([]map[string]interface{}, error) {
	query, args, err := a.BuildSearchQuery(opts)
	if err != nil {
		return nil, err
	}
	return sqllib.ExecuteCloudSearch(ctx, a.DB, query, args...)
}

func (a *AzureSql) Close() error {
	if a.DB != nil {
		return a.DB.Close()
	}
	return nil
}
