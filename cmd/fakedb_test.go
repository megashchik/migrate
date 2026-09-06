package cmd

// In-memory emulation of the PostgreSQL subset the tool uses, registered as a
// test-only "postgres" driver. The cmd test binary does not link lib/pq, so
// the name is free here. This lets the DB-facing functions (Up, createTable,
// appliedVersions, applyMigration, Last, List) be tested without a real DB.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

func init() {
	sql.Register("postgres", memoryDriver)
}

var memoryDriver = &fakeDriver{stores: map[string]*dbState{}}

// baseStore returns the shared in-memory database bound to a DSN. The DSN is
// used as the isolation key: each test uses a unique DSN so tests never leak
// state into each other.
func baseStore(dsn string) *dbState {
	memoryDriver.mu.Lock()
	defer memoryDriver.mu.Unlock()
	s := memoryDriver.stores[dsn]
	if s == nil {
		s = newDBState()
		memoryDriver.stores[dsn] = s
	}
	return s
}

type fakeDriver struct {
	mu     sync.Mutex
	stores map[string]*dbState
}

func (d *fakeDriver) Open(name string) (driver.Conn, error) {
	return &fakeConn{store: baseStore(name), dsn: name}, nil
}

type colState struct {
	name       string
	typ        string
	defaultNow bool // column has DEFAULT CURRENT_TIMESTAMP
}

type tableState struct {
	columns []*colState
	rows    []map[string]driver.Value
}

func (t *tableState) fork() *tableState {
	n := &tableState{columns: append([]*colState(nil), t.columns...)}
	n.rows = make([]map[string]driver.Value, len(t.rows))
	for i, r := range t.rows {
		n.rows[i] = maps.Clone(r)
	}
	return n
}

func (t *tableState) column(name string) *colState {
	for _, c := range t.columns {
		if c.name == name {
			return c
		}
	}
	return nil
}

// ColumnType returns the emulated data type of a column.
func (t *tableState) ColumnType(name string) string {
	if c := t.column(name); c != nil {
		return c.typ
	}
	return ""
}

// Rows returns a copy of the stored rows.
func (t *tableState) Rows() []map[string]driver.Value {
	out := make([]map[string]driver.Value, len(t.rows))
	copy(out, t.rows)
	return out
}

// RowCount returns the number of stored rows.
func (t *tableState) RowCount() int { return len(t.rows) }

type dbState struct {
	mu     sync.Mutex
	tables map[string]*tableState
	now    func() time.Time
}

func newDBState() *dbState {
	return &dbState{tables: map[string]*tableState{}, now: time.Now}
}

func (s *dbState) fork() *dbState {
	n := newDBState()
	n.now = s.now
	for k, t := range s.tables {
		n.tables[k] = t.fork()
	}
	return n
}

func (s *dbState) publish(snap *dbState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tables = snap.tables
}

func (s *dbState) hasTable(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.tables[tableKey(name)]
	return ok
}

// TableNames returns the created tables (for store inspection in tests).
func (s *dbState) TableNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.tables))
	for k := range s.tables {
		names = append(names, k)
	}
	return names
}

func (s *dbState) columnType(table, col string) *colState {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tables[tableKey(table)]
	if !ok {
		return nil
	}
	return t.column(col)
}

type fakeConn struct {
	store *dbState // global shared state for this dsn
	dsn   string   // the connection string, used to simulate failures
	cur   *dbState // active tx snapshot, nil when not in a transaction
}

func (c *fakeConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeStmt{conn: c, query: query}, nil
}

func (c *fakeConn) Close() error { return nil }

func (c *fakeConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *fakeConn) BeginTx(_ context.Context, _ driver.TxOptions) (driver.Tx, error) {
	c.cur = c.store.fork()
	return &fakeTx{conn: c}, nil
}

func (c *fakeConn) Ping(_ context.Context) error {
	if strings.Contains(c.dsn, "ping-fail") {
		return errors.New("connection refused")
	}
	return nil
}

