package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"cli/main/cmd/gmail/tui"
	"cli/main/pkg/gmail"
	"cli/main/pkg/gmail/cache"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze your Gmail inbox for statistics and insights",
	Long: `Analyze your Gmail inbox to get insights about:
- Sender statistics (top senders, emails by domain)
- Size analysis (largest emails, storage usage)
- Date distribution (email activity over time)
- Attachment analysis (types, sizes, counts)
- Pattern matching (find emails matching regex)

Examples:
  # Analyze sender statistics
  mastercrab gmail analyze --analysis-type sender-stats

  # Multiple analysis types
  mastercrab gmail analyze --analysis-type sender-stats,size-analysis

  # With date filter
  mastercrab gmail analyze --analysis-type sender-stats --date-from 2024-01-01

  # JSON output
  mastercrab gmail analyze --analysis-type sender-stats --output json`,
	Run: runAnalyze,
}

var (
	analysisTypes []string
	dateFrom      string
	dateTo        string
	outputFormat  string
	analysisLimit int64
	showProgress  bool
	regexPattern  string
	searchIn      string
	interactive   bool
	useCache      bool
)

func init() {
	analyzeCmd.Flags().StringSliceVar(&analysisTypes, "analysis-type", []string{"sender-stats"},
		"Analysis types: sender-stats, size-analysis, date-analysis, attachment-analysis, regex-patterns")
	analyzeCmd.Flags().StringVar(&dateFrom, "date-from", "",
		"Filter emails from this date (YYYY-MM-DD)")
	analyzeCmd.Flags().StringVar(&dateTo, "date-to", "",
		"Filter emails to this date (YYYY-MM-DD)")
	analyzeCmd.Flags().StringVar(&outputFormat, "output", "table",
		"Output format: table, json, markdown")
	analyzeCmd.Flags().Int64Var(&analysisLimit, "limit", 0,
		"Max emails to analyze (0 = use config default)")
	analyzeCmd.Flags().BoolVar(&showProgress, "progress", true,
		"Show progress indicator")
	analyzeCmd.Flags().StringVar(&regexPattern, "regex", "",
		"Regex pattern for regex-patterns analysis")
	analyzeCmd.Flags().StringVar(&searchIn, "search-in", "subject",
		"Where to search for regex: subject, from, body, all")
	analyzeCmd.Flags().BoolVar(&interactive, "interactive", false,
		"Launch interactive TUI for email selection and deletion")
	analyzeCmd.Flags().BoolVar(&useCache, "use-cache", true,
		"Use cache for faster analysis")
}

