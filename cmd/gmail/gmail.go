package gmail

import (
	"github.com/spf13/cobra"
)

// GmailCmd represents the gmail command
var GmailCmd = &cobra.Command{
	Use:   "gmail",
	Short: "Gmail API operations for email analysis and management",
	Long: `Gmail command provides tools for analyzing and managing your Gmail inbox:

- Analyze: Get statistics about your emails (senders, sizes, dates, attachments)
- Delete: Batch delete emails with interactive filters and preview

Examples:
  mastercrab gmail analyze --analysis-type sender-stats
  mastercrab gmail delete --filter-senders "spam@example.com" --dry-run`,
	Run: func(cmd *cobra.Command, args []string) {
		// Show help if no subcommand is specified
		cmd.Help()
	},
}

func init() {
	// Subcommands will be added here
	GmailCmd.AddCommand(analyzeCmd)
}
