package duckdb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"syscall"

	duckdbdriver "github.com/duckdb/duckdb-go/v2" // registers the "duckdb" database/sql driver
)

// ErrExists is returned by Create when path already exists.
var ErrExists = errors.New("duckdb: file already exists")

// ErrWALRemains is returned by CheckpointClose when a .wal file is still present after CHECKPOINT.
var ErrWALRemains = errors.New("duckdb: wal file remains after checkpoint")

// errNotDuckDBConn reports a pooled connection from some other driver.
var errNotDuckDBConn = errors.New("connection is not a duckdb driver connection")

// DB is a single DuckDB connection opened by this package, pinned to one
// pool connection so callers, the Appender and CheckpointClose all observe
// the same session.
type DB struct {
	conn *sql.DB
	path string
}

// Create makes a new DuckDB database file at path, refusing an existing one
// (ErrExists) so a caller never silently overwrites a store. The file is
// created owner-only (0600). Unlike atomicfile.Create this is a stat then
// open, not an O_EXCL claim: DuckDB refuses to open a pre-created empty file.
func Create(ctx context.Context, path string) (*DB, error) {
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("create %s: %w", path, ErrExists)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("create %s: %w", path, err)
	}

	conn, err := sql.Open("duckdb", path)
	if err != nil {
		// The duckdb driver opens (and for a new path, creates) the file
		// inside sql.Open itself, so a permission fault on path or its
		// parent directory surfaces here, not at PingContext below.
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
	conn.SetMaxOpenConns(1)

	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("create %s: %w", path, err)
	}

	if err := os.Chmod(path, 0o600); err != nil {
		// unreachable: chmod on the path PingContext just validated fails only for an
		// ownership or permission-flag change this single-user test process cannot
		// construct without a race, and not portably across CI.
		_ = conn.Close()
		return nil, fmt.Errorf("create %s: %w", path, err)
	}

	return &DB{conn: conn, path: path}, nil
}

// OpenReadOnly opens an existing DuckDB database file at path for
// read-only access: no write reaches path through this connection.
func OpenReadOnly(ctx context.Context, path string) (*DB, error) {
	conn, err := sql.Open("duckdb", path+"?access_mode=READ_ONLY")
	if err != nil {
		// The duckdb driver opens the file inside sql.Open itself, so a
		// missing path or permission fault surfaces here, not at
		// PingContext below.
		return nil, fmt.Errorf("open %s read-only: %w", path, err)
	}
	conn.SetMaxOpenConns(1)

	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open %s read-only: %w", path, err)
	}
	return &DB{conn: conn, path: path}, nil
}

// Exec runs query against the connection.
func (d *DB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.conn.ExecContext(ctx, query, args...) //nolint:wrapcheck // thin adapter over *sql.DB; callers add context
}

// AppendRows bulk-inserts rows into table via the driver's Appender, one
// value per column in table's column order (use Decimal for a DECIMAL
// column). ctx is checked before each row, since the Appender itself does
// not observe it; a buffered-row constraint violation surfaces at Close.
func (d *DB) AppendRows(ctx context.Context, table string, rows [][]any) error {
	conn, err := d.conn.Conn(ctx)
	if err != nil {
		return fmt.Errorf("append rows to %s: %w", table, err)
	}
	defer func() { _ = conn.Close() }()

	return conn.Raw(func(driverConn any) error { //nolint:wrapcheck // Raw returns the callback's error, already wrapped below
		dc, ok := driverConn.(driver.Conn)
		if !ok {
			// unreachable: this package only ever opens connections through the duckdb driver.
			return fmt.Errorf("append rows to %s: %w", table, errNotDuckDBConn)
		}
		app, err := duckdbdriver.NewAppenderFromConn(dc, "", table)
		if err != nil {
			return fmt.Errorf("append rows to %s: %w", table, err)
		}

		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				_ = app.Close()
				return fmt.Errorf("append rows to %s: %w", table, err)
			}
			values := make([]driver.Value, len(row))
			for i, v := range row {
				values[i] = v
			}
			if err := app.AppendRow(values...); err != nil {
				_ = app.Close()
				return fmt.Errorf("append rows to %s: %w", table, err)
			}
		}

		if err := app.Close(); err != nil {
			return fmt.Errorf("append rows to %s: %w", table, err)
		}
		return nil
	})
}

