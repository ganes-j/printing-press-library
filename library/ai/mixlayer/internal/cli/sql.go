// Copyright 2026 Cathryn Lavery and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/ai/mixlayer/internal/store"
	"github.com/spf13/cobra"
)

// pp:data-source local
func newSQLCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:         "sql <query>",
		Short:       "Run read-only SQL against the local reasoning ledger",
		Example:     `  mixlayer-pp-cli sql "select model, count(*) from runs group by model" --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			query := strings.TrimSpace(args[0])
			if !strings.HasPrefix(strings.ToLower(query), "select") && !strings.HasPrefix(strings.ToLower(query), "with") {
				return usageErr(fmt.Errorf("only SELECT/CTE read queries are allowed"))
			}
			if dbPath == "" {
				dbPath = defaultDBPath("mixlayer-pp-cli")
			}
			s, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer s.Close()
			rows, err := s.DB().QueryContext(cmd.Context(), query)
			if err != nil {
				return err
			}
			defer rows.Close()
			out, err := rowsToMaps(rows)
			if err != nil {
				return err
			}
			return outputJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path")
	return cmd
}

func rowsToMaps(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := map[string]any{}
		for i, c := range cols {
			switch v := values[i].(type) {
			case []byte:
				row[c] = string(v)
			default:
				row[c] = v
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