func runAnalyze(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// Check if interactive mode
	if interactive {
		runInteractiveMode(ctx)
		return
	}

	fmt.Println("📧 Gmail Inbox Analysis")
	fmt.Println(strings.Repeat("═", 80))

	// Create auth manager
	authMgr := gmail.NewAuthManager(viper.GetViper())

	// Get authenticated client
	fmt.Println("\n🔐 Authenticating with Gmail...")
	httpClient, err := authMgr.GetAuthClient(ctx)
	if err != nil {
		fmt.Printf("❌ Authentication failed: %s\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Authenticated successfully")

	// Create Gmail client
	gmailClient, err := gmail.NewGmailClient(ctx, httpClient, viper.GetViper())
	if err != nil {
		fmt.Printf("❌ Failed to create Gmail client: %s\n", err)
		os.Exit(1)
	}

	// Create analyzer
	analyzer := gmail.NewAnalyzer(gmailClient)

	// Build query from date filters
	query := buildQuery()

	// Get limit
	limit := analysisLimit
	if limit == 0 {
		limit = int64(viper.GetInt("gmail.defaultAnalysisLimit"))
		if limit == 0 {
			limit = 5000
		}
	}

	fmt.Printf("\n🔍 Analyzing up to %d emails...\n", limit)
	if query != "" {
		fmt.Printf("   Query: %s\n", query)
	}

	// Run requested analyses
	for _, analysisType := range analysisTypes {
		analysisType = strings.TrimSpace(analysisType)
		fmt.Printf("\n📊 Running %s analysis...\n", analysisType)

		switch analysisType {
		case "sender-stats":
			runSenderStats(ctx, analyzer, gmailClient, query, limit)
		case "size-analysis":
			runSizeAnalysis(ctx, analyzer, query, limit)
		case "date-analysis":
			runDateAnalysis(ctx, analyzer, query, limit)
		case "attachment-analysis":
			runAttachmentAnalysis(ctx, analyzer, query, limit)
		case "regex-patterns":
			runRegexPatterns(ctx, analyzer, query, limit)
		default:
			fmt.Printf("⚠️  Unknown analysis type: %s\n", analysisType)
		}
	}

	fmt.Println("\n✨ Analysis complete!")
}

func buildQuery() string {
	qb := gmail.NewQueryBuilder()

	if dateFrom != "" {
		// Convert YYYY-MM-DD to Gmail format
		date := strings.ReplaceAll(dateFrom, "-", "/")
		qb.After(date)
	}

	if dateTo != "" {
		date := strings.ReplaceAll(dateTo, "-", "/")
		qb.Before(date)
	}

	return qb.Build()
}

func runSenderStats(ctx context.Context, analyzer *gmail.Analyzer, gmailClient *gmail.GmailClient, query string, limit int64) {
	// Get cache configuration
	cacheEnabled := viper.GetBool("gmail.cache.enabled")
	cacheLocation := viper.GetString("gmail.cache.location")

	var cacheDB *cache.Cache
	var stats []gmail.SenderStats
	var err error

	// Try to use cache if enabled
	if cacheEnabled && cacheLocation != "" && outputFormat == "table" {
		fmt.Println("\n💾 Checking cache...")

		// Open cache
		cacheDB, err = cache.NewCache(cacheLocation)
		if err != nil {
			fmt.Printf("⚠️  Failed to open cache: %s\n", err)
			fmt.Println("   Falling back to direct Gmail API query...")
			stats, err = analyzer.AnalyzeSenderStats(ctx, query, limit)
			if err != nil {
				fmt.Printf("❌ Failed to analyze sender stats: %s\n", err)
				return
			}
		} else {
			defer cacheDB.Close()

			// Check if cache needs sync
			lastSync, err := cacheDB.GetLastSyncTime()
			if err != nil {
				fmt.Printf("⚠️  Failed to get last sync time: %s\n", err)
			}

			syncInterval := time.Duration(viper.GetInt("gmail.cache.syncIntervalMinutes")) * time.Minute
			if syncInterval == 0 {
				syncInterval = 60 * time.Minute
			}

			needsSync := lastSync.IsZero() || time.Since(lastSync) > syncInterval

			if needsSync {
				fmt.Printf("🔄 Cache is stale (last sync: %s). Syncing...\n", formatLastSync(lastSync))
				if err := syncCache(ctx, gmailClient, cacheDB); err != nil {
					fmt.Printf("❌ Failed to sync cache: %s\n", err)
					return
				}
			} else {
				fmt.Printf("✅ Cache is up to date (last sync: %s)\n", formatLastSync(lastSync))
			}

			// Get stats from cache
			stats, err = computeSenderStatsFromCache(cacheDB)
			if err != nil {
				fmt.Printf("❌ Failed to compute sender stats from cache: %s\n", err)
				return
			}
		}
	} else {
		// Use direct API query for non-table formats or if cache disabled
		stats, err = analyzer.AnalyzeSenderStats(ctx, query, limit)
		if err != nil {
			fmt.Printf("❌ Failed to analyze sender stats: %s\n", err)
			return
		}
	}

	if len(stats) == 0 {
		fmt.Println("   No emails found")
		return
	}

	switch outputFormat {
	case "json":
		outputJSON(stats)
	case "markdown":
		outputSenderStatsMarkdown(stats)
	default:
		outputSenderStatsTable(stats, gmailClient, cacheDB, ctx)
	}
}

// computeSenderStatsFromCache aggregates cached emails by sender
func computeSenderStatsFromCache(cacheDB *cache.Cache) ([]gmail.SenderStats, error) {
	emails, err := cacheDB.GetCachedEmails()
	if err != nil {
		return nil, fmt.Errorf("failed to get cached emails: %w", err)
	}

	// Aggregate by sender
	statsMap := make(map[string]*gmail.SenderStats)

	for _, email := range emails {
		sender := email.FromEmail
		if sender == "" {
			continue
		}

		if _, exists := statsMap[sender]; !exists {
			// Extract domain
			domain := ""
			parts := strings.Split(sender, "@")
			if len(parts) == 2 {
				domain = parts[1]
			}

			statsMap[sender] = &gmail.SenderStats{
				Email:     sender,
				Domain:    domain,
				Count:     0,
				TotalSize: 0,
				HasUnread: false,
				FirstSeen: email.Date,
				LastSeen:  email.Date,
			}
		}

		stat := statsMap[sender]
		stat.Count++
		stat.TotalSize += email.Size

		if email.Date.Before(stat.FirstSeen) {
			stat.FirstSeen = email.Date
		}
		if email.Date.After(stat.LastSeen) {
			stat.LastSeen = email.Date
		}

		// Check if any email from this sender is unread (check labels)
		if !stat.HasUnread {
			for _, label := range email.Labels {
				if label == "UNREAD" {
					stat.HasUnread = true
					break
				}
			}
		}
	}

	// Convert map to slice and sort by total size (descending)
	stats := make([]gmail.SenderStats, 0, len(statsMap))
	for _, stat := range statsMap {
		stats = append(stats, *stat)
	}

	// Sort by total size (largest first)
	for i := 0; i < len(stats)-1; i++ {
		for j := i + 1; j < len(stats); j++ {
			if stats[j].TotalSize > stats[i].TotalSize {
				stats[i], stats[j] = stats[j], stats[i]
			}
		}
	}

	return stats, nil
}

func runSizeAnalysis(ctx context.Context, analyzer *gmail.Analyzer, query string, limit int64) {
	sizes, err := analyzer.AnalyzeSizeDistribution(ctx, query, limit)
	if err != nil {
		fmt.Printf("❌ Failed to analyze sizes: %s\n", err)
		return
	}

	if len(sizes) == 0 {
		fmt.Println("   No emails found")
		return
	}

	// Show top 20 largest emails
	if len(sizes) > 20 {
		sizes = sizes[:20]
	}

	switch outputFormat {
	case "json":
		outputJSON(sizes)
	case "markdown":
		outputSizeAnalysisMarkdown(sizes)
	default:
		outputSizeAnalysisTable(sizes)
	}
}

func runDateAnalysis(ctx context.Context, analyzer *gmail.Analyzer, query string, limit int64) {
	distribution, err := analyzer.AnalyzeDateDistribution(ctx, query, limit)
	if err != nil {
		fmt.Printf("❌ Failed to analyze date distribution: %s\n", err)
		return
	}

	if len(distribution) == 0 {
		fmt.Println("   No emails found")
		return
	}

	switch outputFormat {
	case "json":
		outputJSON(distribution)
	case "markdown":
		outputDateDistributionMarkdown(distribution)
	default:
		outputDateDistributionTable(distribution)
	}
}

func runAttachmentAnalysis(ctx context.Context, analyzer *gmail.Analyzer, query string, limit int64) {
	attachments, err := analyzer.AnalyzeAttachments(ctx, query, limit)
	if err != nil {
		fmt.Printf("❌ Failed to analyze attachments: %s\n", err)
		return
	}

	if len(attachments) == 0 {
		fmt.Println("   No attachments found")
		return
	}

	switch outputFormat {
	case "json":
		outputJSON(attachments)
	case "markdown":
		outputAttachmentAnalysisMarkdown(attachments)
	default:
		outputAttachmentAnalysisTable(attachments)
	}
}

func runRegexPatterns(ctx context.Context, analyzer *gmail.Analyzer, query string, limit int64) {
	if regexPattern == "" {
		fmt.Println("❌ --regex flag is required for regex-patterns analysis")
		return
	}

	matches, err := analyzer.AnalyzeRegexPatterns(ctx, regexPattern, searchIn, query, limit)
	if err != nil {
		fmt.Printf("❌ Failed to find regex matches: %s\n", err)
		return
	}

	if len(matches) == 0 {
		fmt.Printf("   No matches found for pattern: %s\n", regexPattern)
		return
	}

	fmt.Printf("   Found %d matches for pattern: %s\n", len(matches), regexPattern)

	switch outputFormat {
	case "json":
		outputJSON(matches)
	case "markdown":
		outputRegexMatchesMarkdown(matches)
	default:
		outputRegexMatchesTable(matches)
	}
}

// Output formatters

func outputJSON(data interface{}) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("❌ Failed to marshal JSON: %s\n", err)
		return
	}
	fmt.Println(string(jsonData))
}

