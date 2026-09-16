// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"
)

func newFormsListCmd(flags *rootFlags) *cobra.Command {
	var fetchAll bool
	var raw bool
	var filter string
	var params []string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List custom forms",
		Long: `List the account's custom forms as id/name pairs.

--filter matches locally on the form name because the live list endpoint
ignores filter parameters.`,
		Example: `  lawmatics-pp-cli forms list --all
  lawmatics-pp-cli forms list --filter 0450 --agent`,
		Annotations: map[string]string{"pp:endpoint": "forms.list", "pp:method": "GET", "pp:path": "/forms", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := formsLiveOnly(flags); err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			queryParams, err := parseKeyValueFlags(params, "--param")
			if err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				_, _, err := listForms(c, queryParams, false)
				return err
			}

			forms, rawItems, err := listForms(c, queryParams, fetchAll)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			if raw {
				keep := rawItems
				if filter != "" {
					keep = nil
					for i, form := range forms {
						if matchesFormFilter(form, filter) && i < len(rawItems) {
							keep = append(keep, rawItems[i])
						}
					}
				}
				data, err := json.Marshal(keep)
				if err != nil {
					return err
				}
				return printFormsPayload(cmd, flags, data)
			}

			matches := make([]formSummary, 0, len(forms))
			for _, form := range forms {
				if filter == "" || matchesFormFilter(form, filter) {
					matches = append(matches, form)
				}
			}
			data, err := json.Marshal(matches)
			if err != nil {
				return err
			}
			return printFormsPayload(cmd, flags, data)
		},
	}
	cmd.Flags().BoolVar(&fetchAll, "all", false, "Follow pagination and return every form")
	cmd.Flags().BoolVar(&raw, "raw", false, "Return the API objects verbatim instead of id/name summaries")
	cmd.Flags().StringVar(&filter, "filter", "", "Keep only forms whose name or id contains this text (case-insensitive)")
	cmd.Flags().StringArrayVar(&params, "param", nil, "Extra query parameter as key=value (repeatable)")
	return cmd
}

func matchesFormFilter(form formSummary, filter string) bool {
	needle := strings.ToLower(strings.TrimSpace(filter))
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(form.Name), needle) ||
		strings.Contains(strings.ToLower(form.ID), needle) ||
		strings.Contains(normalizeFormText(form.Name), normalizeFormText(needle))
}

// parseKeyValueFlags parses repeatable key=value flags into a map, naming the
// offending flag in the error so a malformed entry is easy to find.
func parseKeyValueFlags(entries []string, flagName string) (map[string]string, error) {
	out := map[string]string{}
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, errInvalidAssignment(flagName, entry)
		}
		out[key] = value
	}
	return out, nil
}
