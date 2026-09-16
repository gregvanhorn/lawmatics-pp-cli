// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

func newFormsEntriesCmd(flags *rootFlags) *cobra.Command {
	var fetchAll bool
	var params []string

	cmd := &cobra.Command{
		Use:   "entries <form>",
		Short: "List submissions recorded for a custom form",
		Long: `List the entries (submissions) recorded for a custom form.

Only the first page is returned unless --all is passed; a truncated result
emits a warning on stderr.`,
		Example: `  lawmatics-pp-cli forms entries "0450" --agent
  lawmatics-pp-cli forms entries 38d8ec11-87cd-41d6-b983-f025cfec8926 --all`,
		Annotations: map[string]string{"pp:endpoint": "forms.entries", "pp:method": "GET", "pp:path": "/forms/{id}/entries", "mcp:read-only": "true"},
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
			queryParams, err := parseKeyValueFlags(params, "--param")
			if err != nil {
				return usageErr(err)
			}
			form, err := resolveFormTarget(c, flags, args[0])
			if err != nil {
				return err
			}
			items, _, err := fetchFormCollection(c, formPath(form.ID, "entries"), queryParams, fetchAll)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			if dryRunOK(flags) {
				return nil
			}
			data, err := json.Marshal(items)
			if err != nil {
				return err
			}
			return printFormsPayload(cmd, flags, data)
		},
	}
	cmd.Flags().BoolVar(&fetchAll, "all", false, "Follow pagination and return every entry")
	cmd.Flags().StringArrayVar(&params, "param", nil, "Extra query parameter as key=value (repeatable)")
	return cmd
}
