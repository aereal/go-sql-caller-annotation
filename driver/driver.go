package driver

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"runtime"
	"sync"
)

var (
	muInitializedDrivers sync.Mutex
	initializedDrivers   = map[string]bool{}
	driverSuffix         = ":annotator"
)

func injectCaller(query string) string {
	pc, file, line, _ := runtime.Caller(1)
	fn := runtime.FuncForPC(pc)
	return fmt.Sprintf("/* %s (%s:%d) */ %s", fn.Name(), file, line, query)
}

func Adopt(driver, dsn string) (*sql.DB, error) {
	if err := initDriver(driver, dsn); err != nil {
		return nil, err
	}
	return sql.Open(driver+driverSuffix, dsn)
}

// mostly copied by aws-xray-sdk-go (https://github.com/aws/aws-xray-sdk-go/blob/master/xray/sql_context.go)

func initDriver(driver, dsn string) error {
	muInitializedDrivers.Lock()
	defer muInitializedDrivers.Unlock()

	if _, ok := initializedDrivers[driver]; ok {
		return nil
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return err
	}
	sql.Register(driver+driverSuffix, &driverDriver{Driver: db.Driver(), baseName: driver})
	initializedDrivers[driver] = true
	db.Close()

	return nil
}

type driverDriver struct {
	driver.Driver
	baseName string
}

func (d *driverDriver) Open(dsn string) (driver.Conn, error) {
	rawConn, err := d.Driver.Open(dsn)
	if err != nil {
		return nil, err
	}

	return &driverConn{Conn: rawConn}, nil
}

type driverConn struct {
	driver.Conn
}

var _ interface {
	driver.Conn
	driver.ConnPrepareContext
	driver.ConnBeginTx
	driver.Pinger
	driver.Execer
	driver.ExecerContext
	driver.Queryer
	driver.QueryerContext
	driver.SessionResetter
	driver.NamedValueChecker
} = &driverConn{}

func (c *driverConn) CheckNamedValue(nv *driver.NamedValue) (err error) {
	if checker, ok := c.Conn.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(nv)
	}
	nv.Value, err = driver.DefaultParameterConverter.ConvertValue(nv.Value)
	return
}

func (c *driverConn) ResetSession(ctx context.Context) error {
	resetter, ok := c.Conn.(driver.SessionResetter)
	if !ok {
		return driver.ErrSkip
	}
	return resetter.ResetSession(ctx)
}

func (c *driverConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	beginner, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		return nil, driver.ErrSkip
	}
	return beginner.BeginTx(ctx, opts)
}

func (c *driverConn) Ping(ctx context.Context) error {
	pinger, ok := c.Conn.(driver.Pinger)
	if !ok {
		return driver.ErrSkip
	}
	return pinger.Ping(ctx)
}

func (c *driverConn) Prepare(query string) (driver.Stmt, error) {
	return c.Conn.Prepare(injectCaller(query))
}

func (c *driverConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	var (
		stmt driver.Stmt
		err  error
	)
	if connCtx, ok := c.Conn.(driver.ConnPrepareContext); ok {
		stmt, err = connCtx.PrepareContext(ctx, injectCaller(query))
	} else {
		stmt, err = c.Conn.Prepare(query)
		if err == nil {
			select {
			default:
			case <-ctx.Done():
				stmt.Close()
				return nil, ctx.Err()
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return &driverStmt{
		Stmt: stmt,
		conn: c,
	}, nil
}

func (c *driverConn) Exec(query string, args []driver.Value) (driver.Result, error) {
	execer, ok := c.Conn.(driver.Execer)
	if !ok {
		return nil, driver.ErrSkip
	}
	return execer.Exec(injectCaller(query), args)
}

func (c *driverConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	execer, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return execer.ExecContext(ctx, injectCaller(query), args)
}

func (c *driverConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	queryer, ok := c.Conn.(driver.Queryer)
	if !ok {
		return nil, driver.ErrSkip
	}
	return queryer.Query(injectCaller(query), args)
}

func (c *driverConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return queryer.QueryContext(ctx, injectCaller(query), args)
}

type driverStmt struct {
	driver.Stmt
	conn *driverConn
}

var _ interface {
	driver.Stmt
	driver.StmtExecContext
	driver.StmtQueryContext
	driver.ColumnConverter
	driver.NamedValueChecker
} = &driverStmt{}

func (s *driverStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	execer, ok := s.Stmt.(driver.StmtExecContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return execer.ExecContext(ctx, args)
}

func (s *driverStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	queryer, ok := s.Stmt.(driver.StmtQueryContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return queryer.QueryContext(ctx, args)
}

func (s *driverStmt) ColumnConverter(idx int) driver.ValueConverter {
	if conv, ok := s.Stmt.(driver.ColumnConverter); ok {
		return conv.ColumnConverter(idx)
	}
	return driver.DefaultParameterConverter
}

func (s *driverStmt) CheckNamedValue(nv *driver.NamedValue) (err error) {
	if checker, ok := s.Stmt.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(nv)
	}
	if checker, ok := s.conn.Conn.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(nv)
	}
	nv.Value, err = driver.DefaultParameterConverter.ConvertValue(nv.Value)
	return
}