func (c *fakeConn) Exec(query string, args []driver.Value) (driver.Result, error) {
	_, err := c.run(query, args)
	if err != nil {
		return nil, err
	}
	return fakeResult{}, nil
}

func (c *fakeConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.Exec(query, namedToValues(args))
}

func (c *fakeConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	res, err := c.run(query, args)
	if err != nil {
		return nil, err
	}
	return &fakeRows{cols: res.cols, rows: res.rows}, nil
}

func (c *fakeConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Query(query, namedToValues(args))
}

func namedToValues(args []driver.NamedValue) []driver.Value {
	out := make([]driver.Value, len(args))
	for i, a := range args {
		out[i] = a.Value
	}
	return out
}

// run executes a single logical statement (already comment-stripped) against
// the effective state.
func (c *fakeConn) run(query string, args []driver.Value) (*queryResult, error) {
	stmts := splitStatements(query)
	if len(stmts) == 0 {
		return &queryResult{}, nil
	}

	// Inside a transaction: apply on the snapshot; Commit() publishes it.
	if c.cur != nil {
		for _, st := range stmts {
			rr, err := c.execute(c.cur, st, args)
			if err != nil {
				return nil, err
			}
			if len(stmts) == 1 {
				return rr, nil
			}
		}
		return &queryResult{}, nil
	}

	// Outside a transaction: buffer on a fork and publish on success.
	buf := c.store.fork()
	for _, st := range stmts {
		rr, err := c.execute(buf, st, args)
		if err != nil {
			return nil, err
		}
		if len(stmts) == 1 {
			c.store.publish(buf)
			return rr, nil
		}
	}
	c.store.publish(buf)
	return &queryResult{}, nil
}

func (c *fakeConn) execute(s *dbState, st string, args []driver.Value) (*queryResult, error) {
	trimmed := strings.TrimSpace(st)
	if trimmed == "" {
		return &queryResult{}, nil
	}

	switch {
	case isCreateTable(trimmed):
		return &queryResult{}, createTableMem(trimmed, s)
	case isAlterTable(trimmed):
		return &queryResult{}, alterTable(trimmed, s)
	case isInsert(trimmed):
		return &queryResult{}, insert(trimmed, s, args)
	case isSelect(trimmed):
		res, err := selectRows(trimmed, s)
		if err != nil {
			return nil, err
		}
		return res, nil
	case isConstSelect(trimmed):
		return &queryResult{}, nil
	default:
		return nil, fmt.Errorf("syntax error at or near %q", trimmed)
	}
}

type queryResult struct {
	cols []string
	rows [][]driver.Value
}

type fakeStmt struct {
	conn  *fakeConn
	query string
}

func (s *fakeStmt) Close() error  { return nil }
func (s *fakeStmt) NumInput() int { return -1 }
func (s *fakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.conn.Exec(s.query, args)
}
func (s *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.conn.Query(s.query, args)
}

type fakeTx struct {
	conn *fakeConn
	done bool
}

func (t *fakeTx) Commit() error {
	if t.done {
		return sql.ErrTxDone
	}
	t.done = true
	t.conn.store.publish(t.conn.cur)
	t.conn.cur = nil
	return nil
}

func (t *fakeTx) Rollback() error {
	if t.done {
		return sql.ErrTxDone
	}
	t.done = true
	t.conn.cur = nil // discard the snapshot
	return nil
}

type fakeRows struct {
	cols []string
	rows [][]driver.Value
	idx  int
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copyRow := r.rows[r.idx]
	r.idx++
	for i := range dest {
		if i < len(copyRow) {
			dest[i] = copyRow[i]
		} else {
			dest[i] = nil
		}
	}
	return nil
}

type fakeResult struct{}

func (fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeResult) RowsAffected() (int64, error) { return 0, nil }

