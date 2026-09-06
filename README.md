# Migrate

![Go version](https://img.shields.io/github/go-mod/go-version/megashchik/migrate)
![License](https://img.shields.io/github/license/megashchik/migrate)

**The minimalist PostgreSQL migration tool — pure SQL, zero configuration, no boilerplate markers.**

Just point it at a folder of `.sql` files and a connection string. No `-- +goose Up/Down` markers, no config files, no migration packages. If it's valid SQL, it migrates.

## ✨ Why migrate?

Most migration tools make you adopt their workflow. `migrate` makes you adopt nothing:

- **Pure SQL, no annotations.** goose requires you to sprinkle `-- +goose Up/Down` markers through every file. Here, any valid `.sql` file just works — your existing SQL stays untouched.
- **Zero configuration.** A single static binary (`go install`), one connection string, one directory. No config files, no embedded migration language.
- **Safe by default.** Each migration runs in its own transaction.
- **CI-friendly out of the box.** A built-in `check` command fails the build on duplicate version numbers — no third-party tool needed.
- **Flexible versioning.** Timestamps (with millisecond precision), Unix epoch, or auto-incremented numbers (`000001`) — whatever fits your repo.
- **Auto-extracted descriptions.** Pulled from a `-- desc:` comment or, failing that, from the migration filename.
- **Applied-at timestamps.** Optionally records when each migration ran (`-ts` flag).
- **Single binary, no dependencies.** Built with the Go standard library and the `pq` driver.

## 🚀 Quick Start

```bash
# 1. Install
go install github.com/megashchik/migrate

# 2. Create a migration
migrate new create_users_table
# Generates: ./migrations/20260109223005123_create_users_table.sql

# 3. Apply it
migrate -conn "postgres://user:pass@localhost:5432/db_name?sslmode=disable"
```

## 🛠 Usage

### 1. Initialize Infrastructure

Migrate automatically creates the migration table if it doesn't exist. The default directory is `./migrations`.

### 2. Create a New Migration

Create a new empty SQL file with a proper version prefix:

```bash
migrate new create_users_table
# Generates: ./migrations/20260109223005123_create_users_table.sql
```

### 3. Apply Migrations

Simply run the tool. It will scan the `./migrations` directory and apply only new files:

```bash
migrate -conn "postgres://user:pass@localhost:5432/db_name?sslmode=disable"
```

To use a different directory, pass it to both `new` and apply:

```bash
migrate -dir ./sql_migrations new create_users_table
migrate -dir ./sql_migrations -conn "postgres://user:pass@localhost:5432/db_name?sslmode=disable"
```

## ⚙️ Configuration

| Flag        | ENV Variable   | Default            | Description                                  |
| ----------- | -------------- | ------------------ | -------------------------------------------- |
| `-conn`     | `DATABASE_URL` | -                  | PostgreSQL connection string                 |
| `-dir`      | -              | `./migrations`     | Path to your `.sql` files                    |
| `-extra`    | -              | `false`            | Show advanced flags in `help`                |
| `-t`        | -              | `schema_migrations`| Metadata table name                          |
| `-schema`   | -              | `public`           | Database schema for the metadata table       |
| `-short`    | -              | `false`            | Use INT4 instead of INT8 for the version column |
| `-desc`     | -              | `false`            | Enable the description column                |
| `-ts`       | -              | `false`            | Enable the `applied_at` timestamp column     |
| `-env-url`  | -              | `DATABASE_URL`     | Env variable holding the connection string   |
| `-f`        | -              | `T`                | Version prefix format — see legend below     |

**`-f` format legend:**

| Value | Meaning                                    | Example                   |
| ----- | ------------------------------------------ | ------------------------- |
| `T`   | Timestamp (millisecond precision)          | `20260109223005123`       |
| `U`   | Unix epoch (seconds)                       | `1736512272`              |
| `0`   | Incremental, auto-padded to 6 digits       | `000001`, `000002`        |
| `0000`| Incremental, padded to the number of zeros | `0001`, `0002`            |

## 📂 Migration File Example

You can add an optional description comment at the top of your file:

```sql
-- desc: Adds phone column to users table
ALTER TABLE users ADD COLUMN phone TEXT;
```

## ⌨️ Advanced Commands

- `migrate list` — Show all applied migrations.
- `migrate last` — Display the latest applied version number.
- `migrate check` — Fail if two `.sql` files share the same version number (CI-friendly).
- `migrate -extra help` — Show advanced flags (e.g., `-short` for INT4 version column).

## ⚖️ Comparison

| Aspect                     | migrate | goose  | golang-migrate               |
| -------------------------- | ------- | ------ | ---------------------------- |
| Pure SQL, no annotations   | ✅ Yes  | ❌ No   | ✅ Yes                       |
| Single binary, `go install`| ✅ Yes  | ✅ Yes  | ⚠️ CLI + library, needs driver setup (`-tags postgres` or prebuilt binary) |
| Built-in CI `check`        | ✅ Yes  | ❌ No   | ❌ No                        |
| Config files required      | ❌ No   | ❌ No   | ❌ No                        |
| Version formats            | T / U / incremental | timestamp+seq | timestamp+seq |
| Transaction per migration  | ✅ Yes  | ✅ Yes  | ✅ Yes                       |

## 🐳 Docker Usage

You can run migrations using Docker without installing Go.

You need to pull the image first:

```bash
docker pull megashchik/migrate
```

Create a new migration:

```bash
docker run --rm -v $(pwd)/migrations:/migrations megashchik/migrate new migration-name
```

Apply migrations:

```bash
# Note: -dir points to the path inside the container (see the -v mount above)
docker run --rm \
  -v $(pwd)/migrations:/migrations \
  -e DATABASE_URL="postgres://user:pass@host:5432/db" \
  megashchik/migrate -dir /migrations
```

## 🤝 Contributing

1. Fork the repository.
2. Create your feature branch (`git checkout -b feature/amazing-feature`).
3. Commit your changes (`git commit -m 'Add some amazing feature'`).
4. Push to the branch (`git push origin feature/amazing-feature`).
5. Open a Pull Request.

## 📄 License

Distributed under the Apache 2.0 License. See [LICENSE](LICENSE) for more information.