package daily

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
)

// IssueWithNotes stores an issue's details along with user notes
type IssueWithNotes struct {
	Details   LinearIssueDetails
	UserNotes string
}

// DisplayIssueDetails shows a formatted view of the issue details
func DisplayIssueDetails(issue LinearIssueDetails) {
	fmt.Println("\n" + strings.Repeat("═", 80))
	fmt.Printf("📋 %s: %s\n", issue.Identifier, issue.Title)
	fmt.Println(strings.Repeat("═", 80))

	fmt.Printf("\n🔗 URL: %s\n", issue.URL)
	fmt.Printf("📊 State: %s (%s)\n", issue.State.Name, issue.State.Type)
	fmt.Printf("⚡ Priority: %s\n", issue.PriorityLabel)
	fmt.Printf("🕛 Created at: %s\n", issue.CreatedAt)
	fmt.Printf("🕞 Updated at: %s\n", issue.UpdatedAt)

	if issue.Assignee.Name != "" {
		fmt.Printf("👤 Assignee: %s\n", issue.Assignee.Name)
	}

	// Display labels
	if len(issue.Labels.Nodes) > 0 {
		fmt.Printf("🏷️  Labels: ")
		labelNames := make([]string, len(issue.Labels.Nodes))
		for i, label := range issue.Labels.Nodes {
			labelNames[i] = label.Name
		}
		fmt.Println(strings.Join(labelNames, ", "))
	}

	// Display full description with markdown rendering
	if issue.Description != "" {
		fmt.Println("\n📝 Description:")
		fmt.Println(strings.Repeat("─", 80))

		// Render markdown in terminal using glamour
		renderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)

		if err != nil {
			// Fallback to plain text if glamour fails
			fmt.Println(issue.Description)
		} else {
			rendered, err := renderer.Render(issue.Description)
			if err != nil {
				// Fallback to plain text
				fmt.Println(issue.Description)
			} else {
				fmt.Print(rendered)
			}
		}
		fmt.Println(strings.Repeat("─", 80))
	}

	// Display recent comments
	if len(issue.Comments.Nodes) > 0 {
		fmt.Printf("\n💬 Comments (%d total):\n", len(issue.Comments.Nodes))
		// Show only last 10 comments
		start := 0
		if len(issue.Comments.Nodes) > 10 {
			start = len(issue.Comments.Nodes) - 10
			fmt.Printf("   (Showing last 10 comments)\n")
		}
		for i := start; i < len(issue.Comments.Nodes); i++ {
			comment := issue.Comments.Nodes[i]
			fmt.Printf("\n  💭 %s: %s \n", comment.User.Name, comment.UpdatedAt)
			// Render comment body as markdown too
			renderer, err := glamour.NewTermRenderer(
				glamour.WithAutoStyle(),
				glamour.WithWordWrap(76),
			)
			if err == nil {
				rendered, err := renderer.Render(comment.Body)
				if err == nil {
					// Indent the rendered comment
					lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
					for _, line := range lines {
						fmt.Printf("     %s\n", line)
					}
				} else {
					fmt.Printf("     %s\n", comment.Body)
				}
			} else {
				fmt.Printf("     %s\n", comment.Body)
			}
		}
	}

	fmt.Println()
}

// PromptForNotes prompts the user to add notes about their work on this issue
func PromptForNotes(issue LinearIssueDetails) (string, bool, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n❓ Did you work on this issue today? (y/n/skip)")
	fmt.Print("▶ ")

	response, err := reader.ReadString('\n')
	if err != nil {
		return "", false, err
	}

	response = strings.TrimSpace(strings.ToLower(response))

	if response == "n" || response == "no" {
		return "", false, nil
	}

	if response == "skip" || response == "s" {
		return "", false, fmt.Errorf("user skipped")
	}

	fmt.Println("\n✍️  Please describe what you did on this issue:")
	fmt.Println("   (Press Enter on an empty line to finish)")
	fmt.Print("▶ ")

	var notes strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", false, err
		}

		line = strings.TrimRight(line, "\n\r")

		// Empty line signals end of input
		if line == "" {
			break
		}

		if notes.Len() > 0 {
			notes.WriteString("\n")
		}
		notes.WriteString(line)
		fmt.Print("▶ ")
	}

	return notes.String(), true, nil
}

