// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.
// Hand-built Phase 3 transcendence commands.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"lawmatics-pp-cli/internal/cliutil"
	"lawmatics-pp-cli/internal/store"
)

// openLocalStore opens the local SQLite store read-only. Returns a friendly
// error suggesting `sync` when the file is missing.
func openLocalStore(cmd *cobra.Command) (*store.Store, error) {
	path := defaultDBPath("lawmatics-pp-cli")
	db, err := store.OpenWithContext(cmd.Context(), path)
	if err != nil {
		return nil, fmt.Errorf("opening local store at %s: %w\nhint: run 'lawmatics-pp-cli sync' to populate it", path, err)
	}
	return db, nil
}

// loadResourceObjects pulls all rows of a resource_type out of the generic
// resources table as decoded maps. Returns an empty slice when nothing is
// synced yet.
func loadResourceObjects(db *store.Store, resourceType string) ([]map[string]any, error) {
	rows, err := db.Query(`SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(raw), &obj); err != nil {
			continue
		}
		out = append(out, obj)
	}
	return out, rows.Err()
}

func emptyResultMessage(cmd *cobra.Command, flags *rootFlags, resource, msg string) error {
	payload := map[string]any{
		"results": []any{},
		"meta": map[string]any{
			"resource": resource,
			"message":  msg,
		},
	}
	return printJSONFiltered(cmd.OutOrStdout(), payload, flags)
}

// parseDuration accepts compact forms like "2h", "1d", "30m", "7d".
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	// Allow "Nd" for N days.
	if strings.HasSuffix(s, "d") {
		nStr := strings.TrimSuffix(s, "d")
		n, err := strconv.Atoi(nStr)
		if err != nil {
			return 0, fmt.Errorf("invalid days: %s", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// ---------------- 1. bottleneck ----------------

func newBottleneckCmd(flags *rootFlags) *cobra.Command {
	var pipeline string
	cmd := &cobra.Command{
		Use:         "bottleneck",
		Short:       "Median dwell-time per pipeline stage from local matters",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: strings.Trim(`
  lawmatics-pp-cli bottleneck
  lawmatics-pp-cli bottleneck --pipeline intake --json
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = pipeline
			if dryRunOK(flags) {
				return nil
			}
			db, err := openLocalStore(cmd)
			if err != nil {
				return err
			}
			defer db.Close()

			matters, err := loadResourceObjects(db, "matters")
			if err != nil {
				return err
			}
			if len(matters) == 0 {
				return emptyResultMessage(cmd, flags, "matters", "no matters synced; run 'sync' first")
			}

			// Group dwell-time (now - updated_at) by stage; produce median.
			now := time.Now()
			byStage := map[string][]float64{}
			for _, m := range matters {
				stage, _ := m["stage"].(string)
				if stage == "" {
					if s, ok := m["pipeline_stage"].(string); ok {
						stage = s
					}
				}
				if stage == "" {
					stage = "(unknown)"
				}
				ts, _ := m["updated_at"].(string)
				if ts == "" {
					ts, _ = m["created_at"].(string)
				}
				if ts == "" {
					continue
				}
				t, err := time.Parse(time.RFC3339, ts)
				if err != nil {
					continue
				}
				byStage[stage] = append(byStage[stage], now.Sub(t).Hours())
			}

			type row struct {
				Stage        string  `json:"stage"`
				Count        int     `json:"count"`
				MedianHours  float64 `json:"median_hours"`
				P90Hours     float64 `json:"p90_hours"`
			}
			out := make([]row, 0, len(byStage))
			for stage, vals := range byStage {
				sort.Float64s(vals)
				med := vals[len(vals)/2]
				p90 := vals[int(float64(len(vals))*0.9)]
				out = append(out, row{Stage: stage, Count: len(vals), MedianHours: med, P90Hours: p90})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].MedianHours > out[j].MedianHours })
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&pipeline, "pipeline", "", "Limit to a single pipeline (e.g. intake)")
	return cmd
}

// ---------------- 2. overdue ----------------