// QueryRows runs query and calls row once per result row, passing a scan
// func bound to that row's columns. It stops and returns row's error as
// soon as row returns one.
func (d *DB) QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error {
	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query rows %q: %w", query, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		if err := row(rows.Scan); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("query rows %q: %w", query, err)
	}
	return nil
}

// CheckpointClose runs CHECKPOINT so every WAL record moves into the main
// file, then closes the connection. It returns ErrWALRemains if a .wal file
// is still present beside the database afterward — the condition a caller
// must never rename over.
func (d *DB) CheckpointClose(ctx context.Context) error {
	if _, err := d.conn.ExecContext(ctx, "CHECKPOINT"); err != nil {
		return fmt.Errorf("checkpoint %s: %w", d.path, err)
	}
	if err := d.conn.Close(); err != nil {
		// unreachable: database/sql.DB.Close returns only the driver's own Close error
		// for the one pooled connection, which CHECKPOINT just used successfully, and
		// the duckdb driver's Close of a live connection does not fail.
		return fmt.Errorf("checkpoint %s: %w", d.path, err)
	}
	if err := checkNoWAL(d.path); err != nil {
		// unreachable: CHECKPOINT removes the .wal file itself before Close can
		// observe one (see checkpoint_internal_test.go for checkNoWAL's own coverage).
		return fmt.Errorf("checkpoint %s: %w", d.path, err)
	}
	return nil
}

// checkNoWAL reports ErrWALRemains if path+".wal" exists.
func checkNoWAL(path string) error {
	if _, err := os.Stat(path + ".wal"); err == nil {
		return ErrWALRemains
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s.wal: %w", path, err)
	}
	return nil
}

// Close closes the underlying connection without running CHECKPOINT.
func (d *DB) Close() error {
	return d.conn.Close() //nolint:wrapcheck // thin adapter over *sql.DB; callers add context
}

// Decimal returns a driver value representing unscaled x 10^-scale as
// DECIMAL(width, scale), for use as an AppendRows row value. It refuses an
// unscaled magnitude of 10^width or more itself, since the driver appends
// such a value without complaint.
func Decimal(unscaled int64, width, scale uint8) (any, error) {
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(width)), nil)
	value := big.NewInt(unscaled)
	if new(big.Int).Abs(value).Cmp(limit) >= 0 {
		return nil, decimalRangeError{unscaled: unscaled, width: width, scale: scale}
	}
	return duckdbdriver.Decimal{Width: width, Scale: scale, Value: value}, nil
}

// IsDiskFull reports whether err is a disk-full fault: an OS ENOSPC/EDQUOT,
// or the driver's own IO error carrying DuckDB's disk-full message.
func IsDiskFull(err error) bool {
	return errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT) ||
		isDriverIOError(err, syscall.ENOSPC) || isDriverIOError(err, syscall.EDQUOT)
}

// IsPermission reports whether err is a permission fault: an OS
// EACCES/EPERM, or the driver's own IO error carrying DuckDB's permission
// message.
func IsPermission(err error) bool {
	return errors.Is(err, os.ErrPermission) ||
		isDriverIOError(err, syscall.EACCES) || isDriverIOError(err, syscall.EPERM)
}

// isDriverIOError reports whether err is a *duckdbdriver.Error of ErrorTypeIO
// whose message contains errno's OS-supplied text (DuckDB has no errno of its own).
func isDriverIOError(err error, errno syscall.Errno) bool {
	var derr *duckdbdriver.Error
	if !errors.As(err, &derr) || derr.Type != duckdbdriver.ErrorTypeIO {
		return false
	}
	return strings.Contains(strings.ToLower(derr.Msg), strings.ToLower(errno.Error()))
}

// decimalRangeError is Decimal's refusal of an unscaled value too wide for
// DECIMAL(width, scale).
type decimalRangeError struct {
	unscaled     int64
	width, scale uint8
}

func (e decimalRangeError) Error() string {
	return fmt.Sprintf("decimal %d exceeds DECIMAL(%d,%d)", e.unscaled, e.width, e.scale)
}
