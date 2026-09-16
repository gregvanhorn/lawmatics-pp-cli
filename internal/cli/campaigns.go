// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"
)

// newCampaignsCmd mirrors the generated resource groups for /v1/campaigns,
// which the live API serves but the generated command tree omits.
func newCampaignsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "campaigns",
		Short:  "Operations on campaigns",
		Hidden: true,
		RunE:   parentNoSubcommandRunE(flags),
	}

	cmd.AddCommand(newSimpleListCmd(flags, simpleResource{
		Name: "campaigns", Path: "/campaigns", Singular: "campaign",
	}))
	cmd.AddCommand(newSimpleGetCmd(flags, simpleResource{
		Name: "campaigns", Path: "/campaigns", Singular: "campaign",
	}))
	return cmd
}

// newSourcesCmd mirrors the generated resource groups for /v1/sources.
func newSourcesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "sources",
		Short:  "Operations on sources",
		Hidden: true,
		RunE:   parentNoSubcommandRunE(flags),
	}

	cmd.AddCommand(newSimpleListCmd(flags, simpleResource{
		Name: "sources", Path: "/sources", Singular: "source",
	}))
	cmd.AddCommand(newSimpleGetCmd(flags, simpleResource{
		Name: "sources", Path: "/sources", Singular: "source",
	}))
	return cmd
}