func newOverdueCmd(flags *rootFlags) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:         "overdue",
		Short:       "Past-due tasks grouped by assignee",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: strings.Trim(`
  lawmatics-pp-cli overdue
  lawmatics-pp-cli overdue --by assignee --json
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = by
			if dryRunOK(flags) {
				return nil
			}
			db, err := openLocalStore(cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			tasks, err := loadResourceObjects(db, "tasks")
			if err != nil {
				return err
			}
			if len(tasks) == 0 {
				return emptyResultMessage(cmd, flags, "tasks", "no tasks synced; run 'sync' first")
			}
			now := time.Now()
			byAssignee := map[string]int{}
			type item struct {
				ID       string `json:"id"`
				Assignee string `json:"assignee"`
				Due      string `json:"due_date"`
				Name     string `json:"name,omitempty"`
			}
			var items []item
			for _, t := range tasks {
				done, _ := t["completed"].(bool)
				if done {
					continue
				}
				due, _ := t["due_date"].(string)
				if due == "" {
					continue
				}
				parsed, err := time.Parse(time.RFC3339, due)
				if err != nil {
					// Try date-only.
					parsed, err = time.Parse("2006-01-02", due)
					if err != nil {
						continue
					}
				}
				if !parsed.Before(now) {
					continue
				}
				assignee := fmt.Sprintf("%v", t["assignee_id"])
				if assignee == "<nil>" || assignee == "" {
					assignee = "(unassigned)"
				}
				id := fmt.Sprintf("%v", t["id"])
				name, _ := t["name"].(string)
				items = append(items, item{ID: id, Assignee: assignee, Due: due, Name: name})
				byAssignee[assignee]++
			}
			payload := map[string]any{
				"by_assignee": byAssignee,
				"items":       items,
				"total":       len(items),
			}
			return printJSONFiltered(cmd.OutOrStdout(), payload, flags)
		},
	}
	cmd.Flags().StringVar(&by, "by", "assignee", "Group by: assignee|matter")
	return cmd
}

// ---------------- 3. since ----------------

func newSinceCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "since [duration]",
		Short:       "Entities created/updated in last N (e.g. 2h, 1d)",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: strings.Trim(`
  lawmatics-pp-cli since 2h
  lawmatics-pp-cli since 24h --json
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			window := args[0]
			dur, err := parseDuration(window)
			if err != nil {
				return usageErr(err)
			}
			cutoff := time.Now().Add(-dur)
			db, err := openLocalStore(cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			type rec struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				UpdatedAt string `json:"updated_at"`
			}
			var out []rec
			rows, err := db.Query(`SELECT resource_type, id, updated_at FROM resources WHERE updated_at >= ? ORDER BY updated_at DESC`, cutoff.UTC().Format("2006-01-02 15:04:05"))
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var r rec
					if err := rows.Scan(&r.Type, &r.ID, &r.UpdatedAt); err != nil {
						return err
					}
					out = append(out, r)
				}
			}
			payload := map[string]any{
				"window":  window,
				"cutoff":  cutoff.UTC().Format(time.RFC3339),
				"results": out,
				"count":   len(out),
			}
			return printJSONFiltered(cmd.OutOrStdout(), payload, flags)
		},
	}
	return cmd
}

// ---------------- 4. cf / cf report ----------------

func newCfCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cf",
		Short: "Custom-field reporting (pivot contacts/matters with custom fields)",
		RunE:  parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newCfReportCmd(flags))
	return cmd
}