// outputSenderStatsTable launches an interactive TUI for sender stats analysis
func outputSenderStatsTable(stats []gmail.SenderStats, gmailClient *gmail.GmailClient, cacheDB *cache.Cache, ctx context.Context) {
	// Create deleter with cache
	deleter := gmail.NewDeleter(gmailClient, cacheDB)

	// Create and run the interactive TUI
	m := newSenderStatsModel(stats, gmailClient, deleter, cacheDB, ctx)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("❌ Error running interactive analysis: %s\n", err)
		os.Exit(1)
	}
}

// senderStatsModel represents the TUI state for sender stats analysis
type senderStatsModel struct {
	table       table.Model
	stats       []gmail.SenderStats
	selected    map[int]bool
	state       senderStatsViewState
	width       int
	height      int
	gmailClient *gmail.GmailClient
	deleter     *gmail.Deleter
	cacheDB     *cache.Cache
	ctx         context.Context
	err         error
}

type senderStatsViewState int

const (
	viewTable senderStatsViewState = iota
	viewConfirm
	viewDeleting
	viewDone
)

// newSenderStatsModel creates a new sender stats TUI model
func newSenderStatsModel(stats []gmail.SenderStats, gmailClient *gmail.GmailClient, deleter *gmail.Deleter, cacheDB *cache.Cache, ctx context.Context) senderStatsModel {
	// Define columns with proper widths
	columns := []table.Column{
		{Title: "Action Symbol", Width: 15},
		{Title: "Sender", Width: 30},
		{Title: "Count", Width: 10},
		{Title: "Size", Width: 10},
		{Title: "Impact", Width: 10},
	}

	// Create rows from stats
	rows := make([]table.Row, len(stats))
	for i, stat := range stats {
		// Calculate impact bar (relative to max)
		impactBar := calculateImpactBar(stat.TotalSize, stats)

		rows[i] = table.Row{
			"",  // Action symbol (empty initially)
			stat.Email,
			fmt.Sprintf("%d", stat.Count),
			formatSize(stat.TotalSize),
			impactBar,
		}
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(20),
	)

	// Set table styles
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true).
		Align(lipgloss.Center)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("196")). // Bright red
		Bold(true)
	t.SetStyles(s)

	return senderStatsModel{
		table:       t,
		stats:       stats,
		selected:    make(map[int]bool),
		state:       viewTable,
		gmailClient: gmailClient,
		deleter:     deleter,
		cacheDB:     cacheDB,
		ctx:         ctx,
	}
}

