package sqllib_test

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/loopsrage/gotools/lib/sqllib"
)

func TestSqlNullValString(t *testing.T) {
	result, err := sqllib.SqlNullVal[string](&sql.Null[string]{V: "hello", Valid: true})
	if err != nil {
		t.Fatalf("SqlNullVal() error: %v", err)
	}
	if result != "hello" {
		t.Fatalf("SqlNullVal() = %v, want %v", result, "hello")
	}
}

func TestSqlNullValInt64(t *testing.T) {
	result, err := sqllib.SqlNullVal[int64](&sql.Null[int64]{V: 42, Valid: true})
	if err != nil {
		t.Fatalf("SqlNullVal() error: %v", err)
	}
	if result != int64(42) {
		t.Fatalf("SqlNullVal() = %v, want %v", result, int64(42))
	}
}

func TestSqlNullValBool(t *testing.T) {
	result, err := sqllib.SqlNullVal[bool](&sql.Null[bool]{V: true, Valid: true})
	if err != nil {
		t.Fatalf("SqlNullVal() error: %v", err)
	}
	if result != true {
		t.Fatalf("SqlNullVal() = %v, want %v", result, true)
	}
}

func TestSqlNullValInvalidType(t *testing.T) {
	_, err := sqllib.SqlNullVal[string](&sql.Null[int64]{V: 42, Valid: true})
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
	if err.Error() != "sqlNull is not a sql.Null[T]" {
		t.Fatalf("SqlNullVal() wrong type error = %v, want 'sqlNull is not a sql.Null[T]'", err)
	}
}

func TestSqlNullValInvalid(t *testing.T) {
	result, err := sqllib.SqlNullVal[string](&sql.Null[string]{V: "", Valid: false})
	if err != nil {
		t.Fatalf("SqlNullVal() error: %v", err)
	}
	if result != "" {
		t.Fatalf("SqlNullVal() = %v, want %v", result, "")
	}
}

var _ = driver.Valuer(nil)