func newCfReportCmd(flags *rootFlags) *cobra.Command {
	var fieldList string
	var where string
	cmd := &cobra.Command{
		Use:         "report",
		Short:       "Pivot contacts+matters+custom-fields to CSV/JSON",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: strings.Trim(`
  lawmatics-pp-cli cf report --fields name,email
  lawmatics-pp-cli cf report --fields name,custom.budget --where stage=Discovery --csv
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := openLocalStore(cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			contacts, err := loadResourceObjects(db, "contacts")
			if err != nil {
				return err
			}
			matters, _ := loadResourceObjects(db, "matters")
			rows := append([]map[string]any{}, contacts...)
			rows = append(rows, matters...)

			var whereKey, whereVal string
			if where != "" {
				if k, v, ok := strings.Cut(where, "="); ok {
					whereKey, whereVal = k, v
				}
			}
			fields := strings.Split(fieldList, ",")
			for i := range fields {
				fields[i] = strings.TrimSpace(fields[i])
			}

			var out []map[string]any
			for _, r := range rows {
				if whereKey != "" {
					if fmt.Sprintf("%v", r[whereKey]) != whereVal {
						continue
					}
				}
				proj := map[string]any{}
				if fieldList == "" {
					proj = r
				} else {
					for _, f := range fields {
						if f == "" {
							continue
						}
						proj[f] = r[f]
					}
				}
				out = append(out, proj)
			}
			if len(out) == 0 {
				return emptyResultMessage(cmd, flags, "cf.report", "no rows matched; ensure contacts/matters are synced and --where matches a field")
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&fieldList, "fields", "", "Comma-separated fields to include")
	cmd.Flags().StringVar(&where, "where", "", "Filter expression key=value (exact match)")
	return cmd
}

// ---------------- 5. bulk / bulk reassign ----------------

func newBulkCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bulk",
		Short: "Bulk write operations across many entities",
		RunE:  parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newBulkReassignCmd(flags))
	return cmd
}

func newBulkReassignCmd(flags *rootFlags) *cobra.Command {
	var fromUser, toUser, filter string
	cmd := &cobra.Command{
		Use:   "reassign",
		Short: "Reassign tasks matching filter from --from to --to",
		Example: strings.Trim(`
  lawmatics-pp-cli bulk reassign --from u_123 --to u_456 --filter status=open --dry-run
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if fromUser == "" || toUser == "" {
				return cmd.Help()
			}
			if cliutil.IsVerifyEnv() {
				fmt.Fprintf(cmd.OutOrStdout(), "verify-env: would reassign tasks matching %q from %s to %s\n", filter, fromUser, toUser)
				return nil
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would reassign tasks matching %q from %s to %s\n", filter, fromUser, toUser)
				return nil
			}
			db, err := openLocalStore(cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			tasks, err := loadResourceObjects(db, "tasks")
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			var changed []string
			for _, t := range tasks {
				assignee := fmt.Sprintf("%v", t["assignee_id"])
				if assignee != fromUser {
					continue
				}
				if filter != "" {
					if k, v, ok := strings.Cut(filter, "="); ok {
						if fmt.Sprintf("%v", t[k]) != v {
							continue
						}
					}
				}
				id := fmt.Sprintf("%v", t["id"])
				body := map[string]any{"assignee_id": toUser}
				if _, _, err := c.Patch("/v1/tasks/"+id, body); err != nil {
					return classifyAPIError(err, flags)
				}
				changed = append(changed, id)
			}
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"reassigned": changed, "count": len(changed)}, flags)
		},
	}
	cmd.Flags().StringVar(&fromUser, "from", "", "Current assignee ID")
	cmd.Flags().StringVar(&toUser, "to", "", "New assignee ID")
	cmd.Flags().StringVar(&filter, "filter", "", "Optional filter key=value")
	return cmd
}

// ---------------- 6. velocity ----------------

