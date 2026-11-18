package sqllib

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Dialect identifies the SQL wire dialect for query generation and schema inspection.
type Dialect string

const (
	DialectPostgres Dialect = "postgres"
	DialectMSSQL    Dialect = "mssql"
	DialectSQLite   Dialect = "sqlite"
	DialectMySQL    Dialect = "mysql"
)

// ColumnInfo describes a database table column discovered via schema inspection.
type ColumnInfo struct {
	Name       string  `json:"name"`
	DataType   string  `json:"type"`
	Nullable   bool    `json:"nullable"`
	Default    *string `json:"default,omitempty"`
	PrimaryKey bool    `json:"primary_key,omitempty"`
}

// TableMetadata holds inspected metadata for a database table.
type TableMetadata struct {
	Name    string       `json:"name"`
	Schema  string       `json:"schema,omitempty"`
	Columns []ColumnInfo `json:"columns"`
}

// SearchOptions configures keyset-paginated Search queries across GcpSql, AzureSql, and AwsSql.
type SearchOptions struct {
	Table      string
	PrimaryKey string
	Columns    string
	OrderCol   string
	Joins      []string
	Filters    map[string]interface{}
	Limit      *int
	LastItem   interface{}
}

// IntPtr returns a pointer to n for configuring SearchOptions.Limit.
func IntPtr(n int) *int {
	return &n
}

// DefaultSearchOptions returns SearchOptions initialized with PrimaryKey="id", Columns="*", Limit=100, LastItem=0.
func DefaultSearchOptions(table string) SearchOptions {
	return SearchOptions{
		Table:      table,
		PrimaryKey: "id",
		Columns:    "*",
		Limit:      IntPtr(100),
		LastItem:   0,
	}
}

var safeIdentRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`)

func validateIdent(ident string) error {
	clean := strings.TrimSpace(ident)
	if clean == "" || !safeIdentRe.MatchString(clean) {
		return fmt.Errorf("sqllib: unsafe or empty SQL identifier %q", ident)
	}
	return nil
}

func validateColumnsExpr(cols string) error {
	trimmed := strings.TrimSpace(cols)
	if trimmed == "*" {
		return nil
	}
	if strings.ContainsAny(trimmed, ";'\"`") || strings.Contains(trimmed, "--") || strings.Contains(trimmed, "/*") {
		return fmt.Errorf("sqllib: unsafe columns projection %q", cols)
	}
	parts := strings.Split(trimmed, ",")
	for _, p := range parts {
		tok := strings.TrimSpace(p)
		if tok == "" {
			return fmt.Errorf("sqllib: empty column in projection %q", cols)
		}
	}
	return nil
}

// DetectDialect inspects db.Driver()'s concrete Go type name to identify the active SQL dialect,
// falling back to fallbackDialect when unknown.
func DetectDialect(db *sql.DB, fallbackDialect Dialect) Dialect {
	if db == nil {
		return fallbackDialect
	}
	drvName := strings.ToLower(fmt.Sprintf("%T", db.Driver()))
	switch {
	case strings.Contains(drvName, "sqlite"):
		return DialectSQLite
	case strings.Contains(drvName, "mysql"):
		return DialectMySQL
	case strings.Contains(drvName, "pq") || strings.Contains(drvName, "pgx") || strings.Contains(drvName, "postgres"):
		return DialectPostgres
	case strings.Contains(drvName, "mssql") || strings.Contains(drvName, "sqlserver"):
		return DialectMSSQL
	default:
		return fallbackDialect
	}
}

func placeholder(dialect Dialect, index1Based int) string {
	switch dialect {
	case DialectPostgres:
		return fmt.Sprintf("$%d", index1Based)
	case DialectMSSQL:
		return fmt.Sprintf("@p%d", index1Based)
	default:
		return "?"
	}
}

func quoteIdent(ident string, bracketQuote bool) string {
	if !bracketQuote {
		return ident
	}
	if strings.Contains(ident, ".") {
		return ident
	}
	return "[" + ident + "]"
}

// BuildCloudSearchQuery compiles SearchOptions into a parameterized SQL statement and bind slice.
// When bracketQuote is true (AzureSql / AwsSql), table and unqualified column identifiers are [bracketed],
// and DialectMSSQL emits TOP N whereas Postgres/SQLite/MySQL emit trailing LIMIT N.
func BuildCloudSearchQuery(dialect Dialect, bracketQuote bool, opts SearchOptions) (string, []interface{}, error) {
	if err := validateIdent(opts.Table); err != nil {
		return "", nil, err
	}
	pk := opts.PrimaryKey
	if pk == "" {
		pk = "id"
	}
	if err := validateIdent(pk); err != nil {
		return "", nil, err
	}
	cols := opts.Columns
	if cols == "" {
		cols = "*"
	}
	if err := validateColumnsExpr(cols); err != nil {
		return "", nil, err
	}
	if opts.OrderCol != "" {
		if err := validateIdent(opts.OrderCol); err != nil {
			return "", nil, err
		}
	}
	if opts.Limit != nil && *opts.Limit < 0 {
		return "", nil, fmt.Errorf("sqllib: negative limit %d", *opts.Limit)
	}

	var parts []string
	if dialect == DialectMSSQL && opts.Limit != nil {
		parts = append(parts, fmt.Sprintf("SELECT TOP %d %s FROM %s AS base", *opts.Limit, cols, quoteIdent(opts.Table, bracketQuote)))
	} else {
		parts = append(parts, fmt.Sprintf("SELECT %s FROM %s AS base", cols, quoteIdent(opts.Table, bracketQuote)))
	}

	for _, j := range opts.Joins {
		trimmed := strings.TrimSpace(j)
		if trimmed == "" {
			continue
		}
		if strings.ContainsAny(trimmed, ";") || strings.Contains(trimmed, "--") || strings.Contains(trimmed, "/*") {
			return "", nil, fmt.Errorf("sqllib: unsafe join clause %q", j)
		}
		parts = append(parts, trimmed)
	}

	var args []interface{}
	lastItem := opts.LastItem
	if lastItem == nil {
		lastItem = 0
	}
	args = append(args, lastItem)
	parts = append(parts, fmt.Sprintf("WHERE base.%s > %s", quoteIdent(pk, bracketQuote), placeholder(dialect, len(args))))

	if len(opts.Filters) > 0 {
		keys := make([]string, 0, len(opts.Filters))
		for k := range opts.Filters {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			if err := validateIdent(k); err != nil {
				return "", nil, err
			}
			var colExpr string
			if strings.Contains(k, ".") {
				colExpr = k
			} else {
				colExpr = "base." + quoteIdent(k, bracketQuote)
			}

			val := opts.Filters[k]
			rv := reflect.ValueOf(val)
			if val != nil && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) && rv.Type() != reflect.TypeOf([]byte(nil)) {
				if rv.Len() == 0 {
					return "", nil, fmt.Errorf("sqllib: empty slice in filter %q", k)
				}
				phs := make([]string, 0, rv.Len())
				for i := 0; i < rv.Len(); i++ {
					args = append(args, rv.Index(i).Interface())
					phs = append(phs, placeholder(dialect, len(args)))
				}
				parts = append(parts, fmt.Sprintf("AND %s IN (%s)", colExpr, strings.Join(phs, ", ")))
			} else {
				args = append(args, val)
				parts = append(parts, fmt.Sprintf("AND %s = %s", colExpr, placeholder(dialect, len(args))))
			}
		}
	}

	if opts.OrderCol != "" {
		if strings.Contains(opts.OrderCol, ".") {
			parts = append(parts, fmt.Sprintf("ORDER BY %s", opts.OrderCol))
		} else {
			parts = append(parts, fmt.Sprintf("ORDER BY %s", quoteIdent(opts.OrderCol, bracketQuote)))
		}
	}

	if dialect != DialectMSSQL && opts.Limit != nil {
		parts = append(parts, fmt.Sprintf("LIMIT %d", *opts.Limit))
	}

	return strings.Join(parts, " "), args, nil
}

// ExecuteCloudSearch runs a compiled query and scans every row into map[string]interface{}.
func ExecuteCloudSearch(ctx context.Context, db *sql.DB, query string, args ...interface{}) ([]map[string]interface{}, error) {
	if db == nil {
		return nil, errors.New("sqllib: database handle is nil")
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for rows.Next() {
		rawVals := make([]interface{}, len(colNames))
		scanPtrs := make([]interface{}, len(colNames))
		for i := range rawVals {
			scanPtrs[i] = &rawVals[i]
		}
		if err := rows.Scan(scanPtrs...); err != nil {
			return nil, err
		}
		rowMap := make(map[string]interface{}, len(colNames))
		for i, col := range colNames {
			v := rawVals[i]
			if b, ok := v.([]byte); ok && utf8.Valid(b) {
				rowMap[col] = string(b)
			} else {
				rowMap[col] = v
			}
		}
		results = append(results, rowMap)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if results == nil {
		results = []map[string]interface{}{}
	}
	return results, nil
}

// InspectDatabaseTables lists base table names in schema for the active SQL dialect.
func InspectDatabaseTables(ctx context.Context, db *sql.DB, fallbackDialect Dialect, schema string) ([]string, error) {
	if db == nil {
		return nil, errors.New("sqllib: database handle is nil")
	}
	dialect := DetectDialect(db, fallbackDialect)
	var (
		query string
		args  []interface{}
	)
	switch dialect {
	case DialectSQLite:
		query = "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
	case DialectPostgres:
		targetSchema := schema
		if targetSchema == "" {
			targetSchema = "public"
		}
		query = "SELECT table_name FROM information_schema.tables WHERE table_schema = $1 AND table_type = 'BASE TABLE' ORDER BY table_name"
		args = []interface{}{targetSchema}
	case DialectMSSQL:
		if schema != "" {
			query = "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = @p1 AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME"
			args = []interface{}{schema}
		} else {
			query = "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME"
		}
	default:
		if schema != "" {
			query = "SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE' ORDER BY table_name"
			args = []interface{}{schema}
		} else {
			query = "SELECT table_name FROM information_schema.tables WHERE table_type = 'BASE TABLE' ORDER BY table_name"
		}
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// InspectDatabaseColumns lists column metadata for table in schema.
func InspectDatabaseColumns(ctx context.Context, db *sql.DB, fallbackDialect Dialect, table, schema string) ([]ColumnInfo, error) {
	if db == nil {
		return nil, errors.New("sqllib: database handle is nil")
	}
	if err := validateIdent(table); err != nil {
		return nil, err
	}
	dialect := DetectDialect(db, fallbackDialect)
	if dialect == DialectSQLite {
		rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info([%s])", table))
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var cols []ColumnInfo
		for rows.Next() {
			var (
				cid     int
				name    string
				colType string
				notNull int
				dfltVal sql.NullString
				pk      int
			)
			if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltVal, &pk); err != nil {
				return nil, err
			}
			var defPtr *string
			if dfltVal.Valid {
				v := dfltVal.String
				defPtr = &v
			}
			cols = append(cols, ColumnInfo{
				Name:       name,
				DataType:   colType,
				Nullable:   notNull == 0,
				Default:    defPtr,
				PrimaryKey: pk > 0,
			})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(cols) == 0 {
			return nil, fmt.Errorf("sqllib: table %q has no columns or does not exist", table)
		}
		return cols, nil
	}

	var (
		query string
		args  []interface{}
	)
	switch dialect {
	case DialectPostgres:
		targetSchema := schema
		if targetSchema == "" {
			targetSchema = "public"
		}
		query = "SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2 ORDER BY ordinal_position"
		args = []interface{}{targetSchema, table}
	case DialectMSSQL:
		if schema != "" {
			query = "SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE, COLUMN_DEFAULT FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = @p1 AND TABLE_NAME = @p2 ORDER BY ORDINAL_POSITION"
			args = []interface{}{schema, table}
		} else {
			query = "SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE, COLUMN_DEFAULT FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_NAME = @p1 ORDER BY ORDINAL_POSITION"
			args = []interface{}{table}
		}
	default:
		if schema != "" {
			query = "SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position"
			args = []interface{}{schema, table}
		} else {
			query = "SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_name = ? ORDER BY ordinal_position"
			args = []interface{}{table}
		}
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []ColumnInfo
	for rows.Next() {
		var (
			name       string
			dataType   string
			isNullable string
			colDefault sql.NullString
		)
		if err := rows.Scan(&name, &dataType, &isNullable, &colDefault); err != nil {
			return nil, err
		}
		var defPtr *string
		if colDefault.Valid {
			v := colDefault.String
			defPtr = &v
		}
		cols = append(cols, ColumnInfo{
			Name:     name,
			DataType: dataType,
			Nullable: strings.EqualFold(isNullable, "YES"),
			Default:  defPtr,
		})
	}
	return cols, rows.Err()
}
