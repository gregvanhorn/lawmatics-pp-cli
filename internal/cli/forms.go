// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"
)

// newFormsCmd wires the custom-forms command group. Lawmatics exposes
// /v1/forms in the live API but the generated command tree has no forms
// interface, so this group is maintained here; see
// .printing-press-patches.json.
//
// Form *definitions* are read-only over the API: POST /v1/forms returns 404
// and forms can only be authored in the Lawmatics UI (Assets → Custom
// Forms). This group therefore covers discovery and submission only.
func newFormsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "forms",
		Short: "Discover and fill out Lawmatics custom forms",
		Long: `Discover and fill out Lawmatics custom forms.

  forms list                    every form on the account
  forms get <form>              the form definition (fields=all by default)
  forms fields <form>           just the fillable fields, flattened
  forms entries <form>          submissions already recorded for the form
  forms submit <form>           create a new entry

<form> accepts a uuid, an exact name, a form number ("450", "Form 450"), or
an unambiguous part of the name. Ambiguous references fail rather than guess.

Creating, updating, or deleting form definitions is not available: the API
rejects POST /v1/forms, so forms must be authored in the Lawmatics UI.`,
		Example: `  lawmatics-pp-cli forms list --all --agent
  lawmatics-pp-cli forms fields "0450" --required-only --agent
  lawmatics-pp-cli forms fields "0450" --template > filled.json
  lawmatics-pp-cli forms submit "0450" --json-file filled.json --agent`,
		Annotations: map[string]string{"pp:interface": "forms"},
		RunE:        parentNoSubcommandRunE(flags),
	}

	cmd.AddCommand(newFormsListCmd(flags))
	cmd.AddCommand(newFormsGetCmd(flags))
	cmd.AddCommand(newFormsFieldsCmd(flags))
	cmd.AddCommand(newFormsEntriesCmd(flags))
	cmd.AddCommand(newFormsSubmitCmd(flags))
	return cmd
}