func newVelocityCmd(flags *rootFlags) *cobra.Command {
	var window string
	var by string
	cmd := &cobra.Command{
		Use:         "velocity",
		Short:       "Conversion rate per source over a time window",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: strings.Trim(`
  lawmatics-pp-cli velocity --by source --window 90d --json
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = by
			if dryRunOK(flags) {
				return nil
			}
			dur, err := parseDuration(window)
			if err != nil {
				return usageErr(err)
			}
			cutoff := time.Now().Add(-dur)
			db, err := openLocalStore(cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			prospects, err := loadResourceObjects(db, "prospects")
			if err != nil {
				return err
			}
			matters, _ := loadResourceObjects(db, "matters")
			if len(prospects) == 0 && len(matters) == 0 {
				return emptyResultMessage(cmd, flags, "velocity", "no prospects/matters synced")
			}
			leads := map[string]int{}
			wins := map[string]int{}
			for _, p := range prospects {
				src, _ := p["source"].(string)
				if src == "" {
					src = "(unknown)"
				}
				ts, _ := p["created_at"].(string)
				if t, err := time.Parse(time.RFC3339, ts); err == nil && t.After(cutoff) {
					leads[src]++
				}
			}
			for _, m := range matters {
				src, _ := m["source"].(string)
				if src == "" {
					src = "(unknown)"
				}
				wins[src]++
			}
			type row struct {
				Source     string  `json:"source"`
				Leads      int     `json:"leads"`
				Wins       int     `json:"wins"`
				Conversion float64 `json:"conversion_rate"`
			}
			var out []row
			for src, n := range leads {
				w := wins[src]
				rate := 0.0
				if n > 0 {
					rate = float64(w) / float64(n)
				}
				out = append(out, row{Source: src, Leads: n, Wins: w, Conversion: rate})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Conversion > out[j].Conversion })
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&window, "window", "30d", "Time window (e.g. 7d, 24h)")
	cmd.Flags().StringVar(&by, "by", "source", "Group by: source")
	return cmd
}

// ---------------- 7. intake / intake stale ----------------

func newIntakeCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "intake",
		Short: "Intake/prospect workflow analytics",
		RunE:  parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newIntakeStaleCmd(flags))
	return cmd
}

func newIntakeStaleCmd(flags *rootFlags) *cobra.Command {
	var days int
	cmd := &cobra.Command{
		Use:         "stale",
		Short:       "Prospects with no interaction in --days N",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: strings.Trim(`
  lawmatics-pp-cli intake stale --days 14
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := openLocalStore(cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			prospects, err := loadResourceObjects(db, "prospects")
			if err != nil {
				return err
			}
			if len(prospects) == 0 {
				return emptyResultMessage(cmd, flags, "prospects", "no prospects synced")
			}
			cutoff := time.Now().AddDate(0, 0, -days)
			var stale []map[string]any
			for _, p := range prospects {
				ts, _ := p["updated_at"].(string)
				t, err := time.Parse(time.RFC3339, ts)
				if err != nil {
					continue
				}
				if t.Before(cutoff) {
					stale = append(stale, map[string]any{
						"id":               p["id"],
						"name":             p["name"],
						"last_interaction": ts,
					})
				}
			}
			return printJSONFiltered(cmd.OutOrStdout(), stale, flags)
		},
	}
	cmd.Flags().IntVar(&days, "days", 14, "Days of inactivity to consider stale")
	return cmd
}

// ---------------- 8. who-touched ----------------

func newWhoTouchedCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "who-touched <contact>",
		Short:       "Chronological feed of interactions/notes/tasks/emails for contact",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: strings.Trim(`
  lawmatics-pp-cli who-touched contact_123
  lawmatics-pp-cli who-touched jane.smith@example.com --json
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			contactID := args[0]
			db, err := openLocalStore(cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			type event struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				When      string `json:"when"`
				Summary   string `json:"summary,omitempty"`
				Actor     string `json:"actor,omitempty"`
			}
			var events []event
			for _, rt := range []string{"interactions", "notes", "tasks", "emails"} {
				items, err := loadResourceObjects(db, rt)
				if err != nil {
					continue
				}
				for _, it := range items {
					cid := fmt.Sprintf("%v", it["contact_id"])
					if cid != contactID {
						continue
					}
					ts, _ := it["created_at"].(string)
					summary, _ := it["subject"].(string)
					if summary == "" {
						summary, _ = it["name"].(string)
					}
					actor := fmt.Sprintf("%v", it["user_id"])
					events = append(events, event{Type: rt, ID: fmt.Sprintf("%v", it["id"]), When: ts, Summary: summary, Actor: actor})
				}
			}
			sort.Slice(events, func(i, j int) bool { return events[i].When > events[j].When })
			payload := map[string]any{
				"contact": contactID,
				"events":  events,
				"count":   len(events),
			}
			if len(events) == 0 {
				payload["message"] = fmt.Sprintf("no synced events touch contact %q; run 'sync' first", contactID)
			}
			return printJSONFiltered(cmd.OutOrStdout(), payload, flags)
		},
	}
	return cmd
}

// ---------------- 9. revenue ----------------

func newRevenueCmd(flags *rootFlags) *cobra.Command {
	var by string
	var window string
	cmd := &cobra.Command{
		Use:         "revenue",
		Short:       "Sum invoiced + paid per group",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: strings.Trim(`
  lawmatics-pp-cli revenue --by source --window ytd --json
  lawmatics-pp-cli revenue --by practice-area --json
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = window
			if dryRunOK(flags) {
				return nil
			}
			db, err := openLocalStore(cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			invoices, err := loadResourceObjects(db, "invoices")
			if err != nil {
				return err
			}
			txs, _ := loadResourceObjects(db, "transactions")
			matters, _ := loadResourceObjects(db, "matters")
			matterIdx := map[string]map[string]any{}
			for _, m := range matters {
				matterIdx[fmt.Sprintf("%v", m["id"])] = m
			}
			groupKey := func(item map[string]any) string {
				mid := fmt.Sprintf("%v", item["matter_id"])
				if m, ok := matterIdx[mid]; ok {
					switch by {
					case "practice-area":
						if pa, ok := m["practice_area"].(string); ok && pa != "" {
							return pa
						}
					default:
						if s, ok := m["source"].(string); ok && s != "" {
							return s
						}
					}
				}
				return "(unknown)"
			}
			invoiced := map[string]float64{}
			paid := map[string]float64{}
			for _, inv := range invoices {
				amt, _ := inv["amount"].(float64)
				invoiced[groupKey(inv)] += amt
			}
			for _, tx := range txs {
				amt, _ := tx["amount"].(float64)
				paid[groupKey(tx)] += amt
			}
			type row struct {
				Group    string  `json:"group"`
				Invoiced float64 `json:"invoiced"`
				Paid     float64 `json:"paid"`
			}
			seen := map[string]bool{}
			var out []row
			for k, v := range invoiced {
				out = append(out, row{Group: k, Invoiced: v, Paid: paid[k]})
				seen[k] = true
			}
			for k, v := range paid {
				if !seen[k] {
					out = append(out, row{Group: k, Invoiced: 0, Paid: v})
				}
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Invoiced+out[i].Paid > out[j].Invoiced+out[j].Paid })
			if len(out) == 0 {
				return emptyResultMessage(cmd, flags, "revenue", "no invoices/transactions synced")
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&by, "by", "source", "Group by: source|practice-area")
	cmd.Flags().StringVar(&window, "window", "ytd", "Time window (e.g. ytd, 90d)")
	return cmd
}

// ---------------- 10. load ----------------

func newLoadCmd(flags *rootFlags) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:         "load",
		Short:       "Open matters + overdue tasks + billable hours MTD by attorney",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: strings.Trim(`
  lawmatics-pp-cli load --by attorney
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			_ = by
			db, err := openLocalStore(cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			matters, _ := loadResourceObjects(db, "matters")
			tasks, _ := loadResourceObjects(db, "tasks")
			time_entries, _ := loadResourceObjects(db, "time_entries")
			now := time.Now()
			monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			open := map[string]int{}
			overdue := map[string]int{}
			hours := map[string]float64{}
			for _, m := range matters {
				st, _ := m["status"].(string)
				if strings.EqualFold(st, "closed") {
					continue
				}
				a := fmt.Sprintf("%v", m["assignee_id"])
				open[a]++
			}
			for _, t := range tasks {
				if done, _ := t["completed"].(bool); done {
					continue
				}
				due, _ := t["due_date"].(string)
				if due == "" {
					continue
				}
				if parsed, err := time.Parse(time.RFC3339, due); err == nil && parsed.Before(now) {
					a := fmt.Sprintf("%v", t["assignee_id"])
					overdue[a]++
				}
			}
			for _, e := range time_entries {
				ts, _ := e["date"].(string)
				if t, err := time.Parse(time.RFC3339, ts); err == nil && t.After(monthStart) {
					h, _ := e["hours"].(float64)
					a := fmt.Sprintf("%v", e["user_id"])
					hours[a] += h
				}
			}
			type row struct {
				Attorney     string  `json:"attorney"`
				OpenMatters  int     `json:"open_matters"`
				OverdueTasks int     `json:"overdue_tasks"`
				HoursMTD     float64 `json:"hours_mtd"`
			}
			seen := map[string]bool{}
			var out []row
			collect := func(k string) {
				if seen[k] {
					return
				}
				seen[k] = true
				out = append(out, row{Attorney: k, OpenMatters: open[k], OverdueTasks: overdue[k], HoursMTD: hours[k]})
			}
			for k := range open {
				collect(k)
			}
			for k := range overdue {
				collect(k)
			}
			for k := range hours {
				collect(k)
			}
			if len(out) == 0 {
				return emptyResultMessage(cmd, flags, "load", "no matters/tasks/time_entries synced")
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&by, "by", "attorney", "Group by (currently only 'attorney')")
	return cmd
}

// ---------------- 11. pipeline / pipeline drift ----------------

func newPipelineXCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pipeline",
		Short: "Pipeline analytics (stage drift, etc.)",
		RunE:  parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newPipelineDriftCmd(flags))
	return cmd
}

func newPipelineDriftCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "drift",
		Short:       "Matters that skipped a stage or moved backward",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: strings.Trim(`
  lawmatics-pp-cli pipeline drift
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			// We don't sync a pipeline_stage_history table; emit honest no-data envelope.
			return emptyResultMessage(cmd, flags, "pipeline.drift",
				"no historical stage data: local store doesn't track pipeline_stage_history; use Lawmatics' web report or enable a stage-change webhook")
		},
	}
	return cmd
}

