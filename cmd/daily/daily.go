/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package daily

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// dailyCmd represents the daily command
var DailyCmd = &cobra.Command{
	Use:   "daily",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("📅 Daily Work Summary Generator")
		fmt.Println(strings.Repeat("═", 80))

		// Use -P1D for last 24 hours by default (can be overridden by flag in the future)
		dateFilter := "-P1D"

		// Create HTTP client
		client := &http.Client{}

		// Fetch viewer's assigned issues
		fmt.Println("\n🔍 Fetching issues updated in the last 24 hours...")
		linearViewer, err := GetViewerAssignedIssues(client, dateFilter, viper.GetViper())
		if err != nil {
			fmt.Printf("❌ Failed to fetch assigned issues: %s\n", err)
			return
		}

		// Display the results
		issues := linearViewer.Viewer.AssignedIssues.Edges
		if len(issues) == 0 {
			fmt.Println("✅ No issues updated in the last 24 hours")
			return
		}

		fmt.Printf("\n✅ Found %d issue(s) updated in the last 24 hours\n", len(issues))

		// Interactive flow: fetch details and prompt for notes
		var issuesWithNotes []IssueWithNotes

		for i, edge := range issues {
			fmt.Printf("\n\n📦 Processing issue %d of %d...\n", i+1, len(issues))

			// Fetch detailed information for this issue
			details, err := GetIssueDetails(client, edge.Node.ID, viper.GetViper())
			if err != nil {
				fmt.Printf("⚠️  Failed to fetch details for issue %s: %s\n", edge.Node.ID, err)
				fmt.Println("   Skipping this issue...")
				continue
			}

			// Display the issue details
			DisplayIssueDetails(details)

			// Prompt for user notes
			notes, workedOnIt, err := PromptForNotes(details)
			if err != nil {
				// User skipped, continue to next issue
				fmt.Println("⏭️  Skipped")
				continue
			}

			if workedOnIt {
				issuesWithNotes = append(issuesWithNotes, IssueWithNotes{
					Details:   details,
					UserNotes: notes,
				})
				fmt.Println("✅ Notes recorded!")
			} else {
				fmt.Println("➖ No work recorded for this issue")
			}
		}

		// Generate summary if any issues were worked on
		if len(issuesWithNotes) == 0 {
			fmt.Println("\n📝 No work recorded for today. No summary generated.")
			return
		}

		// Generate the markdown summary
		summaryFilename := fmt.Sprintf("daily-summary-%s.md", time.Now().Format("2006-01-02"))
		fmt.Printf("\n📄 Generating summary file: %s\n", summaryFilename)

		err = GenerateMarkdownSummary(issuesWithNotes, summaryFilename)
		if err != nil {
			fmt.Printf("❌ Failed to generate summary: %s\n", err)
			return
		}

		fmt.Printf("\n✨ Summary generated successfully!\n")
		fmt.Printf("📂 File: %s\n", summaryFilename)
		fmt.Printf("📊 Total issues documented: %d\n", len(issuesWithNotes))
	},
}

func init() {

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// dailyCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	DailyCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
