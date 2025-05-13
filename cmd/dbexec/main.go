// dbexec is a tool for securely executing predefined SQL queries with parameter validation.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"gopkg.in/yaml.v3"
)

type QueryDefinition struct {
	ID               string   `yaml:"id" json:"id"`
	Description      string   `yaml:"description" json:"description"`
	SQL              string   `yaml:"sql" json:"sql"`
	RequiresApproval bool     `yaml:"requires_approval" json:"requires_approval"`
	MaxRowsAffected  int      `yaml:"max_rows_affected" json:"max_rows_affected"`
	AllowedParams    []string `yaml:"allowed_params" json:"allowed_params"`
}

var queries = map[string]QueryDefinition{}

// loadQueriesFromYAML loads query definitions from a YAML file and stores them in the queries map.
func loadQueriesFromYAML(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read YAML file: %w", err)
	}

	var list []QueryDefinition
	if err := yaml.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	for _, q := range list {
		queries[q.ID] = q
	}
	return nil
}

// runQueriesInTransaction executes a list of predefined queries within a single transaction.
// If approve is false, it performs a dry run without committing changes.
func runQueriesInTransaction(db *sql.DB, ids []string, params map[string]string, approve bool) error {
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			tx.Rollback() // Will be ignored if already committed
		}
	}()

	for _, id := range ids {
		qdef, ok := queries[strings.TrimSpace(id)]
		if !ok {
			return fmt.Errorf("unknown query ID: %s", id)
		}

		args := []interface{}{}
		for _, key := range qdef.AllowedParams {
			val, ok := params[key]
			if !ok {
				return fmt.Errorf("missing parameter: %s", key)
			}
			args = append(args, val)
		}


		// Check if this is a SELECT query
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(qdef.SQL)), "SELECT") {
			// For SELECT statements, use QueryContext and print results
			rows, err := tx.QueryContext(ctx, qdef.SQL, args...)
			if err != nil {
				return fmt.Errorf("execution error for %s: %v", id, err)
			}
			defer rows.Close()

			// Print the query results
			prefix := "[EXECUTED]"
			title := "Results:"
			rowCount, err := printQueryResults(rows, qdef.ID, prefix, title)
			if err != nil {
				return fmt.Errorf("error printing results for %s: %v", id, err)
			}

			fmt.Printf("Total rows: %d\n\n", rowCount)
		} else if !approve {
			// For preview mode, create a simple SELECT statement
			// Extract table name and WHERE clause from the UPDATE statement
			sql := qdef.SQL
			
			// Normalize SQL by removing newlines and extra spaces
			normalizedSQL := strings.Join(strings.Fields(sql), " ")
			upper := strings.ToUpper(normalizedSQL)
			
			// Find key parts of the SQL
			updateIndex := strings.Index(upper, "UPDATE ")
			setIndex := strings.Index(upper, " SET ")
			whereIndex := strings.Index(upper, " WHERE ")
			
			var previewSQL string
			
			if updateIndex != -1 && setIndex != -1 && updateIndex < setIndex {
				// Extract table name
				tableName := strings.TrimSpace(normalizedSQL[updateIndex+7:setIndex])
				
				// Build a simple SELECT statement
				if whereIndex != -1 && whereIndex > setIndex {
					whereClause := normalizedSQL[whereIndex:]
					previewSQL = fmt.Sprintf("SELECT * FROM %s %s", tableName, whereClause)
				} else {
					previewSQL = fmt.Sprintf("SELECT * FROM %s", tableName)
				}
			} else {
				// Fallback to original SQL with a comment
				previewSQL = "-- Could not parse UPDATE statement properly\n" + sql
				return fmt.Errorf("could not parse UPDATE statement for preview: %s", id)
			}
			
			fmt.Printf("[PREVIEW] Using query: %s\n", previewSQL)
			rows, err := tx.QueryContext(ctx, previewSQL, args...)
			if err != nil {
				return fmt.Errorf("preview failed for %s: %v", id, err)
			}
			defer rows.Close()
			
			// Print the query results
			prefix := "[PREVIEW]"
			title := "Results that would be affected by the UPDATE:"
			rowCount, err := printQueryResults(rows, qdef.ID, prefix, title)
			if err != nil {
				return fmt.Errorf("error printing preview results for %s: %v", id, err)
			}
			
			fmt.Printf("Total rows that would be affected: %d\n\n", rowCount)
			continue
		} else {
			// For non-SELECT statements, use ExecContext
			res, err := tx.ExecContext(ctx, qdef.SQL, args...)
			if err != nil {
				return fmt.Errorf("execution error for %s: %v", id, err)
			}
			n, _ := res.RowsAffected()
			if qdef.MaxRowsAffected > 0 && int(n) > qdef.MaxRowsAffected {
				return fmt.Errorf("exceeded row limit for %s: %d > %d", id, n, qdef.MaxRowsAffected)
			}

			fmt.Printf("[EXECUTED] QueryID=%s RowsAffected=%d\n", qdef.ID, n)
		}
	}

	if approve {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
		tx = nil // Prevent rollback in defer
		fmt.Println("All queries committed successfully.")
	} else {
		fmt.Println("Dry run completed. No changes applied.")
	}
	return nil
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	yamlPath := os.Getenv("QUERY_DEFINITIONS_PATH")
	if yamlPath == "" {
		yamlPath = "queries.yaml"
	}
	if err := loadQueriesFromYAML(yamlPath); err != nil {
		log.Fatalf("Failed to load queries: %v", err)
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// CLI flags
	queryIDs := flag.String("queries", "", "Comma-separated list of query IDs to run")
	paramsJSON := flag.String("params", "", "JSON string of parameters for all queries")
	approve := flag.Bool("approve", false, "Set to true to execute (false for preview)")
	flag.Parse()

	if *queryIDs == "" || *paramsJSON == "" {
		log.Fatal("You must provide --queries and --params")
	}

	var params map[string]string
	if err := json.Unmarshal([]byte(*paramsJSON), &params); err != nil {
		log.Fatalf("Failed to parse parameters: %v", err)
	}

	ids := strings.Split(*queryIDs, ",")
	if err := runQueriesInTransaction(db, ids, params, *approve); err != nil {
		log.Fatalf("Error executing queries: %v", err)
	}
}

