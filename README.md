# DBExec

A secure tool for executing predefined SQL queries with parameter validation, transaction support, and preview capabilities.

## Overview

DBExec allows you to define SQL queries in a YAML configuration file and execute them safely with parameter validation. It provides:

- Transaction support (all queries run in a single transaction)
- Parameter validation against an allowed list
- Preview mode to see what would be affected before making changes
- Row limit enforcement to prevent accidental large-scale changes
- Secure parameter handling to prevent SQL injection

## Installation

```bash
go install github.com/tendant/dbexec@latest
```

Or clone the repository and build:

```bash
git clone https://github.com/tendant/dbexec.git
cd dbexec
go build
```

## Configuration

Define your queries in a YAML file (default: `queries.yaml`):

```yaml
- id: update_user_status
  description: Update a user's status
  sql: UPDATE users SET status = $1 WHERE user_id = $2
  requires_approval: true
  max_rows_affected: 1
  allowed_params:
    - status
    - user_id

- id: delete_inactive_users
  description: Delete users who have been inactive for a specified period
  sql: DELETE FROM users WHERE last_login < NOW() - INTERVAL '$1 days' AND status = 'inactive'
  requires_approval: true
  max_rows_affected: 100
  allowed_params:
    - days
```

### Query Definition Fields

- `id`: Unique identifier for the query
- `description`: Human-readable description
- `sql`: The SQL query to execute (with positional parameters)
- `requires_approval`: Whether this query requires explicit approval
- `max_rows_affected`: Maximum number of rows that can be affected (0 for unlimited)
- `allowed_params`: List of parameter names that are allowed for this query

## Usage

```bash
# Set required environment variables
export DATABASE_URL="postgres://username:password@localhost:5432/mydb"
export QUERY_DEFINITIONS_PATH="./queries.yaml"  # Optional, defaults to queries.yaml

# Preview mode (dry run)
dbexec --queries="update_user_status" --params='{"status":"active","user_id":"123"}'

# Execute mode (with approval)
dbexec --queries="update_user_status" --params='{"status":"active","user_id":"123"}' --approve
```

### Multiple Queries

You can execute multiple queries in a single transaction:

```bash
dbexec --queries="update_user_status,delete_inactive_users" --params='{"status":"active","user_id":"123","days":"90"}' --approve
```

## Environment Variables

- `DATABASE_URL`: PostgreSQL connection string (required)
- `QUERY_DEFINITIONS_PATH`: Path to the YAML file containing query definitions (optional, defaults to `queries.yaml`)

## Security Considerations

- All queries run in a transaction, ensuring atomicity
- Parameters are validated against an allowed list
- Preview mode allows checking affected rows before committing changes
- Row limits prevent accidental large-scale data modifications
- Positional parameters prevent SQL injection

## License

See the [LICENSE](LICENSE) file for details.