// calculateImpactBar creates a visual bar representing email impact
func calculateImpactBar(size int64, allStats []gmail.SenderStats) string {
	if len(allStats) == 0 {
		return ""
	}

	// Find max size for normalization
	maxSize := int64(0)
	for _, stat := range allStats {
		if stat.TotalSize > maxSize {
			maxSize = stat.TotalSize
		}
	}

	if maxSize == 0 {
		return ""
	}

	// Create bar (0-10 blocks)
	barLength := int((float64(size) / float64(maxSize)) * 10)
	return strings.Repeat("█", barLength)
}

// Init initializes the model
func (m senderStatsModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m senderStatsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch m.state {
		case viewTable:
			return m.handleTableInput(msg)
		case viewConfirm:
			return m.handleConfirmInput(msg)
		case viewDone:
			if msg.String() == "q" || msg.String() == "enter" {
				return m, tea.Quit
			}
		}

	case deletionCompleteMsg:
		m.state = viewDone
		if msg.err != nil {
			m.err = msg.err
		}
		return m, nil
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// handleTableInput handles input in table view
func (m senderStatsModel) handleTableInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit

	case " ": // Space to toggle selection
		cursor := m.table.Cursor()
		m.selected[cursor] = !m.selected[cursor]
		m.updateTableRows()
		return m, nil

	case "a": // Select all
		for i := range m.stats {
			m.selected[i] = true
		}
		m.updateTableRows()
		return m, nil

	case "n": // Select none
		m.selected = make(map[int]bool)
		m.updateTableRows()
		return m, nil

	case "enter": // Proceed to confirmation
		if len(m.getSelectedIndices()) == 0 {
			return m, nil // Nothing selected
		}
		m.state = viewConfirm
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// handleConfirmInput handles input in confirmation view
func (m senderStatsModel) handleConfirmInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		// Start deletion process
		m.state = viewDeleting
		return m, m.executeDeletion()
	case "n", "N", "esc":
		m.state = viewTable
		return m, nil
	}
	return m, nil
}

// executeDeletion performs the actual deletion
func (m *senderStatsModel) executeDeletion() tea.Cmd {
	return func() tea.Msg {
		// Get selected senders
		selectedIndices := m.getSelectedIndices()
		if len(selectedIndices) == 0 {
			return deletionCompleteMsg{err: fmt.Errorf("no senders selected")}
		}

		// Get message IDs from cache for selected senders
		if m.cacheDB == nil {
			return deletionCompleteMsg{err: fmt.Errorf("cache not available for deletion")}
		}

		emails, err := m.cacheDB.GetCachedEmails()
		if err != nil {
			return deletionCompleteMsg{err: fmt.Errorf("failed to get cached emails: %w", err)}
		}

		// Build list of message IDs for selected senders
		var messageIDs []string
		senderSet := make(map[string]bool)
		for _, idx := range selectedIndices {
			senderSet[m.stats[idx].Email] = true
		}

		for _, email := range emails {
			if senderSet[email.FromEmail] {
				messageIDs = append(messageIDs, email.ID)
			}
		}

		if len(messageIDs) == 0 {
			return deletionCompleteMsg{err: fmt.Errorf("no emails found for selected senders")}
		}

		// Execute deletion
		batchSize := viper.GetInt("gmail.deletionBatchSize")
		if batchSize == 0 {
			batchSize = 100
		}

		progressCh := make(chan int, len(messageIDs))
		go func() {
			if err := m.deleter.ExecuteDeletion(messageIDs, batchSize, progressCh); err != nil {
				// Error will be handled
			}
		}()

		// Wait for completion
		for range progressCh {
			// Progress updates
		}

		return deletionCompleteMsg{count: len(messageIDs)}
	}
}

