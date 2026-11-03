package store

import (
	"context"
	"database/sql/driver"
	"errors"
)

// checkpointFault forwards real database operations except the deliberately
// failed final checkpoint, avoiding a fake SQL engine in retention tests.
type checkpointFault struct{ base driver.Connector }

func (f checkpointFault) Driver() driver.Driver { return f.base.Driver() }
func (f checkpointFault) Connect(ctx context.Context) (driver.Conn, error) {
	c, err := f.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return checkpointFaultConn{Conn: c}, nil
}

type checkpointFaultConn struct{ driver.Conn }

func (c checkpointFaultConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	if q == "CHECKPOINT" {
		return nil, errors.New("injected checkpoint I/O failure")
	}
	if ex, ok := c.Conn.(driver.ExecerContext); ok {
		return ex.ExecContext(ctx, q, args)
	}
	return nil, driver.ErrSkip
}
