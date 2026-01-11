# Migrate

A minimalist, CLI tool for PostgreSQL schema migrations written in Go.  

**No complex configurations. Just plain SQL.**

## ✨ Key Features

- **Pure SQL**: No specific markers like `-- migrate:up` required. If it's a valid SQL, it works.
- **Flexible Versioning**: Supports Unix timestamps, `YYYYMMDD`, and incremental numbers (`000001`).
- **Description Support**: Automatically extracts descriptions from SQL comments (`-- desc: text`) or filenames.
- **Single Binary**: No dependencies. Built with Go standard library and `pq` driver.
- **Safety First**: Each migration runs in a single transaction.

## 🚀 Installation

```bash
go install github.com/megashchik/migrate
```

🛠 Usage
1. Initialize Infrastructure
Migrate automatically creates the table if it doesn't exist.
```bash
migrate -conn "postgres://user:pass@localhost:5432/db_name?sslmode=disable"
```

2. Create a New Migration
Create a new empty SQL file with a proper version prefix:
```bash
migrate new create_users_table
# Generates: ./migrations/20260109223005_create_users_table.sql
```

3. Apply Migrations
Simply run the tool. It will scan the directory and apply only new files.
```bash
migrate -dir ./sql_migrations
```

⚙️ Configuration
| Flag  | ENV Variable | Default      | Description                             |
| ----- | ------------ | ------------ | --------------------------------------- |
| -conn | DATABASE_URL | -	          | PostgreSQL connection string            |
| -dir  |	-            | ./migrations	| ./migrations Path to your .sql files    |
| -f	  | -            | T            | Prefix format (0, 000, T, U)            |
| -desc	| -            | false        | Enable/disable description column in DB |


📂 Migration File Example
You can add an optional description comment at the top of your file:
```sql
-- desc: Adds phone column to users table
ALTER TABLE users ADD COLUMN phone TEXT;
```

## Docker Usage
You can run migrations using Docker without installing Go:

You need pull the image first:
```bash
docker pull megashchik/migrate
```

Create new migration
```bash
docker run --rm -v $(pwd)/migrations:/migrations megashchik/migrate new migration-name
```

Run migrations
```bash
docker run --rm \
  -v $(pwd)/migrations:/migrations \
  -e DATABASE_URL="postgres://user:pass@host:5432/db" \
  megashchik/migrate -dir /migrations
```

⌨️ Advanced Commands  
migrate list: Show all applied migrations.  
migrate last: Display the latest applied version number.  
migrate -extra: Show advanced flags (e.g., -short for INT4 version column).  

🤝 Contributing
Fork the repository.  
Create your feature branch (git checkout -b feature/amazing-feature).  
Commit your changes (git commit -m 'Add some amazing feature').  
Push to the branch (git push origin feature/amazing-feature).  
Open a Pull Request.

📄 License
Distributed under the Apache 2.0 License. See LICENSE for more information.