type deletionCompleteMsg struct {
	count int
	err   error
}

// updateTableRows updates table rows with selection status
func (m *senderStatsModel) updateTableRows() {
	rows := make([]table.Row, len(m.stats))
	for i, stat := range m.stats {
		actionSymbol := ""
		if m.selected[i] {
			actionSymbol = "🗑️"
		}

		impactBar := calculateImpactBar(stat.TotalSize, m.stats)

		rows[i] = table.Row{
			actionSymbol,
			stat.Email,
			fmt.Sprintf("%d", stat.Count),
			formatSize(stat.TotalSize),
			impactBar,
		}
	}
	m.table.SetRows(rows)
}

// getSelectedIndices returns indices of selected senders
func (m senderStatsModel) getSelectedIndices() []int {
	var indices []int
	for i := range m.stats {
		if m.selected[i] {
			indices = append(indices, i)
		}
	}
	return indices
}

// View renders the UI
func (m senderStatsModel) View() string {
	switch m.state {
	case viewTable:
		return m.renderTableView()
	case viewConfirm:
		return m.renderConfirmView()
	case viewDeleting:
		return m.renderDeletingView()
	case viewDone:
		return m.renderDoneView()
	default:
		return ""
	}
}

// renderTableView renders the table view
func (m senderStatsModel) renderTableView() string {
	var b strings.Builder

	b.WriteString("\n📧 Gmail Sender Statistics - Interactive Analysis\n")
	b.WriteString(strings.Repeat("═", 80) + "\n\n")
	b.WriteString(m.table.View())
	b.WriteString("\n\n")

	// Show help
	b.WriteString("[SPACE] Select  [A] Select All  [N] Select None  [ENTER] Delete  [Q] Quit\n\n")

	// Show selection summary
	selectedCount := len(m.getSelectedIndices())
	if selectedCount > 0 {
		totalEmails := 0
		totalSize := int64(0)
		for idx := range m.selected {
			if m.selected[idx] {
				totalEmails += m.stats[idx].Count
				totalSize += m.stats[idx].TotalSize
			}
		}
		b.WriteString(fmt.Sprintf("Selected: %d sender(s) (%d emails, %s)\n",
			selectedCount, totalEmails, formatSize(totalSize)))
	}

	return b.String()
}

// renderConfirmView renders the confirmation dialog
func (m senderStatsModel) renderConfirmView() string {
	var b strings.Builder

	selectedIndices := m.getSelectedIndices()
	totalEmails := 0
	totalSize := int64(0)

	b.WriteString("\n┌──────────────────────────────────────────────────────┐\n")
	b.WriteString("│                                                      │\n")
	b.WriteString("│  ⚠️  Confirm Deletion                                │\n")
	b.WriteString("│                                                      │\n")
	b.WriteString(fmt.Sprintf("│  You are about to delete emails from %d sender(s):   │\n", len(selectedIndices)))
	b.WriteString("│                                                      │\n")

	for i, idx := range selectedIndices {
		stat := m.stats[idx]
		totalEmails += stat.Count
		totalSize += stat.TotalSize

		b.WriteString(fmt.Sprintf("│  %d. %-45s │\n", i+1, truncate(stat.Email, 45)))
		b.WriteString(fmt.Sprintf("│     Emails: %d  |  Size: %-28s │\n",
			stat.Count, formatSize(stat.TotalSize)))
		b.WriteString("│                                                      │\n")
	}

	b.WriteString(fmt.Sprintf("│  Total: %d emails (%s)%-25s │\n", totalEmails, formatSize(totalSize), ""))
	b.WriteString("│                                                      │\n")
	b.WriteString("│  This action cannot be undone.                      │\n")
	b.WriteString("│                                                      │\n")
	b.WriteString("│  [Y] Confirm    [N] Cancel                          │\n")
	b.WriteString("│                                                      │\n")
	b.WriteString("└──────────────────────────────────────────────────────┘\n")

	return b.String()
}