// GenerateMarkdownSummary creates a markdown summary of the daily work
func GenerateMarkdownSummary(issuesWithNotes []IssueWithNotes, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create summary file: %w", err)
	}
	defer file.Close()

	// Write header
	today := time.Now().Format("Monday, January 2, 2006")
	fmt.Fprintf(file, "# Daily Work Summary - %s\n\n", today)

	if len(issuesWithNotes) == 0 {
		fmt.Fprintln(file, "No work recorded for today.")
		return nil
	}

	fmt.Fprintf(file, "## Summary\n\n")
	fmt.Fprintf(file, "Worked on **%d issue(s)** today.\n\n", len(issuesWithNotes))

	// Write each issue
	for i, issueNote := range issuesWithNotes {
		issue := issueNote.Details

		fmt.Fprintf(file, "## %d. [%s] %s\n\n", i+1, issue.Identifier, issue.Title)
		fmt.Fprintf(file, "**Status:** %s | **Priority:** %s\n\n", issue.State.Name, issue.PriorityLabel)
		fmt.Fprintf(file, "🔗 [View in Linear](%s)\n\n", issue.URL)

		// Add labels
		if len(issue.Labels.Nodes) > 0 {
			fmt.Fprintf(file, "**Labels:** ")
			labelNames := make([]string, len(issue.Labels.Nodes))
			for j, label := range issue.Labels.Nodes {
				labelNames[j] = "`" + label.Name + "`"
			}
			fmt.Fprintf(file, "%s\n\n", strings.Join(labelNames, ", "))
		}

		// Add user notes
		if issueNote.UserNotes != "" {
			fmt.Fprintf(file, "### Work Completed\n\n")
			fmt.Fprintf(file, "%s\n\n", issueNote.UserNotes)
		}

		fmt.Fprintf(file, "---\n\n")
	}

	// Footer
	fmt.Fprintf(file, "\n*Generated at %s*\n", time.Now().Format("2006-01-02 15:04:05"))

	return nil
}

// GenerateSimplifiedMarkdownSummary creates a simplified markdown summary with GitHub activity
func GenerateSimplifiedMarkdownSummary(issuesWithNotes []IssueWithNotes, githubActivity GitHubActivity, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create summary file: %w", err)
	}
	defer file.Close()

	// Write header
	today := time.Now().Format("Monday, January 2, 2006")
	fmt.Fprintf(file, "# Daily Work Summary - %s\n\n", today)

	// GitHub Activity Section
	if githubActivity.TotalCommits > 0 || githubActivity.TotalPullRequests > 0 ||
		githubActivity.TotalReviews > 0 || githubActivity.TotalIssues > 0 {
		fmt.Fprintf(file, "## GitHub Activity\n\n")

		// Pull Requests Created
		if len(githubActivity.PullRequestsCreated) > 0 {
			for _, pr := range githubActivity.PullRequestsCreated {
				fmt.Fprintf(file, "- [%s/%s#%d: %s](%s)\n",
					pr.RepoOwner, pr.RepoName, pr.Number, pr.Title, pr.URL)
			}
			fmt.Fprintln(file)
		}

		// Issues Created
		if len(githubActivity.IssuesCreated) > 0 {
			for _, issue := range githubActivity.IssuesCreated {
				fmt.Fprintf(file, "- [%s/%s#%d: %s](%s)\n",
					issue.RepoOwner, issue.RepoName, issue.Number, issue.Title, issue.URL)
			}
			fmt.Fprintln(file)
		}

		// Pull Request Reviews
		if len(githubActivity.PullRequestsReviewed) > 0 {
			for _, pr := range githubActivity.PullRequestsReviewed {
				fmt.Fprintf(file, "- [%s/%s#%d: %s](%s) - Reviewed\n",
					pr.RepoOwner, pr.RepoName, pr.Number, pr.Title, pr.URL)
			}
			fmt.Fprintln(file)
		}

		// Commits by Repository
		if len(githubActivity.CommitsByRepo) > 0 {
			for repo, count := range githubActivity.CommitsByRepo {
				fmt.Fprintf(file, "- %s: %d commit(s)\n", repo, count)
			}
			fmt.Fprintln(file)
		}
	}

	// Linear Issues Section
	if len(issuesWithNotes) > 0 {
		fmt.Fprintf(file, "## Linear Issues\n\n")

		for _, issueNote := range issuesWithNotes {
			issue := issueNote.Details

			// First level: URL + short description
			fmt.Fprintf(file, "- [%s: %s](%s)\n", issue.Identifier, issue.Title, issue.URL)

			// Second level: user notes (if any)
			if issueNote.UserNotes != "" {
				noteLines := strings.Split(issueNote.UserNotes, "\n")
				for _, line := range noteLines {
					if strings.TrimSpace(line) != "" {
						fmt.Fprintf(file, "  - %s\n", strings.TrimSpace(line))
					}
				}
			}
		}
		fmt.Fprintln(file)
	}

	if len(issuesWithNotes) == 0 && githubActivity.TotalCommits == 0 &&
		githubActivity.TotalPullRequests == 0 && githubActivity.TotalReviews == 0 &&
		githubActivity.TotalIssues == 0 {
		fmt.Fprintln(file, "No activity recorded for this period.")
	}

	return nil
}