// --- SQL subset parsing ---------------------------------------------------

func splitStatements(query string) []string {
	// strip line comments
	var b strings.Builder
	for line := range strings.SplitSeq(query, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte(' ')
	}

	parts := strings.Split(b.String(), ";")
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return out
}

func tableKey(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, `"`, ""))
	if _, after, found := strings.CutLast(name, "."); found {
		name = after
	}
	return name
}

func parseColDef(def string) *colState {
	fields := strings.Fields(def)
	if len(fields) == 0 {
		return nil
	}
	upper := strings.ToUpper(def)
	c := &colState{name: fields[0]}
	switch {
	case strings.Contains(upper, "BIGINT"):
		c.typ = "bigint"
	case strings.Contains(upper, "INT"):
		c.typ = "integer"
	case strings.Contains(upper, "TIMESTAMPTZ"):
		c.typ = "timestamp with time zone"
	case strings.Contains(upper, "TEXT"):
		c.typ = "text"
	default:
		c.typ = "text"
	}
	if strings.Contains(upper, "DEFAULT") {
		c.defaultNow = strings.Contains(upper, "CURRENT_TIMESTAMP")
	}
	return c
}

func splitTopLevel(s string) []string {
	var parts []string
	var cur strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, cur.String())
				cur.Reset()
				continue
			}
		}
		cur.WriteRune(r)
	}
	parts = append(parts, cur.String())
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return out
}

var (
	reCreate = regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([^\s(]+)\s*\((.*)\)\s*$`)
	reAlter  = regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+([^\s(]+)\s+(.+)$`)
	reInsert = regexp.MustCompile(`(?i)^\s*INSERT\s+INTO\s+([^\s(]+)\s*\(([^)]*)\)\s*VALUES\s*\(([^)]*)\)\s*$`)
	reSelect = regexp.MustCompile(`(?i)^\s*SELECT\s+(.+?)\s+FROM\s+([^\s;]+)(?:\s+ORDER\s+BY\s+(\S+)(?:\s+(ASC|DESC))?)?(?:\s+LIMIT\s+(\d+))?\s*$`)
)

func isCreateTable(q string) bool { return reCreate.MatchString(q) }

func isAlterTable(q string) bool { return reAlter.MatchString(q) }
func isInsert(q string) bool     { return reInsert.MatchString(q) }
func isSelect(q string) bool     { return reSelect.MatchString(q) }

func isConstSelect(q string) bool {
	m := reSelect.FindStringSubmatch(q)
	return m == nil && strings.HasPrefix(strings.ToUpper(q), "SELECT")
}

func insert(q string, s *dbState, args []driver.Value) error {
	m := reInsert.FindStringSubmatch(q)
	table := m[1]
	cols := splitTopLevel(m[2])
	placeholders := splitTopLevel(m[3])

	if len(placeholders) != len(cols) || len(cols) != len(args) {
		return fmt.Errorf("could not determine data type of parameter")
	}

	rows := s.tables[tableKey(table)]
	if rows == nil {
		return fmt.Errorf(`relation %q does not exist`, tableKey(table))
	}

	row := map[string]driver.Value{}
	for i, col := range cols {
		row[tableKey(col)] = args[i]
	}

	// apply DEFAULTs for columns not provided
	ts := rows
	for _, c := range ts.columns {
		if _, ok := row[c.name]; !ok && c.defaultNow {
			row[c.name] = s.now()
		}
	}

	ts.rows = append(ts.rows, row)
	return nil
}

