// Package sql provides a javascript module for performing SQL actions against relational databases.
package sql

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modules"
)

// ImportPath contains module's JavaScript import path.
const ImportPath = "k6/x/sql"

// MySQL defaults from the interwebs
const defaultConnMaxLifetime = 14400 * time.Second // 4 hours
const defaultConnMaxIdleTime = 28800 * time.Second // 8 hours
const defaultMaxConnections = 151

// New creates a new instance of the extension's JavaScript module.
func New() modules.Module {
	return new(rootModule)
}

// rootModule is the global module object type. It is instantiated once per test
// run and will be used to create `k6/x/sql` module instances for each VU.
type rootModule struct{}

// NewModuleInstance implements the modules.Module interface to return
// a new instance for each VU.
func (*rootModule) NewModuleInstance(vu modules.VU) modules.Instance {
	instance := &module{vu: vu}

	instance.exports.Default = instance
	instance.exports.Named = map[string]interface{}{
		"open": instance.Open,
	}

	return instance
}

// module represents an instance of the JavaScript module for every VU.
type module struct {
	vu      modules.VU
	exports modules.Exports
}

// SQL Connection options
type connectOptions struct {
	ConnMaxLifetime *time.Duration `json:"ConnMaxLifetime,omitempty"`
	ConnMaxIdleTime *time.Duration `json:"ConnMaxIdleTime,omitempty"`
	MaxOpenConns    *int           `json:"MaxOpenConns,omitempty"`
	MaxIdleConns    *int           `json:"MaxIdleConns,omitempty"`
}

// Exports is representation of ESM exports of a module.
func (mod *module) Exports() modules.Exports {
	return mod.exports
}

// KeyValue is a simple key-value pair.
type KeyValue map[string]interface{}

// options provides a constructor interface for the Connection Options for the Javascript runtime
// ```js
// const options = new sql.Options(...);
// ```
func (mod *module) ConnOptions(c sobek.ConstructorCall) *sobek.Object {
	rt := mod.vu.Runtime()
	options := &connectOptions{}
	// 	ConnMaxLifetime: &connMaxLifeTime,
	// 	// ConnMaxIdleTime: defaultConnMaxIdleTime,
	// 	// MaxOpenConns:    defaultMaxConnections,
	// 	// MaxIdleConns:    defaultMaxConnections,
	// }

	if len(c.Arguments) > 1 || c.Argument(0).ExportType().Kind() == reflect.String {
		if err := mod.parsePositionalOptions(c, options); err != nil {
			common.Throw(rt, fmt.Errorf("could not parse connectOptions positional parameter: %w", err))
		}
	} else {
		if err := mod.parseOptionsObject(c.Argument(0).ToObject(rt), options); err != nil {
			common.Throw(rt, fmt.Errorf("could not parse connectOptions object: %w", err))
		}
	}

	return rt.ToValue(options).ToObject(rt)
}

// open establishes a connection to the specified database type using
// the provided connection string.
func (mod *module) Open(driverID sobek.Value, connectionString string) (*Database, error) {
	driverSym, ok := driverID.(*sobek.Symbol)
	if !ok {
		return nil, fmt.Errorf("%w: invalid driver parameter type", errUnsupportedDatabase)
	}

	registered, database := lookupDriver(driverSym)
	if !registered {
		return nil, fmt.Errorf("%w: %s", errUnsupportedDatabase, database)
	}

	db, err := sql.Open(database, connectionString)
	if err != nil {
		return nil, err
	}

	return &Database{db: db}, nil
}

func (mod *module) OpenWithOptions(driverID sobek.Value, connectionString string, c sobek.Value) *sobek.Object {
	rt := mod.vu.Runtime()
	connOptions, err := parseOptions(rt, c)
	if err != nil {
		common.Throw(rt, fmt.Errorf("Open expects connection options as it's argument %w", err))
	}

	database, err := mod.Open(driverID, connectionString)
	if err != nil {
		common.Throw(rt, fmt.Errorf("failed to open connection to %s", connectionString))
	}

	if connOptions.ConnMaxIdleTime != nil {
		database.db.SetConnMaxIdleTime(*connOptions.ConnMaxIdleTime)
	}

	if connOptions.ConnMaxLifetime != nil {
		database.db.SetConnMaxLifetime(*connOptions.ConnMaxLifetime)
	}

	if connOptions.MaxIdleConns != nil {
		database.db.SetMaxIdleConns(*connOptions.MaxIdleConns)
	}

	if connOptions.MaxOpenConns != nil {
		database.db.SetMaxOpenConns(*connOptions.MaxOpenConns)
	}

	return rt.ToValue(database).ToObject(rt)
}