// ---------------- 12. explain ----------------

func newExplainCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "explain <matter-id>",
		Short:       "Markdown brief: notes + interactions + tasks for a matter",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: strings.Trim(`
  lawmatics-pp-cli explain matter_42
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			matterID := args[0]
			db, err := openLocalStore(cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			var b strings.Builder
			fmt.Fprintf(&b, "# Matter %s\n\n", matterID)
			if raw, err := db.Get("matters", matterID); err == nil {
				var m map[string]any
				_ = json.Unmarshal(raw, &m)
				if name, ok := m["name"].(string); ok {
					fmt.Fprintf(&b, "**Name:** %s\n\n", name)
				}
				if stage, ok := m["stage"].(string); ok {
					fmt.Fprintf(&b, "**Stage:** %s\n\n", stage)
				}
			} else {
				fmt.Fprintf(&b, "_matter not in local store; run 'sync' first_\n\n")
			}
			renderFiltered := func(label, rt, key string) {
				items, _ := loadResourceObjects(db, rt)
				fmt.Fprintf(&b, "## %s\n\n", label)
				count := 0
				for _, it := range items {
					if fmt.Sprintf("%v", it[key]) != matterID {
						continue
					}
					summary, _ := it["subject"].(string)
					if summary == "" {
						summary, _ = it["name"].(string)
					}
					if summary == "" {
						summary, _ = it["body"].(string)
					}
					fmt.Fprintf(&b, "- %s\n", summary)
					count++
				}
				if count == 0 {
					fmt.Fprintf(&b, "_(none)_\n")
				}
				fmt.Fprintln(&b)
			}
			renderFiltered("Notes", "notes", "matter_id")
			renderFiltered("Interactions", "interactions", "matter_id")
			renderFiltered("Tasks", "tasks", "matter_id")
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"matter_id": matterID, "markdown": b.String()}, flags)
			}
			fmt.Fprint(cmd.OutOrStdout(), b.String())
			return nil
		},
	}
	return cmd
}

// ---------------- 13. watch ----------------

func newWatchCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Poll a resource for changes since --since; optionally POST diffs to --webhook",
		Example: strings.Trim(`
  lawmatics-pp-cli watch contacts --since 1h
  lawmatics-pp-cli watch matters --since 30m --webhook https://example.com/hook
`, "\n"),
		RunE: parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newWatchContactsCmd(flags))
	cmd.AddCommand(newWatchMattersCmd(flags))
	cmd.AddCommand(newWatchTasksCmd(flags))
	cmd.AddCommand(newWatchProspectsCmd(flags))
	cmd.AddCommand(newWatchEventsCmd(flags))
	cmd.AddCommand(newWatchNotesCmd(flags))
	return cmd
}

func newWatchContactsCmd(flags *rootFlags) *cobra.Command {
	var since string
	var webhook string
	cmd := &cobra.Command{
		Use:   "contacts",
		Short: "Poll contacts for changes since --since",
		RunE:  watchRunE(flags, "contacts", &since, &webhook),
	}
	cmd.Flags().StringVar(&since, "since", "5m", "Window to look back (e.g. 5m, 1h)")
	cmd.Flags().StringVar(&webhook, "webhook", "", "Optional webhook URL to POST changes")
	return cmd
}

func newWatchMattersCmd(flags *rootFlags) *cobra.Command {
	var since string
	var webhook string
	cmd := &cobra.Command{
		Use:   "matters",
		Short: "Poll matters for changes since --since",
		RunE:  watchRunE(flags, "matters", &since, &webhook),
	}
	cmd.Flags().StringVar(&since, "since", "5m", "Window to look back (e.g. 5m, 1h)")
	cmd.Flags().StringVar(&webhook, "webhook", "", "Optional webhook URL to POST changes")
	return cmd
}

func newWatchTasksCmd(flags *rootFlags) *cobra.Command {
	var since string
	var webhook string
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "Poll tasks for changes since --since",
		RunE:  watchRunE(flags, "tasks", &since, &webhook),
	}
	cmd.Flags().StringVar(&since, "since", "5m", "Window to look back (e.g. 5m, 1h)")
	cmd.Flags().StringVar(&webhook, "webhook", "", "Optional webhook URL to POST changes")
	return cmd
}

func newWatchProspectsCmd(flags *rootFlags) *cobra.Command {
	var since string
	var webhook string
	cmd := &cobra.Command{
		Use:   "prospects",
		Short: "Poll prospects for changes since --since",
		RunE:  watchRunE(flags, "prospects", &since, &webhook),
	}
	cmd.Flags().StringVar(&since, "since", "5m", "Window to look back (e.g. 5m, 1h)")
	cmd.Flags().StringVar(&webhook, "webhook", "", "Optional webhook URL to POST changes")
	return cmd
}

func newWatchEventsCmd(flags *rootFlags) *cobra.Command {
	var since string
	var webhook string
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Poll events for changes since --since",
		RunE:  watchRunE(flags, "events", &since, &webhook),
	}
	cmd.Flags().StringVar(&since, "since", "5m", "Window to look back (e.g. 5m, 1h)")
	cmd.Flags().StringVar(&webhook, "webhook", "", "Optional webhook URL to POST changes")
	return cmd
}

func newWatchNotesCmd(flags *rootFlags) *cobra.Command {
	var since string
	var webhook string
	cmd := &cobra.Command{
		Use:   "notes",
		Short: "Poll notes for changes since --since",
		RunE:  watchRunE(flags, "notes", &since, &webhook),
	}
	cmd.Flags().StringVar(&since, "since", "5m", "Window to look back (e.g. 5m, 1h)")
	cmd.Flags().StringVar(&webhook, "webhook", "", "Optional webhook URL to POST changes")
	return cmd
}

func watchRunE(flags *rootFlags, resource string, since, webhook *string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if cliutil.IsVerifyEnv() {
			fmt.Fprintf(cmd.OutOrStdout(), "verify-env: would poll %s since %s (webhook=%q)\n", resource, *since, *webhook)
			return nil
		}
		if dryRunOK(flags) {
			fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would poll %s since %s (webhook=%q)\n", resource, *since, *webhook)
			return nil
		}
		c, err := flags.newClient()
		if err != nil {
			return err
		}
		params := map[string]string{}
		if *since != "" {
			if d, err := parseDuration(*since); err == nil {
				params["updated_since"] = time.Now().Add(-d).UTC().Format(time.RFC3339)
			}
		}
		data, err := c.Get("/v1/"+resource, params)
		if err != nil {
			return classifyAPIError(err, flags)
		}
		if *webhook != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: --webhook delivery not implemented in single-shot poll; printing diffs instead\n")
		}
		return printOutputWithFlags(cmd.OutOrStdout(), data, flags)
	}
}

func newWatchResourceCmd(flags *rootFlags, resource string) *cobra.Command {
	var since string
	var webhook string
	cmd := &cobra.Command{
		Use:   resource,
		Short: fmt.Sprintf("Poll %s for changes since --since", resource),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cliutil.IsVerifyEnv() {
				fmt.Fprintf(cmd.OutOrStdout(), "verify-env: would poll %s since %s (webhook=%q)\n", resource, since, webhook)
				return nil
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would poll %s since %s (webhook=%q)\n", resource, since, webhook)
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			params := map[string]string{}
			if since != "" {
				if d, err := parseDuration(since); err == nil {
					params["updated_since"] = time.Now().Add(-d).UTC().Format(time.RFC3339)
				}
			}
			data, err := c.Get("/v1/"+resource, params)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			if webhook != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: --webhook delivery not implemented in single-shot poll; printing diffs instead\n")
			}
			return printOutputWithFlags(cmd.OutOrStdout(), data, flags)
		},
	}
	cmd.Flags().StringVar(&since, "since", "5m", "Window to look back (e.g. 5m, 1h)")
	cmd.Flags().StringVar(&webhook, "webhook", "", "Optional webhook URL to POST changes")
	return cmd
}

// ---------------- 14. whoami ----------------

func newWhoamiCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "whoami",
		Short:       "Token introspection (GET /v1/me)",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: strings.Trim(`
  lawmatics-pp-cli whoami
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			data, err := c.Get("/v1/me", nil)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			return printOutputWithFlags(cmd.OutOrStdout(), data, flags)
		},
	}
	return cmd
}

