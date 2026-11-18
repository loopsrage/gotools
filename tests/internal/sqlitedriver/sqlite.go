package sqlitedriver

/*
#cgo LDFLAGS: -lsqlite3
#include <sqlite3.h>
#include <stdlib.h>

static int bind_text_transient(sqlite3_stmt* stmt, int idx, const char* val, int n) {
    return sqlite3_bind_text(stmt, idx, val, n, SQLITE_TRANSIENT);
}

static int bind_blob_transient(sqlite3_stmt* stmt, int idx, const void* val, int n) {
    return sqlite3_bind_blob(stmt, idx, val, n, SQLITE_TRANSIENT);
}
*/
import "C"

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sync"
	"unsafe"
)

const DriverName = "gotools_sqlite3"

var registerOnce sync.Once

func Register() {
	registerOnce.Do(func() {
		sql.Register(DriverName, &sqliteDriver{})
	})
}

// OpenMemory opens a single-connection in-memory SQLite3 *sql.DB backed by native libsqlite3.
func OpenMemory() (*sql.DB, error) {
	Register()
	db, err := sql.Open(DriverName, ":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}

type sqliteDriver struct{}

func (d *sqliteDriver) Open(name string) (driver.Conn, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	var db *C.sqlite3
	flags := C.int(C.SQLITE_OPEN_READWRITE | C.SQLITE_OPEN_CREATE | C.SQLITE_OPEN_FULLMUTEX)
	rc := C.sqlite3_open_v2(cName, &db, flags, nil)
	if rc != C.SQLITE_OK {
		msg := "sqlite3_open_v2 failed"
		if db != nil {
			msg = C.GoString(C.sqlite3_errmsg(db))
			C.sqlite3_close_v2(db)
		}
		return nil, errors.New(msg)
	}
	return &sqliteConn{db: db}, nil
}

type sqliteConn struct {
	db *C.sqlite3
}

func (c *sqliteConn) Prepare(query string) (driver.Stmt, error) {
	cQuery := C.CString(query)
	defer C.free(unsafe.Pointer(cQuery))

	var stmt *C.sqlite3_stmt
	rc := C.sqlite3_prepare_v2(c.db, cQuery, -1, &stmt, nil)
	if rc != C.SQLITE_OK {
		return nil, errors.New(C.GoString(C.sqlite3_errmsg(c.db)))
	}
	return &sqliteStmt{conn: c, stmt: stmt}, nil
}

func (c *sqliteConn) Close() error {
	if c.db != nil {
		C.sqlite3_close_v2(c.db)
		c.db = nil
	}
	return nil
}

func (c *sqliteConn) Begin() (driver.Tx, error) {
	if _, err := c.ExecContext(context.Background(), "BEGIN", nil); err != nil {
		return nil, err
	}
	return &sqliteTx{conn: c}, nil
}

func (c *sqliteConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	st, err := c.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	return st.(*sqliteStmt).execNamed(args)
}

func (c *sqliteConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	st, err := c.Prepare(query)
	if err != nil {
		return nil, err
	}
	rows, err := st.(*sqliteStmt).queryNamed(args)
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	rows.(*sqliteRows).ownsStmt = true
	return rows, nil
}

type sqliteTx struct {
	conn *sqliteConn
}

func (t *sqliteTx) Commit() error {
	_, err := t.conn.ExecContext(context.Background(), "COMMIT", nil)
	return err
}

func (t *sqliteTx) Rollback() error {
	_, err := t.conn.ExecContext(context.Background(), "ROLLBACK", nil)
	return err
}

type sqliteStmt struct {
	conn *sqliteConn
	stmt *C.sqlite3_stmt
}

func (s *sqliteStmt) Close() error {
	if s.stmt != nil {
		C.sqlite3_finalize(s.stmt)
		s.stmt = nil
	}
	return nil
}

func (s *sqliteStmt) NumInput() int {
	return int(C.sqlite3_bind_parameter_count(s.stmt))
}

func (s *sqliteStmt) bindArgs(args []driver.NamedValue) error {
	C.sqlite3_reset(s.stmt)
	C.sqlite3_clear_bindings(s.stmt)
	for _, nv := range args {
		idx := C.int(nv.Ordinal)
		var rc C.int
		switch v := nv.Value.(type) {
		case nil:
			rc = C.sqlite3_bind_null(s.stmt, idx)
		case int64:
			rc = C.sqlite3_bind_int64(s.stmt, idx, C.sqlite3_int64(v))
		case float64:
			rc = C.sqlite3_bind_double(s.stmt, idx, C.double(v))
		case bool:
			var iv C.int
			if v {
				iv = 1
			}
			rc = C.sqlite3_bind_int(s.stmt, idx, iv)
		case string:
			cs := C.CString(v)
			rc = C.bind_text_transient(s.stmt, idx, cs, C.int(len(v)))
			C.free(unsafe.Pointer(cs))
		case []byte:
			if len(v) == 0 {
				rc = C.bind_blob_transient(s.stmt, idx, nil, 0)
			} else {
				rc = C.bind_blob_transient(s.stmt, idx, unsafe.Pointer(&v[0]), C.int(len(v)))
			}
		default:
			str := fmt.Sprintf("%v", v)
			cs := C.CString(str)
			rc = C.bind_text_transient(s.stmt, idx, cs, C.int(len(str)))
			C.free(unsafe.Pointer(cs))
		}
		if rc != C.SQLITE_OK {
			return errors.New(C.GoString(C.sqlite3_errmsg(s.conn.db)))
		}
	}
	return nil
}

func (s *sqliteStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, a := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: a}
	}
	return s.execNamed(named)
}

