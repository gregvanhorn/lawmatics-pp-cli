// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// simpleResource describes a read-only API resource that needs nothing
// beyond list and get. The generator emits one file per endpoint; these
// hand-maintained resources share one implementation instead, because
// duplicating the full endpoint template for a stub is all cost and no
// signal.
type simpleResource struct {
	Name     string // plural resource name, also the local store resource type
	Path     string // collection path, e.g. "/campaigns"
	Singular string // singular noun used in help text
}

func newSimpleListCmd(flags *rootFlags, resource simpleResource) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   fmt.Sprintf("List %s", resource.Name),
		Example: fmt.Sprintf("  lawmatics-pp-cli %s list", resource.Name),
		Annotations: map[string]string{
			"pp:endpoint": resource.Name + ".list", "pp:method": "GET",
			"pp:path": resource.Path, "mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			data, prov, err := resolveRead(cmd.Context(), c, flags, resource.Name, true, resource.Path, map[string]string{}, nil)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			return printResourcePayload(cmd, flags, data, prov)
		},
	}
}

func newSimpleGetCmd(flags *rootFlags, resource simpleResource) *cobra.Command {
	return &cobra.Command{
		Use:     "get <id>",
		Short:   fmt.Sprintf("Get a %s", resource.Singular),
		Example: fmt.Sprintf("  lawmatics-pp-cli %s get 123", resource.Name),
		Annotations: map[string]string{
			"pp:endpoint": resource.Name + ".get", "pp:method": "GET",
			"pp:path": resource.Path + "/{id}", "mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			path := replacePathParam(resource.Path+"/{id}", "id", args[0])
			data, prov, err := resolveRead(cmd.Context(), c, flags, resource.Name, false, path, map[string]string{}, nil)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			return printResourcePayload(cmd, flags, data, prov)
		},
	}
}

// printResourcePayload renders a read response exactly like the generated
// endpoint commands: provenance-wrapped JSON for agents and piped consumers,
// a table for humans at a terminal.
func printResourcePayload(cmd *cobra.Command, flags *rootFlags, data json.RawMessage, prov DataProvenance) error {
	data = extractResponseData(data)
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		var countItems []json.RawMessage
		if json.Unmarshal(data, &countItems) != nil {
			countItems = []json.RawMessage{data}
		}
		printProvenance(cmd, len(countItems), prov)
	}
	if flags.asJSON || (!isTerminal(cmd.OutOrStdout()) && !flags.csv && !flags.quiet && !flags.plain) {
		filtered := data
		if flags.selectFields != "" {
			filtered = filterFields(filtered, flags.selectFields)
		} else if flags.compact {
			filtered = compactFields(filtered)
		}
		wrapped, err := wrapWithProvenance(filtered, prov)
		if err != nil {
			return err
		}
		return printOutput(cmd.OutOrStdout(), wrapped, true)
	}
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		var items []map[string]any
		if json.Unmarshal(data, &items) == nil && len(items) > 0 {
			return printAutoTable(cmd.OutOrStdout(), items)
		}
	}
	return printOutputWithFlags(cmd.OutOrStdout(), data, flags)
}