// renderDeletingView renders the deletion progress view
func (m senderStatsModel) renderDeletingView() string {
	var b strings.Builder

	b.WriteString("\n\n")
	b.WriteString("   🗑️  Deleting emails...\n\n")
	b.WriteString("   Please wait while emails are being deleted.\n")
	b.WriteString("   This may take a moment.\n\n")

	return b.String()
}

// renderDoneView renders the completion view
func (m senderStatsModel) renderDoneView() string {
	var b strings.Builder

	b.WriteString("\n\n")
	if m.err != nil {
		b.WriteString("   ❌ Deletion failed\n\n")
		b.WriteString(fmt.Sprintf("   Error: %s\n\n", m.err))
	} else {
		b.WriteString("   ✅ Deletion complete!\n\n")
		b.WriteString("   All selected emails have been deleted.\n\n")
	}

	b.WriteString("   Press [Q] or [ENTER] to exit\n")

	return b.String()
}

func outputSizeAnalysisTable(sizes []gmail.EmailSize) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FROM\tSUBJECT\tSIZE\tATTACHMENTS\tDATE")
	fmt.Fprintln(w, strings.Repeat("─", 80))

	for _, size := range sizes {
		hasAtt := ""
		if size.HasAttachments {
			hasAtt = fmt.Sprintf("✓ (%s)", formatSize(size.AttachmentSize))
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			truncate(size.From, 25),
			truncate(size.Subject, 30),
			formatSize(size.Size),
			hasAtt,
			size.Date.Format("2006-01-02"),
		)
	}
	w.Flush()
}

func outputDateDistributionTable(distribution []gmail.DateDistribution) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DATE\tCOUNT\tTOTAL SIZE\tAVG SIZE")
	fmt.Fprintln(w, strings.Repeat("─", 60))

	for _, dist := range distribution {
		avgSize := int64(0)
		if dist.Count > 0 {
			avgSize = dist.Size / int64(dist.Count)
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\n",
			dist.Date.Format("2006-01-02"),
			dist.Count,
			formatSize(dist.Size),
			formatSize(avgSize),
		)
	}
	w.Flush()
}

func outputAttachmentAnalysisTable(attachments []gmail.AttachmentAnalysis) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\tEXTENSION\tCOUNT\tTOTAL SIZE\tAVG SIZE")
	fmt.Fprintln(w, strings.Repeat("─", 70))

	for _, att := range attachments {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			truncate(att.MimeType, 30),
			att.Extension,
			att.Count,
			formatSize(att.TotalSize),
			formatSize(att.AvgSize),
		)
	}
	w.Flush()
}

func outputRegexMatchesTable(matches []gmail.RegexMatch) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FROM\tSUBJECT\tMATCHED ON\tDATE")
	fmt.Fprintln(w, strings.Repeat("─", 80))

	for i, match := range matches {
		if i >= 50 { // Limit to 50 matches
			fmt.Printf("\n... and %d more matches\n", len(matches)-50)
			break
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			truncate(match.From, 25),
			truncate(match.Subject, 35),
			match.MatchedOn,
			match.Date.Format("2006-01-02"),
		)
	}
	w.Flush()
}

// Markdown formatters (simplified versions)

func outputSenderStatsMarkdown(stats []gmail.SenderStats) {
	fmt.Println("\n## Sender Statistics\n")
	fmt.Println("| Sender | Domain | Count | Total Size | Unread |")
	fmt.Println("|--------|--------|-------|------------|--------|")
	for i, stat := range stats {
		if i >= 20 {
			break
		}
		unread := ""
		if stat.HasUnread {
			unread = "✓"
		}
		fmt.Printf("| %s | %s | %d | %s | %s |\n",
			stat.Email, stat.Domain, stat.Count, formatSize(stat.TotalSize), unread)
	}
}

func outputSizeAnalysisMarkdown(sizes []gmail.EmailSize) {
	fmt.Println("\n## Largest Emails\n")
	fmt.Println("| From | Subject | Size | Date |")
	fmt.Println("|------|---------|------|------|")
	for _, size := range sizes {
		fmt.Printf("| %s | %s | %s | %s |\n",
			truncate(size.From, 30), truncate(size.Subject, 40),
			formatSize(size.Size), size.Date.Format("2006-01-02"))
	}
}

func outputDateDistributionMarkdown(distribution []gmail.DateDistribution) {
	fmt.Println("\n## Email Distribution Over Time\n")
	fmt.Println("| Date | Count | Total Size |")
	fmt.Println("|------|-------|------------|")
	for _, dist := range distribution {
		fmt.Printf("| %s | %d | %s |\n",
			dist.Date.Format("2006-01-02"), dist.Count, formatSize(dist.Size))
	}
}

