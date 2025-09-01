package tui

import (
	"fmt"
	"strings"

	"gooru.local/gooru"
	"gooru.local/gooru/types"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// App encapsulates the TUI application.
type App struct {
	gooruClient *gooru.Client
	tviewApp    *tview.Application
	input       *tview.InputField
	results     *tview.Table
}

// NewApp creates a new TUI application.
func NewApp(client *gooru.Client) (*App, error) {
	app := &App{
		gooruClient: client,
		tviewApp:    tview.NewApplication().EnableMouse(true),
	}
	app.initComponents()
	return app, nil
}

func (a *App) initComponents() {
	// Search Input Field
	a.input = tview.NewInputField().
		SetLabel("Search: ").
		SetFieldWidth(0).
		SetFieldBackgroundColor(tcell.ColorDarkSlateGray).
		SetLabelColor(tcell.ColorYellow)

	// Results Table
	a.results = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false). // rows selectable, not columns
		SetFixed(1, 0)              // Fix the header row.

	// Help text footer
	help := tview.NewTextView().
		SetText("In Search: Down to focus results | In Results: Up/Down to navigate, Esc or type to search | 'q' to quit").
		SetTextColor(tcell.ColorGray).
		SetTextAlign(tview.AlignCenter)

	// Layout
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.input, 1, 1, true).
		AddItem(a.results, 0, 1, false).
		AddItem(help, 1, 1, false)

	a.tviewApp.SetRoot(flex, true)
	a.setupEventHandlers()
}

func (a *App) setupEventHandlers() {
	a.input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			a.runSearch()
		}
	})

	// Capture input for the search field.
	a.input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// On Down arrow, move focus to the results table if it's not empty.
		if event.Key() == tcell.KeyDown {
			if a.results.GetRowCount() > 1 { // More than just the header
				a.tviewApp.SetFocus(a.results)
				return nil // Event handled
			}
		}
		return event // Process all other events normally
	})

	// Global key handler
	a.tviewApp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Handle events when the results table has focus
		if a.tviewApp.GetFocus() == a.results {
			// If a letter/number is typed, switch focus back to the input field.
			if event.Key() == tcell.KeyRune {
				a.tviewApp.SetFocus(a.input)
				return event // Forward the event to the now-focused input field.
			}
			// Pressing Esc also returns focus to the input field.
			if event.Key() == tcell.KeyEscape {
				a.tviewApp.SetFocus(a.input)
				return nil // Don't forward the escape key.
			}
		}

		// Press 'q' to quit, unless the input field is focused.
		if event.Rune() == 'q' && a.tviewApp.GetFocus() != a.input {
			a.tviewApp.Stop()
			return nil
		}

		return event
	})
}

func (a *App) runSearch() {
	query := a.input.GetText()
	a.results.Clear() // Clear previous results

	// Set headers and column properties for stable widths
	headers := []string{"PATH", "SIZE", "TAGS"}
	expansions := []int{5, 1, 4} // Proportional widths
	for i, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetSelectable(false).
			SetExpansion(expansions[i])
		a.results.SetCell(0, i, cell)
	}

	var files []types.FileInfo
	var err error

	if strings.TrimSpace(query) == "" {
		files, err = a.gooruClient.GetAllFilesInfo()
	} else {
		// The service layer doesn't know about verbose mode from the TUI.
		files, err = a.gooruClient.GetFilesInfoByQuery(query, false)
	}

	if err != nil {
		// Display the error in the table
		a.results.SetCell(1, 0, tview.NewTableCell(fmt.Sprintf("Error: %v", err)).
			SetTextColor(tcell.ColorRed).
			SetExpansion(3)) // Span across all columns
		return
	}

	if len(files) == 0 {
		a.results.SetCell(1, 0, tview.NewTableCell("No results found.").
			SetTextColor(tcell.ColorGray).
			SetExpansion(3))
	} else {
		for row, file := range files {
			// Data for the current row
			rowData := []string{
				file.Path,
				humanReadableSize(file.Size),
				strings.ReplaceAll(file.Tags, ",", ", "),
			}
			// Create cells for the row, applying the expansion factors.
			for col, data := range rowData {
				cell := tview.NewTableCell(data).
					SetExpansion(expansions[col])
				a.results.SetCell(row+1, col, cell)
			}
		}
	}

	a.tviewApp.SetFocus(a.results)
}

// humanReadableSize converts a size in bytes to a human-readable string.
func humanReadableSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

// Run starts the TUI application.
func (a *App) Run() error {
	return a.tviewApp.Run()
}