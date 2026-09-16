// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

func newFormsFieldsCmd(flags *rootFlags) *cobra.Command {
	var filter string
	var requiredOnly bool
	var allComponents bool
	var template bool

	cmd := &cobra.Command{
		Use:   "fields <form>",
		Short: "List the fillable fields of a custom form",
		Long: `Flatten a custom form's rows into the fields a submission can carry.

Each row reports the id to use as a submit key, the label shown to the
client, the field type, and whether the field is required. Layout
components (page dividers, instruction blocks) are omitted unless
--all-components is set, and a component repeated across conditional rows
is reported once.

--template prints a submit-ready JSON body keyed by field id, pre-filled
with the form's own defaults:

  lawmatics-pp-cli forms fields "0450" --template > filled.json
  # edit filled.json, then
  lawmatics-pp-cli forms submit "0450" --json-file filled.json`,
		Example: `  lawmatics-pp-cli forms fields "0450" --agent
  lawmatics-pp-cli forms fields "0450" --required-only
  lawmatics-pp-cli forms fields "0450" --filter "spouse"
  lawmatics-pp-cli forms fields "0450" --template`,
		Annotations: map[string]string{"pp:endpoint": "forms.fields", "pp:method": "GET", "pp:path": "/forms/{id}", "mcp:read-only": "true"},
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
			data, err := c.GetWithHeaders(formPath(form.ID), map[string]string{"fields": "all"}, nil)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			if dryRunOK(flags) {
				return nil
			}
			fields, err := flattenFormFields(data, allComponents)
			if err != nil {
				return err
			}
			fields = filterFormFields(fields, filter, requiredOnly)

			if template {
				return flags.printJSON(cmd, buildFieldTemplate(fields))
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				if len(fields) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "no matching fields")
					return nil
				}
				rows := make([][]string, 0, len(fields))
				for _, field := range fields {
					rows = append(rows, []string{
						field.ID,
						truncate(field.Label, 60),
						field.FieldType,
						strconv.FormatBool(field.Required),
						field.ComponentType,
						field.SimplifiedID,
					})
				}
				return flags.printTable(cmd,
					[]string{"ID", "LABEL", "FIELD_TYPE", "REQUIRED", "COMPONENT_TYPE", "SIMPLIFIED_ID"}, rows)
			}
			return flags.printJSON(cmd, fields)
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "", "Keep only fields whose label, id, or type contains this text (case-insensitive)")
	cmd.Flags().BoolVar(&requiredOnly, "required-only", false, "Keep only required fields")
	cmd.Flags().BoolVar(&allComponents, "all-components", false, "Include layout components (page dividers, instructions) as well")
	cmd.Flags().BoolVar(&template, "template", false, "Print a submit-ready JSON body keyed by field id")
	return cmd
}