func outputAttachmentAnalysisMarkdown(attachments []gmail.AttachmentAnalysis) {
	fmt.Println("\n## Attachment Analysis\n")
	fmt.Println("| Type | Extension | Count | Total Size |")
	fmt.Println("|------|-----------|-------|------------|")
	for _, att := range attachments {
		fmt.Printf("| %s | %s | %d | %s |\n",
			att.MimeType, att.Extension, att.Count, formatSize(att.TotalSize))
	}
}

func outputRegexMatchesMarkdown(matches []gmail.RegexMatch) {
	fmt.Println("\n## Pattern Matches\n")
	fmt.Println("| From | Subject | Matched On | Date |")
	fmt.Println("|------|---------|------------|------|")
	for i, match := range matches {
		if i >= 50 {
			fmt.Printf("\n*... and %d more matches*\n", len(matches)-50)
			break
		}
		fmt.Printf("| %s | %s | %s | %s |\n",
			match.From, match.Subject, match.MatchedOn, match.Date.Format("2006-01-02"))
	}
}

// Helper functions

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	hours := int(d.Hours())
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

func runInteractiveMode(ctx context.Context) {
	fmt.Println("📧 Gmail Interactive Mode")
	fmt.Println(strings.Repeat("═", 80))

	// Get cache configuration
	cacheEnabled := viper.GetBool("gmail.cache.enabled")
	cacheLocation := viper.GetString("gmail.cache.location")

	if !cacheEnabled || cacheLocation == "" {
		fmt.Println("❌ Cache is not enabled. Interactive mode requires cache to be configured.")
		fmt.Println("   Please set gmail.cache.enabled=true and gmail.cache.location in your config.")
		os.Exit(1)
	}

	// Create auth manager
	authMgr := gmail.NewAuthManager(viper.GetViper())

	// Get authenticated client
	fmt.Println("\n🔐 Authenticating with Gmail...")
	httpClient, err := authMgr.GetAuthClient(ctx)
	if err != nil {
		fmt.Printf("❌ Authentication failed: %s\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Authenticated successfully")

	// Create Gmail client
	gmailClient, err := gmail.NewGmailClient(ctx, httpClient, viper.GetViper())
	if err != nil {
		fmt.Printf("❌ Failed to create Gmail client: %s\n", err)
		os.Exit(1)
	}

	// Open cache
	fmt.Printf("\n💾 Opening cache at %s...\n", cacheLocation)
	cacheDB, err := cache.NewCache(cacheLocation)
	if err != nil {
		fmt.Printf("❌ Failed to open cache: %s\n", err)
		os.Exit(1)
	}
	defer cacheDB.Close()

	// Check last sync time
	lastSync, err := cacheDB.GetLastSyncTime()
	if err != nil {
		fmt.Printf("❌ Failed to get last sync time: %s\n", err)
		os.Exit(1)
	}

	// Sync if cache is stale or empty
	syncInterval := time.Duration(viper.GetInt("gmail.cache.syncIntervalMinutes")) * time.Minute
	if syncInterval == 0 {
		syncInterval = 60 * time.Minute
	}

	needsSync := lastSync.IsZero() || time.Since(lastSync) > syncInterval

	if needsSync {
		fmt.Printf("\n🔄 Cache is stale (last sync: %s). Syncing with Gmail...\n", formatLastSync(lastSync))
		if err := syncCache(ctx, gmailClient, cacheDB); err != nil {
			fmt.Printf("❌ Failed to sync cache: %s\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("\n✅ Cache is up to date (last sync: %s)\n", formatLastSync(lastSync))
	}

	// Get cached emails
	fmt.Println("\n📊 Loading cached emails...")
	emails, err := cacheDB.GetCachedEmails()
	if err != nil {
		fmt.Printf("❌ Failed to load cached emails: %s\n", err)
		os.Exit(1)
	}

	if len(emails) == 0 {
		fmt.Println("❌ No emails found in cache")
		os.Exit(1)
	}

	fmt.Printf("✅ Loaded %d emails from cache\n\n", len(emails))

	// Create deleter
	deleter := gmail.NewDeleter(gmailClient, cacheDB)

	// Launch TUI
	fmt.Println("🚀 Launching interactive mode...")
	time.Sleep(500 * time.Millisecond) // Brief pause for user to read

	if err := tui.Run(emails, deleter); err != nil {
		fmt.Printf("\n❌ TUI error: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("\n👋 Goodbye!")
}

func syncCache(ctx context.Context, gmailClient *gmail.GmailClient, cacheDB *cache.Cache) error {
	// Build query from date filters
	query := buildQuery()

	// Create rate limiter from config
	limiter := createRateLimiter()

	// Create progress tracker with display callbacks
	tracker := createProgressDisplay()

	fmt.Println("\n📧 Syncing inbox...")

	// Use the sync function with progress tracking
	if err := gmailClient.SyncToCache(ctx, cacheDB, query, tracker, limiter); err != nil {
		fmt.Fprintf(os.Stderr, "\n") // New line after progress
		return err
	}

	// Print final newline after progress display
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Println("✅ Sync complete!")

	return nil
}

// createRateLimiter creates a rate limiter from config
func createRateLimiter() *gmail.RateLimiter {
	config := &gmail.RateLimitConfig{
		RequestsPerSecond: float64(viper.GetInt("gmail.rateLimiting.requestsPerSecond")),
		BurstCapacity:     int64(viper.GetInt("gmail.rateLimiting.burstCapacity")),
		MaxRetries:        viper.GetInt("gmail.rateLimiting.maxRetries"),
		InitialBackoffMs:  viper.GetInt("gmail.rateLimiting.initialBackoffMs"),
		MaxBackoffMs:      viper.GetInt("gmail.rateLimiting.maxBackoffMs"),
	}

	// Use defaults if not configured
	if config.RequestsPerSecond <= 0 {
		config.RequestsPerSecond = 100
	}
	if config.BurstCapacity <= 0 {
		config.BurstCapacity = 1000
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 5
	}
	if config.InitialBackoffMs <= 0 {
		config.InitialBackoffMs = 100
	}
	if config.MaxBackoffMs <= 0 {
		config.MaxBackoffMs = 5000
	}

	return gmail.NewRateLimiter(config)
}

// createProgressDisplay creates a progress tracker with display callbacks
func createProgressDisplay() *gmail.ProgressTracker {
	var lastUpdate time.Time
	var mu sync.Mutex
	var linesWritten int

	return gmail.NewProgressTracker(
		// Fetch callback - shows download progress
		func(event *gmail.ProgressEvent) {
			mu.Lock()
			defer mu.Unlock()

			// Throttle updates to avoid flooding (max 10 updates/sec)
			now := time.Now()
			if now.Sub(lastUpdate) < 100*time.Millisecond {
				return
			}
			lastUpdate = now

			// Move cursor up to overwrite previous lines
			if linesWritten > 0 {
				fmt.Fprintf(os.Stderr, "\033[%dA", linesWritten)
			}

			// Line 1: Download progress
			percentage := ""
			totalStr := "?"
			if event.Total > 0 {
				pct := float64(event.Current) / float64(event.Total) * 100
				percentage = fmt.Sprintf(" | %.0f%%", pct)
				totalStr = formatNumber(event.Total)
			}

			rateStr := ""
			if event.ItemsPerSec > 0 {
				rateStr = fmt.Sprintf(" (%.1f msgs/sec)", event.ItemsPerSec)
			}

			fmt.Fprintf(os.Stderr, "\r\033[KDownloading: %s/%s messages%s%s\n",
				formatNumber(event.Current), totalStr, rateStr, percentage)

			linesWritten = 1
		},
		// Cache callback - shows caching progress
		func(event *gmail.ProgressEvent) {
			mu.Lock()
			defer mu.Unlock()

			// Line 2: Cache progress
			percentage := ""
			totalStr := "?"
			if event.Total > 0 {
				pct := float64(event.Current) / float64(event.Total) * 100
				percentage = fmt.Sprintf(" | %.0f%%", pct)
				totalStr = formatNumber(event.Total)
			}

			rateStr := ""
			if event.ItemsPerSec > 0 {
				rateStr = fmt.Sprintf(" (%.1f msgs/sec)", event.ItemsPerSec)
			}

			etaStr := ""
			if event.EstimatedRemaining > 0 {
				etaStr = fmt.Sprintf(" | Est. %s remaining", formatDuration(event.EstimatedRemaining))
			}

			fmt.Fprintf(os.Stderr, "\r\033[KCaching:     %s/%s messages%s%s%s\n",
				formatNumber(event.Current), totalStr, rateStr, percentage, etaStr)

			linesWritten = 2
		},
	)
}

// formatNumber formats a number with thousand separators
func formatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%s,%03d", formatNumber(n/1000), n%1000)
}

func formatLastSync(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	duration := time.Since(t)
	if duration < time.Minute {
		return "just now"
	}
	if duration < time.Hour {
		return fmt.Sprintf("%d minutes ago", int(duration.Minutes()))
	}
	if duration < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(duration.Hours()))
	}
	return fmt.Sprintf("%d days ago", int(duration.Hours()/24))
}
