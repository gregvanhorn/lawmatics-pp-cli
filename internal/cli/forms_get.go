// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newFormsGetCmd(flags *rootFlags) *cobra.Command {
	var fields string

	cmd := &cobra.Command{
		Use:   "get <form>",
		Short: "Get a custom form definition",
		Long: `Get a custom form definition.

The request sends fields=all by default: without it Lawmatics returns only
the form's name and timestamps, with no rows or components. Pass
--fields rows for the layout alone, or --fields "" for the bare metadata.`,
		Example: `  lawmatics-pp-cli forms get 38d8ec11-87cd-41d6-b983-f025cfec8926 --agent
  lawmatics-pp-cli forms get "0450" --fields rows`,
		Annotations: map[string]string{"pp:endpoint": "forms.get", "pp:method": "GET", "pp:path": "/forms/{id}", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if err := formsLiveOnly(flags); err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			form, err := resolveFormTarget(c, flags, args[0])
			if err != nil {
				return err
			}
			data, err := c.GetWithHeaders(formPath(form.ID), formFieldsParams(fields), nil)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			if dryRunOK(flags) {
				return nil
			}
			return printFormsPayload(cmd, flags, data)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "all", `Field expansion sent to the API ("all", "rows", or "" for none)`)
	return cmd
}

// formFieldsParams builds the query for the fields expansion, omitting the
// parameter entirely when the caller asked for no expansion.
func formFieldsParams(fields string) map[string]string {
	fields = strings.TrimSpace(fields)
	if fields == "" || strings.EqualFold(fields, "none") {
		return map[string]string{}
	}
	return map[string]string{"fields": fields}
}