// printQueryResults formats and prints the results of a SQL query
func printQueryResults(rows *sql.Rows, queryID, prefix, title string) (int, error) {
	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("failed to get columns: %v", err)
	}
	
	fmt.Printf("%s QueryID=%s\n", prefix, queryID)
	fmt.Println(title)
	
	// Prepare values to scan into
	values := make([]interface{}, len(columns))
	scanArgs := make([]interface{}, len(columns))
	for i := range values {
		scanArgs[i] = &values[i]
	}
	
	// Print each row
	rowCount := 0
	for rows.Next() {
		err = rows.Scan(scanArgs...)
		if err != nil {
			return rowCount, fmt.Errorf("error scanning row: %v", err)
		}
		
		// Print each column on a new line
		fmt.Printf("Row %d:\n", rowCount+1)
		fmt.Println(strings.Repeat("-", 40))
		
		for i, col := range columns {
			// Format the value based on type
			var displayVal string
			v := values[i]
			
			if v == nil {
				displayVal = "<NULL>"
			} else {
				switch val := v.(type) {
				case []byte:
					// Try to convert byte slice to UUID string if it looks like a UUID
					if len(val) == 16 {
						// Format as UUID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
						displayVal = fmt.Sprintf("%x-%x-%x-%x-%x", 
							val[0:4], val[4:6], val[6:8], val[8:10], val[10:16])
					} else {
						// Try to convert to string
						displayVal = string(val)
					}
				case time.Time:
					// Format time values consistently
					displayVal = val.Format("2006-01-02 15:04:05")
				default:
					// Use default formatting for other types
					displayVal = fmt.Sprintf("%v", val)
				}
			}
			
			fmt.Printf("  %s: %s\n", col, displayVal)
		}
		fmt.Println()
		rowCount++
	}
	
	if err = rows.Err(); err != nil {
		return rowCount, fmt.Errorf("error iterating rows: %v", err)
	}
	
	return rowCount, nil
}
