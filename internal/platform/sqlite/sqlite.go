package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	"github.com/koblas/quarry/internal/platform/sqlschema"
	sqlite3 "github.com/mattn/go-sqlite3"
)

// DB is a single SQLite connection opened by this package.
type DB struct {
	conn *sql.DB
}

// OpenReadOnly opens path as a strictly read-only SQLite connection: no
// write, including no journal or WAL change, ever reaches path through this
// connection. It probes the connection with a read against sqlite_master so
// a caller learns immediately whether path is a readable SQLite database.
func OpenReadOnly(ctx context.Context, path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", "file:"+escapePath(path)+"?mode=ro")
	if err != nil {
		// unreachable: sql.Open only validates the driver name, which the sqlite3 import always registers.
		return nil, fmt.Errorf("open %s read-only: %w", path, err)
	}
	conn.SetMaxOpenConns(1)

	var count int
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master").Scan(&count); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("probe %s: %w", path, err)
	}
	return &DB{conn: conn}, nil
}

// escapePath percent-encodes path for use as a SQLite URI filename, so
// characters query parsing would otherwise treat specially (space, ?, #)
// survive as part of the path.
func escapePath(path string) string {
	return (&url.URL{Path: path}).EscapedPath()
}

// Close closes the underlying connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

// Exec runs query against the connection.
func (d *DB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.conn.ExecContext(ctx, query, args...)
}

// OpenMemory opens a private, in-process SQLite database. Because
// database/sql pools connections and ":memory:" is per-connection, the pool
// is pinned to one connection so every call sees the same database.
func OpenMemory(ctx context.Context) (*DB, error) {
	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		// unreachable: sql.Open only validates the driver name, which the sqlite3 import always registers.
		return nil, fmt.Errorf("open in-memory database: %w", err)
	}
	conn.SetMaxOpenConns(1)

	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open in-memory database: %w", err)
	}
	return &DB{conn: conn}, nil
}

// QueryInt runs query, which must select exactly one integer column and row,
// and returns that value.
func (d *DB) QueryInt(ctx context.Context, query string, args ...any) (int, error) {
	var n int
	if err := d.conn.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("query int %q: %w", query, err)
	}
	return n, nil
}

// IntegrityCheck runs PRAGMA integrity_check and returns an error naming
// the first reported problem when the result is not "ok".
func (d *DB) IntegrityCheck(ctx context.Context) error {
	var result string
	if err := d.conn.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("integrity_check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check: %s", result)
	}
	return nil
}

// Schema reads every table's column names via pragma_table_info, in the
// exact spelling SQLite itself reports.
func (d *DB) Schema(ctx context.Context) (sqlschema.Schema, error) {
	rows, err := d.conn.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table'")
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			// unreachable: sqlite_master.name is a non-null text column; Scan fails only if the driver is broken.
			return nil, fmt.Errorf("list tables: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		// unreachable: the same connection just succeeded above; a cursor error appearing only now is not constructible.
		return nil, fmt.Errorf("list tables: %w", err)
	}

	schema := make(sqlschema.Schema, len(tables))
	for _, table := range tables {
		cols, err := d.tableColumns(ctx, table)
		if err != nil {
			// unreachable: tableColumns' own error paths are unreachable on this same connection; see there.
			return nil, err
		}
		schema[table] = cols
	}
	return schema, nil
}

func (d *DB) tableColumns(ctx context.Context, table string) ([]string, error) {
	rows, err := d.conn.QueryContext(ctx, "SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		// unreachable: the same connection just listed tables above; it cannot break only for this second query.
		return nil, fmt.Errorf("columns of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var cols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			// unreachable: pragma_table_info's name column is a non-null text column; Scan fails only if the driver is broken.
			return nil, fmt.Errorf("columns of %s: %w", table, err)
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

// Backup copies src's "main" database into destPath, which must already
// exist, using SQLite's online backup API so pages still in src's -wal file
// are included. It then sets destPath's journal_mode to DELETE, since the
// backup API copies the WAL flag from the source header and would otherwise
// leave destPath in WAL mode with its own -wal/-shm files.
func Backup(ctx context.Context, src *DB, destPath string) error {
	destDB, err := sql.Open("sqlite3", "file:"+escapePath(destPath))
	if err != nil {
		// unreachable: sql.Open only validates the driver name, which the sqlite3 import always registers.
		return fmt.Errorf("backup to %s: open destination: %w", destPath, err)
	}
	defer func() { _ = destDB.Close() }()
	destDB.SetMaxOpenConns(1)

	srcConn, err := src.conn.Conn(ctx)
	if err != nil {
		return fmt.Errorf("backup to %s: %w", destPath, err)
	}
	defer func() { _ = srcConn.Close() }()

	destConn, err := destDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("backup to %s: %w", destPath, err)
	}
	defer func() { _ = destConn.Close() }()

	err = destConn.Raw(func(destDriverConn any) error {
		return srcConn.Raw(func(srcDriverConn any) error {
			return runBackup(destDriverConn, srcDriverConn)
		})
	})
	if err != nil {
		return fmt.Errorf("backup to %s: %w", destPath, err)
	}

	// Run on destConn itself: destDB's pool holds only this one connection,
	// so a query through destDB here would block waiting for destConn to be
	// released.
	if _, err := destConn.ExecContext(ctx, "PRAGMA journal_mode = DELETE"); err != nil {
		// unreachable: destConn just completed the backup write above; a pragma on the same connection failing only now needs a fault between the two statements that this package cannot construct.
		return fmt.Errorf("backup to %s: set journal_mode delete: %w", destPath, err)
	}
	return nil
}

// runBackup drives one SQLite online-backup pass from srcDriverConn's "main"
// database into destDriverConn's "main" database, stepping until done.
func runBackup(destDriverConn, srcDriverConn any) error {
	dc, ok := destDriverConn.(*sqlite3.SQLiteConn)
	if !ok {
		// unreachable: this package only ever opens connections through the sqlite3 driver.
		return fmt.Errorf("destination connection is not a sqlite3 connection")
	}
	sc, ok := srcDriverConn.(*sqlite3.SQLiteConn)
	if !ok {
		// unreachable: this package only ever opens connections through the sqlite3 driver.
		return fmt.Errorf("source connection is not a sqlite3 connection")
	}

	bk, err := dc.Backup("main", sc, "main")
	if err != nil {
		// unreachable: sqlite3_backup_init fails only for a same-connection or concurrent-backup misuse this package's callers never construct; Step's own fault path below exercises the failure contract.
		return err
	}
	defer func() { _ = bk.Close() }()

	for {
		done, err := bk.Step(-1)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}