// ---------------- 15. health (doctor-deep) ----------------

func newHealthCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "health",
		Short:       "Deep store health checks: orphan tasks, contacts w/o email, dead stages, unused CFs, dormant users",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: strings.Trim(`
  lawmatics-pp-cli health
  lawmatics-pp-cli health --json
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := openLocalStore(cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			contacts, _ := loadResourceObjects(db, "contacts")
			tasks, _ := loadResourceObjects(db, "tasks")
			matters, _ := loadResourceObjects(db, "matters")
			users, _ := loadResourceObjects(db, "users")
			stages, _ := loadResourceObjects(db, "stages")
			cfs, _ := loadResourceObjects(db, "custom_fields")

			matterIDs := map[string]bool{}
			for _, m := range matters {
				matterIDs[fmt.Sprintf("%v", m["id"])] = true
			}
			var orphanTasks []string
			for _, t := range tasks {
				mid := fmt.Sprintf("%v", t["matter_id"])
				if mid != "" && mid != "<nil>" && !matterIDs[mid] {
					orphanTasks = append(orphanTasks, fmt.Sprintf("%v", t["id"]))
				}
			}
			var noEmailContacts []string
			for _, c := range contacts {
				if e, _ := c["email"].(string); e == "" {
					noEmailContacts = append(noEmailContacts, fmt.Sprintf("%v", c["id"]))
				}
			}
			usedStages := map[string]bool{}
			for _, m := range matters {
				if s, ok := m["stage"].(string); ok {
					usedStages[s] = true
				}
			}
			var deadStages []string
			for _, st := range stages {
				name, _ := st["name"].(string)
				if name != "" && !usedStages[name] {
					deadStages = append(deadStages, name)
				}
			}
			activeUsers := map[string]bool{}
			for _, t := range tasks {
				activeUsers[fmt.Sprintf("%v", t["assignee_id"])] = true
			}
			var dormantUsers []string
			for _, u := range users {
				id := fmt.Sprintf("%v", u["id"])
				if !activeUsers[id] {
					dormantUsers = append(dormantUsers, id)
				}
			}
			var unusedCFs []string
			for _, cf := range cfs {
				name, _ := cf["name"].(string)
				if name != "" {
					unusedCFs = append(unusedCFs, name) // best-effort; we don't index values
				}
			}
			report := map[string]any{
				"orphan_tasks":      orphanTasks,
				"contacts_no_email": noEmailContacts,
				"dead_stages":       deadStages,
				"dormant_users":     dormantUsers,
				"custom_fields_seen": unusedCFs,
				"counts": map[string]int{
					"orphan_tasks":      len(orphanTasks),
					"contacts_no_email": len(noEmailContacts),
					"dead_stages":       len(deadStages),
					"dormant_users":     len(dormantUsers),
				},
			}
			return printJSONFiltered(cmd.OutOrStdout(), report, flags)
		},
	}
	return cmd
}
