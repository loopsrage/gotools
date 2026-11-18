package sqllib

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"github.com/loopsrage/gotools/lib/reflectTags"
	"reflect"
	"strings"
	"time"
)

type NewSchemaFunc func() interface{}

func nullTypeFn[T driver.Value]() *sql.Null[T] {
	return &sql.Null[T]{}
}

func fieldMap[T driver.Value]() (nullFn func() *sql.Null[T], callback func(sqlNull interface{}) (driver.Value, error)) {
	nullFn = nullTypeFn[T]
	callback = SqlNullVal[T]
	return nullFn, callback
}

func SqlNullVal[T driver.Value](sqlNull interface{}) (driver.Value, error) {
	dv, ok := sqlNull.(*sql.Null[T])
	if !ok {
		return nil, errors.New("sqlNull is not a sql.Null[T]")
	}
	return dv.V, nil
}

func SqlQuery(ctx context.Context, db *sql.DB, table, tag string, schema func() interface{}, limit int64) error {
	qd, err := buildQuery(schema(), table, tag, limit)
	if err != nil {
		return err
	}

	var args []interface{}
	res, err := db.QueryContext(ctx, qd.query, args...)
	if err != nil {
		return err
	}

	var output []interface{}
	for res.Next() {
		na, sa := qd.CopyScanArgs()
		err = res.Scan(sa...)
		if err != nil {
			return err
		}

		sch := schema()
		for col, val := range na {
			rf, err := qd.reflectTags.Field(col)
			if err != nil {
				return err
			}
			if !rf.CanSet() {
				continue
			}

			vsch := reflect.ValueOf(sch)
			if vsch.Kind() == reflect.Ptr {
				vsch = vsch.Elem()
			}

			field := vsch.FieldByName(col)

			value, err := qd.callbacks[col].(func(interface{}) (driver.Value, error))(val)
			if err != nil {
				return err
			}

			rv := reflect.ValueOf(value)

			if !rv.CanConvert(field.Type()) {
				continue
			}
			field.Set(rv.Convert(rf.Type()))
		}
		output = append(output, sch)
	}
	return nil
}

type queryData struct {
	reflectTags *reflectTags.ReflectTags
	query       string
	scanArgs    []interface{}
	callbacks   map[string]interface{}
	nulltypes   map[string]interface{}
}

func (qd *queryData) CopyScanArgs() (out map[string]interface{}, args []interface{}) {
	out = make(map[string]interface{})
	for k, v := range qd.nulltypes {
		tp, _ := qd.reflectTags.Field(k)
		var arg interface{}
		switch tp.Kind() {
		case reflect.String:
			arg = v.(func() *sql.Null[string])()
		case reflect.Bool:
			arg = v.(func() *sql.Null[bool])()
		case reflect.Int64:
			arg = v.(func() *sql.Null[int64])()
		case reflect.Int16:
			arg = v.(func() *sql.Null[int16])()
		case reflect.Int32:
			arg = v.(func() *sql.Null[int32])()
		case reflect.Float64:
			arg = v.(func() *sql.Null[float64])()
		case reflect.TypeOf(time.Time{}).Kind():
			arg = v.(func() *sql.Null[time.Time])()
		default:
			arg = v.(func() *sql.Null[interface{}])()
		}
		out[k] = arg
		args = append(args, arg)
	}
	return out, args
}

func buildQuery(to interface{}, table, tag string, limit int64) (*queryData, error) {
	const qs = "SELECT TOP %d %s FROM %s"
	const where = "WHERE %s "

	var selectColumns []string
	rt := &reflectTags.ReflectTags{}
	err := rt.FindTags(to, tag)
	if err != nil {
		return nil, err
	}

	var scanArgs []interface{}
	nullTypes := make(map[string]interface{})
	callbacks := make(map[string]interface{})
	err = rt.RangeFieldTypes(func(name string, _type reflect.Type, value reflect.Value) {
		if name == "" {
			return
		}

		var nullType interface{}
		var callback interface{}

		switch _type.Kind() {
		case reflect.String:
			nullType, callback = fieldMap[string]()
			scanArgs = append(scanArgs, nullType.(func() *sql.Null[string])())
		case reflect.Bool:
			nullType, callback = fieldMap[bool]()
			scanArgs = append(scanArgs, nullType.(func() *sql.Null[bool])())
		case reflect.Int64:
			nullType, callback = fieldMap[int64]()
			scanArgs = append(scanArgs, nullType.(func() *sql.Null[int64])())
		case reflect.Int16:
			nullType, callback = fieldMap[int16]()
			scanArgs = append(scanArgs, nullType.(func() *sql.Null[int16])())
		case reflect.Int32:
			nullType, callback = fieldMap[int32]()
			scanArgs = append(scanArgs, nullType.(func() *sql.Null[int32])())
		case reflect.Float64:
			nullType, callback = fieldMap[float64]()
			scanArgs = append(scanArgs, nullType.(func() *sql.Null[float64])())
		case reflect.TypeOf(time.Time{}).Kind():
			nullType, callback = fieldMap[time.Time]()
			scanArgs = append(scanArgs, nullType.(func() *sql.Null[time.Time])())
		default:
			nullType, callback = fieldMap[interface{}]()
			scanArgs = append(scanArgs, nullType.(func() *sql.Null[interface{}])())
		}

		nn := rt.Tag(name, tag)
		if nn == "" {
			nn = _type.Name()
		}

		callbacks[name] = callback
		nullTypes[name] = nullType
		selectColumns = append(selectColumns, name)
	})
	if err != nil {
		return nil, err
	}

	return &queryData{
		query:       fmt.Sprintf(qs, limit, strings.Join(selectColumns, ","), table),
		scanArgs:    scanArgs,
		callbacks:   callbacks,
		nulltypes:   nullTypes,
		reflectTags: rt,
	}, nil
}

func SqlQueryRow(ctx context.Context, db *sql.DB, table, tag string, to interface{}, limit int64) error {
	qd, err := buildQuery(to, table, tag, limit)
	if err != nil {
		return err
	}

	var args []interface{}
	res := db.QueryRowContext(ctx, qd.query, args...)
	if res.Err() != nil {
		return res.Err()
	}

	dat, sa := qd.CopyScanArgs()

	err = res.Scan(sa...)
	if err != nil {
		return err
	}

	for col, val := range dat {
		rf, err := qd.reflectTags.Field(col)
		if err != nil {
			return err
		}
		if !rf.CanSet() {
			continue
		}

		value, err := qd.callbacks[col].(func(interface{}) (driver.Value, error))(val)
		if err != nil {
			return err
		}

		if value == nil {
			continue
		}
		rv := reflect.ValueOf(value)
		if rv.CanConvert(rf.Type()) {
			rf.Set(rv.Convert(rf.Type()))
		}
	}

	return nil
}