func (s *sqliteStmt) execNamed(args []driver.NamedValue) (driver.Result, error) {
	if err := s.bindArgs(args); err != nil {
		return nil, err
	}
	rc := C.sqlite3_step(s.stmt)
	if rc != C.SQLITE_DONE && rc != C.SQLITE_ROW {
		return nil, errors.New(C.GoString(C.sqlite3_errmsg(s.conn.db)))
	}
	lastID := int64(C.sqlite3_last_insert_rowid(s.conn.db))
	changes := int64(C.sqlite3_changes(s.conn.db))
	return sqliteResult{lastID: lastID, rowsAffected: changes}, nil
}

func (s *sqliteStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, a := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: a}
	}
	return s.queryNamed(named)
}

func (s *sqliteStmt) queryNamed(args []driver.NamedValue) (driver.Rows, error) {
	if err := s.bindArgs(args); err != nil {
		return nil, err
	}
	colCount := int(C.sqlite3_column_count(s.stmt))
	cols := make([]string, colCount)
	for i := 0; i < colCount; i++ {
		cols[i] = C.GoString(C.sqlite3_column_name(s.stmt, C.int(i)))
	}
	return &sqliteRows{stmt: s, cols: cols}, nil
}

type sqliteResult struct {
	lastID       int64
	rowsAffected int64
}

func (r sqliteResult) LastInsertId() (int64, error) { return r.lastID, nil }
func (r sqliteResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

type sqliteRows struct {
	stmt     *sqliteStmt
	cols     []string
	ownsStmt bool
}

func (r *sqliteRows) Columns() []string { return r.cols }

func (r *sqliteRows) Close() error {
	if r.ownsStmt && r.stmt != nil {
		return r.stmt.Close()
	}
	if r.stmt != nil && r.stmt.stmt != nil {
		C.sqlite3_reset(r.stmt.stmt)
	}
	return nil
}

func (r *sqliteRows) Next(dest []driver.Value) error {
	rc := C.sqlite3_step(r.stmt.stmt)
	if rc == C.SQLITE_DONE {
		return io.EOF
	}
	if rc != C.SQLITE_ROW {
		return errors.New(C.GoString(C.sqlite3_errmsg(r.stmt.conn.db)))
	}
	for i := range dest {
		colIdx := C.int(i)
		colType := C.sqlite3_column_type(r.stmt.stmt, colIdx)
		switch colType {
		case C.SQLITE_INTEGER:
			dest[i] = int64(C.sqlite3_column_int64(r.stmt.stmt, colIdx))
		case C.SQLITE_FLOAT:
			dest[i] = float64(C.sqlite3_column_double(r.stmt.stmt, colIdx))
		case C.SQLITE_TEXT:
			ptr := C.sqlite3_column_text(r.stmt.stmt, colIdx)
			n := C.sqlite3_column_bytes(r.stmt.stmt, colIdx)
			dest[i] = C.GoStringN((*C.char)(unsafe.Pointer(ptr)), n)
		case C.SQLITE_BLOB:
			ptr := C.sqlite3_column_blob(r.stmt.stmt, colIdx)
			n := C.sqlite3_column_bytes(r.stmt.stmt, colIdx)
			dest[i] = C.GoBytes(ptr, n)
		default:
			dest[i] = nil
		}
	}
	return nil
}
