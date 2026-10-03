package duckdb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"strings"
	"syscall"

	duckdbdriver "github.com/duckdb/duckdb-go/v2" // registers the "duckdb" database/sql driver
	"github.com/duckdb/duckdb-go/v2/mapping"
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
	// cache is the instance cache made for this database alone; release destroys it.
	cache *mapping.InstanceCache
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

	db, err := openPrivate(ctx, path, path)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", path, err)
	}

	if err := os.Chmod(path, 0o600); err != nil {
		// unreachable: chmod on the path PingContext just validated fails only for an
		// ownership or permission-flag change this single-user test process cannot
		// construct without a race, and not portably across CI.
		_ = db.release()
		return nil, fmt.Errorf("create %s: %w", path, err)
	}

	return db, nil
}

// readOnlyDSN locks a read session down: no write, no outside access, no SET.
const readOnlyDSN = "?access_mode=READ_ONLY&enable_external_access=false" +
	"&autoload_known_extensions=false&autoinstall_known_extensions=false&lock_configuration=true"

// OpenReadOnly opens an existing DuckDB database file at path read-only and
// locked down: no write reaches path, no other file, database or extension is
// reachable, and the configuration cannot be changed. Every read-only open
// in the process must use this one configuration.
//
// Each open is an instance of its own: it never waits on, and never reads
// through, another open of path, so it sees the file now at path.
func OpenReadOnly(ctx context.Context, path string) (*DB, error) {
	db, err := openPrivate(ctx, path+readOnlyDSN, path)
	if err != nil {
		return nil, fmt.Errorf("open %s read-only: %w", path, err)
	}
	return db, nil
}

// OpenReadWrite opens an existing DuckDB database file at path for reading
// and writing, with none of OpenReadOnly's lockdown. It exists so a test can
// edit a store behind quarry's back; production code opens a store only
// through Create and OpenReadOnly. Like them, it is an instance of its own.
func OpenReadWrite(ctx context.Context, path string) (*DB, error) {
	db, err := openPrivate(ctx, path, path)
	if err != nil {
		return nil, fmt.Errorf("open %s read-write: %w", path, err)
	}
	return db, nil
}

// openPrivate opens dsn on a new instance cache that the returned DB owns.
func openPrivate(ctx context.Context, dsn, path string) (*DB, error) {
	cache := mapping.CreateInstanceCache()
	return openOwned(ctx, &cache, dsn, path)
}

// openOwned opens dsn on cache and pings it, taking ownership of cache: the
// returned DB's Close destroys it, and so does any failure here. The driver
// opens the file inside openDB itself, so a missing path or permission fault
// surfaces there, not at the ping.
func openOwned(ctx context.Context, cache *mapping.InstanceCache, dsn, path string) (*DB, error) {
	conn, err := openDB(dsn, cache)
	if err != nil {
		mapping.DestroyInstanceCache(cache)
		return nil, err
	}
	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn, path: path, cache: cache}
	if err := conn.PingContext(ctx); err != nil {
		_ = db.release()
		return nil, err //nolint:wrapcheck // callers add the path and operation
	}
	return db, nil
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
// soon as row returns one, or ctx's error once ctx is done.
func (d *DB) QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error {
	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query rows %q: %w", query, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		// database/sql closes rows asynchronously on cancel, so rows.Err() alone can miss it.
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("query rows %q: %w", query, err)
		}
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
	if err := d.release(); err != nil {
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
	return d.release()
}

// release closes the connection, then destroys the instance cache made for it. Calling it again is a no-op.
func (d *DB) release() error {
	err := d.conn.Close()
	mapping.DestroyInstanceCache(d.cache)
	return err //nolint:wrapcheck // callers add context
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

// IsReadOnlyViolation reports whether err is the driver's refusal of a
// statement that would write through a read-only connection.
func IsReadOnlyViolation(err error) bool {
	derr, ok := errors.AsType[*duckdbdriver.Error](err)
	return ok && derr.Type == duckdbdriver.ErrorTypeInvalidInput && strings.Contains(derr.Msg, "read-only mode")
}

// IsAccessDisabled reports whether err is the driver's refusal of a file,
// database or extension that OpenReadOnly's configuration turned off. It is
// a configuration refusal, not an OS permission fault (see IsPermission).
func IsAccessDisabled(err error) bool {
	derr, ok := errors.AsType[*duckdbdriver.Error](err)
	return ok && derr.Type == duckdbdriver.ErrorTypePermission
}

// IsEmptyQuery reports whether err is the driver's refusal of a query with no
// statement in it, such as ";" or a lone comment.
func IsEmptyQuery(err error) bool {
	// Whole text: the driver's sentinel is unexported and error('empty query') gains a prefix.
	return err.Error() == "empty query"
}

// IsNotDatabase reports whether err is the driver's refusal to open a file
// that is not a DuckDB database, an empty file included.
func IsNotDatabase(err error) bool {
	return driverIOMessageContains(err, "not a valid DuckDB database file")
}

// IsLocked reports whether err is the driver's refusal to open a file
// another process holds the lock on.
func IsLocked(err error) bool {
	return driverIOMessageContains(err, "Could not set lock on file")
}

// errorTypePrefix matches the error type DuckDB leads its messages with, e.g. "IO Error: ".
var errorTypePrefix = regexp.MustCompile(`^[A-Za-z ]+ Error: `)

// ErrorLine returns the first line of the driver's message for err, or of
// err.Error() when no driver error is in its tree, without DuckDB's leading
// error type.
func ErrorLine(err error) string {
	msg := err.Error()
	if derr, ok := errors.AsType[*duckdbdriver.Error](err); ok {
		msg = derr.Msg
	}
	line, _, _ := strings.Cut(msg, "\n")
	return errorTypePrefix.ReplaceAllString(line, "")
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

// driverIOMessageContains reports whether err is a *duckdbdriver.Error of ErrorTypeIO whose message contains text.
func driverIOMessageContains(err error, text string) bool {
	derr, ok := errors.AsType[*duckdbdriver.Error](err)
	return ok && derr.Type == duckdbdriver.ErrorTypeIO && strings.Contains(derr.Msg, text)
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
