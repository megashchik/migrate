package cmd

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/megashchik/migrate/config"
)

func TestGetQuery(t *testing.T) {
	const fullTable = `"public"."schema_migrations"`

	tests := []struct {
		name        string
		command     string
		desc        bool
		ts          bool
		wantQuery   string
		wantColumns int
	}{
		{
			name:        "last minimal",
			command:     config.CommandLast,
			wantQuery:   `SELECT version FROM "public"."schema_migrations" ORDER BY version DESC LIMIT 1`,
			wantColumns: 1,
		},
		{
			name:        "list minimal",
			command:     config.CommandList,
			wantQuery:   `SELECT version FROM "public"."schema_migrations" ORDER BY version DESC`,
			wantColumns: 1,
		},
		{
			name:        "last with description",
			command:     config.CommandLast,
			desc:        true,
			wantQuery:   `SELECT version, description FROM "public"."schema_migrations" ORDER BY version DESC LIMIT 1`,
			wantColumns: 2,
		},
		{
			name:        "list with description and ts",
			command:     config.CommandList,
			desc:        true,
			ts:          true,
			wantQuery:   `SELECT version, description, applied_at FROM "public"."schema_migrations" ORDER BY version DESC`,
			wantColumns: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &config.Config{Command: tt.command, Desc: tt.desc, Ts: tt.ts, FullTableName: fullTable}
			query, values, err := getQuery(c)
			if err != nil {
				t.Fatalf("getQuery error: %v", err)
			}
			if query != tt.wantQuery {
				t.Errorf("getQuery = %q, want %q", query, tt.wantQuery)
			}
			if len(values) != tt.wantColumns {
				t.Errorf("getQuery values len = %d, want %d", len(values), tt.wantColumns)
			}
		})
	}

	t.Run("unknown command errors", func(t *testing.T) {
		c := &config.Config{Command: "bogus", FullTableName: fullTable}
		if _, _, err := getQuery(c); err == nil {
			t.Fatal("expected error for unknown command, got nil")
		}
	})

	t.Run("scan destinations are typed pointers", func(t *testing.T) {
		c := &config.Config{Command: config.CommandLast, Desc: true, Ts: true, FullTableName: fullTable}
		_, values, err := getQuery(c)
		if err != nil {
			t.Fatalf("getQuery error: %v", err)
		}
		if _, ok := values[0].(*int64); !ok {
			t.Errorf("values[0] type = %T, want *int64", values[0])
		}
		if _, ok := values[1].(*sql.NullString); !ok {
			t.Errorf("values[1] type = %T, want *sql.NullString", values[1])
		}
		if _, ok := values[2].(*sql.NullTime); !ok {
			t.Errorf("values[2] type = %T, want *sql.NullTime", values[2])
		}
	})
}

func TestFormatValue(t *testing.T) {
	fixed := time.Date(2026, 9, 6, 10, 51, 44, 0, time.UTC)

	tests := []struct {
		name string
		val  any
		want string
	}{
		{name: "int64", val: ptr(int64(20260906105144950)), want: "20260906105144950"},
		{name: "valid string", val: &sql.NullString{Valid: true, String: "create users"}, want: "create users"},
		{name: "invalid string shows dash", val: &sql.NullString{}, want: "-"},
		{name: "valid time", val: &sql.NullTime{Valid: true, Time: fixed}, want: "2026-09-06T10:51:44Z"},
		{name: "invalid time shows dash", val: &sql.NullTime{}, want: "-"},
		{name: "unknown type shows dash", val: (*int)(nil), want: "-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatValue(tt.val); got != tt.want {
				t.Errorf("formatValue(%T) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

func TestFormatRow(t *testing.T) {
	fixed := time.Date(2026, 9, 6, 10, 51, 44, 0, time.UTC)

	values := []any{
		ptr(int64(1)),
		&sql.NullString{Valid: true, String: "first"},
		&sql.NullTime{Valid: true, Time: fixed},
	}

	got := formatRow(values)
	want := "1  first  2026-09-06T10:51:44Z"
	if got != want {
		t.Errorf("formatRow = %q, want %q", got, want)
	}

	got = formatRow(nil)
	if got != "" {
		t.Errorf("formatRow(nil) = %q, want empty", got)
	}
}

func TestFormatRowHandlesNulls(t *testing.T) {
	got := formatRow([]any{
		ptr(int64(7)),
		&sql.NullString{},
		&sql.NullTime{},
	})
	if !strings.Contains(got, "7  -  -") {
		t.Errorf("formatRow with nulls = %q, want it to contain %q", got, "7  -  -")
	}
}

func ptr[T any](v T) *T { return &v }