func alterTable(q string, s *dbState) error {
	m := reAlter.FindStringSubmatch(q)
	table := m[1]
	rest := strings.TrimSpace(m[2])
	upper := strings.ToUpper(rest)

	ts := s.tables[tableKey(table)]
	if ts == nil {
		return fmt.Errorf(`relation %q does not exist`, tableKey(table))
	}

	switch {
	case strings.HasPrefix(upper, "ADD COLUMN IF NOT EXISTS"):
		rest = strings.TrimSpace(rest[len("ADD COLUMN IF NOT EXISTS"):])
		return addColumn(ts, rest)
	case strings.HasPrefix(upper, "ADD COLUMN"):
		rest = strings.TrimSpace(rest[len("ADD COLUMN"):])
		return addColumn(ts, rest)
	case strings.HasPrefix(upper, "ALTER COLUMN"):
		rest = strings.TrimSpace(rest[len("ALTER COLUMN"):])
		fields := strings.Fields(rest)
		if len(fields) < 1 {
			return fmt.Errorf("syntax error in ALTER COLUMN")
		}
		col := ts.column(fields[0])
		if col == nil {
			return fmt.Errorf(`column %q does not exist`, fields[0])
		}
		if strings.Contains(strings.ToUpper(rest), "SET DEFAULT") {
			if strings.Contains(strings.ToUpper(rest), "CURRENT_TIMESTAMP") {
				col.defaultNow = true
			}
		}
		return nil
	default:
		return fmt.Errorf("syntax error in ALTER TABLE")
	}
}

func addColumn(ts *tableState, def string) error {
	c := parseColDef(def)
	if c == nil {
		return fmt.Errorf("syntax error in ADD COLUMN")
	}
	if ts.column(c.name) != nil {
		return nil // IF NOT EXISTS semantics
	}
	ts.columns = append(ts.columns, c)
	return nil
}

func createTableMem(q string, s *dbState) error {
	m := reCreate.FindStringSubmatch(q)
	if m == nil {
		return fmt.Errorf("syntax error in CREATE TABLE")
	}
	name := tableKey(m[1])

	if s.tables[name] != nil {
		return nil // IF NOT EXISTS semantics
	}

	ts := &tableState{}
	for _, def := range splitTopLevel(m[2]) {
		c := parseColDef(def)
		if c != nil {
			ts.columns = append(ts.columns, c)
		}
	}
	s.tables[name] = ts
	return nil
}

func selectRows(q string, s *dbState) (*queryResult, error) {
	m := reSelect.FindStringSubmatch(q)
	if m == nil {
		return nil, fmt.Errorf("unsupported SELECT")
	}

	selCols := splitTopLevel(m[1])
	table := tableKey(m[2])
	orderCol := m[3]
	orderDesc := strings.EqualFold(m[4], "DESC")
	var limit int
	if m[5] != "" {
		_, _ = fmt.Sscanf(m[5], "%d", &limit)
	}

	ts := s.tables[table]
	if ts == nil {
		return nil, fmt.Errorf(`relation %q does not exist`, table)
	}

	res := &queryResult{cols: []string{}}
	for _, c := range selCols {
		res.cols = append(res.cols, strings.Trim(c, `"`))
	}

	indexes := make([]int, len(ts.rows))
	for i := range indexes {
		indexes[i] = i
	}

	slices.SortStableFunc(indexes, func(a, b int) int {
		cmp := compareValues(ts.rows[a][orderCol], ts.rows[b][orderCol])
		if orderDesc {
			return -cmp
		}
		return cmp
	})

	if limit > 0 && limit < len(indexes) {
		indexes = indexes[:limit]
	}

	for _, idx := range indexes {
		row := ts.rows[idx]
		vals := make([]driver.Value, len(res.cols))
		for i, col := range res.cols {
			vals[i] = row[col]
		}
		res.rows = append(res.rows, vals)
	}

	return res, nil
}

func compareValues(a, b driver.Value) int {
	ai, aok := asInt(a)
	bi, bok := asInt(b)
	if aok && bok {
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
		return 0
	}
	return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
}

func asInt(v driver.Value) (int64, bool) {
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	}
	var n int64
	if _, err := fmt.Sscan(fmt.Sprint(v), &n); err == nil {
		return n, true
	}
	return 0, false
}