func (mod *module) parsePositionalOptions(c sobek.ConstructorCall, connectOptions *connectOptions) error {
	if len(c.Arguments) > 0 {
		connMaxLifeTime, err := time.ParseDuration(c.Argument(0).String())
		if err != nil {
			return fmt.Errorf("connMaxLifeTime should be duration as string: %w", err)
		}
		connectOptions.ConnMaxLifetime = &connMaxLifeTime
	}

	if len(c.Arguments) > 1 {
		connMaxIdleTime, err := time.ParseDuration(c.Argument(1).String())
		if err != nil {
			return fmt.Errorf("ConnMaxIdleTime should be duration as string: %w", err)
		}
		connectOptions.ConnMaxLifetime = &connMaxIdleTime
	}

	if len(c.Arguments) > 2 {
		maxOpenConns := int(c.Argument(2).ToInteger())
		connectOptions.MaxOpenConns = &maxOpenConns
	}

	if len(c.Arguments) > 3 {
		maxIdleConns := int(c.Argument(3).ToInteger())
		connectOptions.MaxIdleConns = &maxIdleConns
	}

	return nil
}

func (mod *module) parseOptionsObject(c *sobek.Object, connectOptions *connectOptions) error {
	if value := c.Get("ConnMaxLifetime"); !isUndefined(value) {
		connMaxLifeTime, err := time.ParseDuration(value.String())
		if err != nil {
			return fmt.Errorf("connMaxLifeTime should be duration as string: %w", err)
		}
		connectOptions.ConnMaxLifetime = &connMaxLifeTime
	}

	if value := c.Get("ConnMaxIdleTime"); !isUndefined(value) {
		connMaxIdleTime, err := time.ParseDuration(value.String())
		if err != nil {
			return fmt.Errorf("connMaxLifeTime should be duration as string: %w", err)
		}
		connectOptions.ConnMaxIdleTime = &connMaxIdleTime
	}

	if value := c.Get("MaxOpenConns"); !isUndefined(value) {
		maxOpenConns := int(value.ToInteger())
		connectOptions.MaxOpenConns = &maxOpenConns
	}

	if value := c.Get("MaxIdleConns"); !isUndefined(value) {
		maxIdleConns := int(value.ToInteger())
		connectOptions.MaxIdleConns = &maxIdleConns
	}

	return nil
}

// Database is a database handle representing a pool of zero or more underlying connections.
type Database struct {
	db *sql.DB
}

// Query executes a query that returns rows, typically a SELECT.
func (dbase *Database) Query(query string, args ...interface{}) ([]KeyValue, error) {
	rows, err := dbase.db.Query(query, args...)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = rows.Close()
	}()
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	result := make([]KeyValue, 0)

	for rows.Next() {
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		err = rows.Scan(valuePtrs...)
		if err != nil {
			return nil, err
		}

		data := make(KeyValue, len(cols))
		for i, colName := range cols {
			data[colName] = *valuePtrs[i].(*interface{}) //nolint:forcetypeassert
		}
		result = append(result, data)
	}

	return result, nil
}

// Exec a query without returning any rows.
func (dbase *Database) Exec(query string, args ...interface{}) (sql.Result, error) {
	return dbase.db.Exec(query, args...)
}

// Close the database and prevents new queries from starting.
func (dbase *Database) Close() error {
	return dbase.db.Close()
}

var errUnsupportedDatabase = errors.New("unsupported database")

func isUndefined(v sobek.Value) bool {
	return v == nil || sobek.IsUndefined(v) || sobek.IsNull(v)
}

func parseOptions(rt *sobek.Runtime, inOpts sobek.Value) (*connectOptions, error) {
	var connOpts connectOptions

	if isUndefined(inOpts) {
		return &connOpts, nil
	}

	return inOpts.ToObject(rt).Export().(*connectOptions), nil
	// return &connOpts, nil
}
