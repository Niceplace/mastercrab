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
	Short: "Generate a daily work summary from Linear and GitHub activity",
	Long: `Generate a daily work summary by fetching your recent activity from Linear
(assigned issues) and GitHub (commits, PRs, reviews, issues).

By default, this command looks back 24 hours, but you can customize the time period
using the --hours flag.

Example:
  mastercrab daily              # Summary for the last 24 hours
  mastercrab daily --hours 48   # Summary for the last 48 hours`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("📅 Daily Work Summary Generator")
		fmt.Println(strings.Repeat("═", 80))

		// Get the lookback hours from flag, defaulting to 24 hours
		lookbackHours, _ := cmd.Flags().GetInt("hours")

		// Calculate time period
		until := time.Now()
		since := until.Add(-time.Duration(lookbackHours) * time.Hour)

		// Convert to Linear date filter format (ISO 8601 duration)
		linearDateFilter := fmt.Sprintf("-P%dD", lookbackHours/24)
		if lookbackHours < 24 {
			linearDateFilter = "-P1D" // Linear doesn't support hourly granularity well
		}

		// Create HTTP client
		client := &http.Client{}

		// Fetch GitHub activity
		fmt.Printf("\n🔍 Fetching GitHub activity from the last %d hours...\n", lookbackHours)
		githubActivity, err := GetViewerActivity(client, since, until, viper.GetViper())
		if err != nil {
			fmt.Printf("⚠️  Failed to fetch GitHub activity: %s\n", err)
			// Continue with Linear even if GitHub fails
		} else {
			fmt.Printf("✅ Found GitHub activity: %d commits, %d PRs, %d reviews, %d issues\n",
				githubActivity.TotalCommits,
				githubActivity.TotalPullRequests,
				githubActivity.TotalReviews,
				githubActivity.TotalIssues)
		}

		// Fetch Linear assigned issues
		fmt.Printf("\n🔍 Fetching Linear issues updated in the last %d hours...\n", lookbackHours)
		linearViewer, err := GetViewerAssignedIssues(client, linearDateFilter, viper.GetViper())
		if err != nil {
			fmt.Printf("❌ Failed to fetch assigned issues: %s\n", err)
			return
		}

		// Display the results
		issues := linearViewer.Viewer.AssignedIssues.Edges
		if len(issues) == 0 && githubActivity.TotalCommits == 0 &&
			githubActivity.TotalPullRequests == 0 && githubActivity.TotalReviews == 0 &&
			githubActivity.TotalIssues == 0 {
			fmt.Println("✅ No activity found in the specified time period")
			return
		}

		fmt.Printf("✅ Found %d Linear issue(s) updated in the last %d hours\n", len(issues), lookbackHours)

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

		// Generate the markdown summary
		summaryFilename := fmt.Sprintf("daily-summary-%s.md", time.Now().Format("2006-01-02"))
		fmt.Printf("\n📄 Generating summary file: %s\n", summaryFilename)

		err = GenerateSimplifiedMarkdownSummary(issuesWithNotes, githubActivity, summaryFilename)
		if err != nil {
			fmt.Printf("❌ Failed to generate summary: %s\n", err)
			return
		}

		fmt.Printf("\n✨ Summary generated successfully!\n")
		fmt.Printf("📂 File: %s\n", summaryFilename)
	},
}

func init() {
	// Add flag for configurable time period (in hours)
	DailyCmd.Flags().IntP("hours", "H", 24, "Number of hours to look back for activity (default: 24)")
}
