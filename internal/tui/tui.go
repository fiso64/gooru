package tui

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"gooru.local/gooru"
	"gooru.local/gooru/types"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// App encapsulates the TUI application.
type App struct {
	gooruClient *gooru.Client
	tviewApp    *tview.Application

	// Layout components.
	mainFlex  *tview.Flex
	sidePanel *tview.Flex

	// Interactive widgets.
	input     *tview.InputField
	results   *tview.Table
	thumbnail *ImageView
	tagEditor *tview.InputField
	helpText  *tview.TextView

	// State.
	currentFiles       []types.FileInfo
	selectedFile       *types.FileInfo
	imageLoadRequestID uint64 // Used for debouncing image loads.
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
	// --- Widgets ---

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

	// Thumbnail View
	a.thumbnail = NewImageView(a.tviewApp)

	// Tag Editor
	a.tagEditor = tview.NewInputField().
		SetLabel("Tags: ").
		SetFieldBackgroundColor(tcell.ColorDarkSlateGray)

	// Help text footer
	a.helpText = tview.NewTextView().
		SetText("Tab: cycle focus | Enter in search: run | Enter in tags: save | q: quit").
		SetTextColor(tcell.ColorGray).
		SetTextAlign(tview.AlignCenter)

	// --- Layout ---

	// Left panel contains search input and results table.
	leftPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.input, 1, 0, true).
		AddItem(a.results, 0, 1, false)

	// Right side panel contains thumbnail and tag editor.
	a.sidePanel = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.thumbnail, 0, 1, false).
		AddItem(a.tagEditor, 1, 0, false)
	a.sidePanel.SetBorder(true).SetTitle(" Details ")

	// Main layout is a horizontal flex with left and right panels.
	a.mainFlex = tview.NewFlex().
		AddItem(leftPanel, 0, 2, true).
		AddItem(a.sidePanel, 40, 1, false)

	// Root layout is a vertical flex with the main content and the help text.
	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.mainFlex, 0, 1, true).
		AddItem(a.helpText, 1, 0, false)

	a.tviewApp.SetRoot(root, true)
	a.setupEventHandlers()
}

func (a *App) setupEventHandlers() {
	// When Enter is pressed in search, run the search.
	a.input.SetDoneFunc(func(key tcell.Key) {
		// We only want to search when Enter is pressed. Other keys like Tab/Escape
		// are handled by the global input capture and should not trigger a search.
		if key == tcell.KeyEnter {
			// The entire search process must be in a goroutine.
			// Calling QueueUpdateDraw from an event handler (like SetDoneFunc)
			// without a goroutine will cause a deadlock, as the main UI loop
			// is waiting for the handler to return before it can process the queue.
			go a.runSearch(a.input.GetText(), true)
		}
	})

	// When Enter is pressed in tag editor, save the tags and refresh.
	a.tagEditor.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter || a.selectedFile == nil {
			return
		}

		newTagsRaw := strings.Split(a.tagEditor.GetText(), " ")
		var newTags []string
		for _, t := range newTagsRaw {
			if t != "" {
				newTags = append(newTags, t)
			}
		}

		// Must capture the path and query for the goroutine, as state can change.
		selectedPath := a.selectedFile.Path
		currentSearchQuery := a.input.GetText()

		// Run the blocking database operation in a background goroutine.
		go func() {
			_, err := a.gooruClient.SetTagsForFiles([]string{selectedPath}, newTags, nil)

			// After the DB write is done, queue the UI update to run on the main thread.
			a.tviewApp.QueueUpdateDraw(func() {
				if err != nil {
					a.helpText.SetText(fmt.Sprintf("[red]Error saving tags: %v", err))
					return
				}
				// Refresh search results to show new tags, and ensure focus is on results.
				// This must also be in a goroutine to prevent deadlock, as runSearch
				// itself will queue updates.
				go a.runSearch(currentSearchQuery, true)
			})
		}()
	})

	// When selection in the results table changes, update the side panel.
	a.results.SetSelectionChangedFunc(func(row, column int) {
		// row is 1-based, index is 0-based. row 0 is header.
		if row < 1 || row > len(a.currentFiles) {
			a.selectedFile = nil
			a.thumbnail.SetImage("")
			a.tagEditor.SetText("")
			return
		}

		selected := a.currentFiles[row-1]
		a.selectedFile = &selected
		a.tagEditor.SetText(strings.ReplaceAll(selected.Tags, ",", " "))

		// Debounce image loading: only load the image if the user pauses scrolling.
		currentRequestID := atomic.AddUint64(&a.imageLoadRequestID, 1)
		go func() {
			// Wait a short moment to see if another selection event occurs.
			time.Sleep(150 * time.Millisecond)
			// If no new selection has been made, proceed with loading the image.
			if atomic.LoadUint64(&a.imageLoadRequestID) == currentRequestID {
				a.thumbnail.SetImage(selected.Path)
			}
		}()
	})

	// Global key handler for focus cycling and quitting.
	focusableWidgets := []tview.Primitive{a.input, a.results, a.tagEditor}
	currentFocusIndex := 0
	a.tviewApp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			currentFocusIndex = (currentFocusIndex + 1) % len(focusableWidgets)
			a.tviewApp.SetFocus(focusableWidgets[currentFocusIndex])
			return nil
		case tcell.KeyBacktab:
			currentFocusIndex = (currentFocusIndex - 1 + len(focusableWidgets)) % len(focusableWidgets)
			a.tviewApp.SetFocus(focusableWidgets[currentFocusIndex])
			return nil
		}

		if event.Rune() == 'q' && a.tviewApp.GetFocus() != a.input && a.tviewApp.GetFocus() != a.tagEditor {
			a.tviewApp.Stop()
			return nil
		}
		return event
	})
}

func (a *App) runSearch(query string, setFocusOnResults bool) {
	headers := []string{"PATH", "SIZE", "TAGS"}
	expansions := []int{5, 1, 4}

	// ---- UI Update: Phase 1 (Immediate Feedback) ----
	// This part is queued to run on the main UI thread, making this function safe
	// to call from any goroutine.
	a.tviewApp.QueueUpdateDraw(func() {
		a.results.Clear()
		a.selectedFile = nil
		a.currentFiles = nil
		a.thumbnail.SetImage("")
		a.tagEditor.SetText("")

		for i, header := range headers {
			cell := tview.NewTableCell(header).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetSelectable(false).
				SetExpansion(expansions[i])
			a.results.SetCell(0, i, cell)
		}
		a.results.SetCell(1, 0, tview.NewTableCell("Searching...").
			SetTextColor(tcell.ColorGray).
			SetExpansion(1).SetMaxWidth(0))
	})

	// ---- Background Task: Blocking I/O ----
	// The database query runs in a separate goroutine to not block the UI.
	go func() {
		var files []types.FileInfo
		var err error

		if strings.TrimSpace(query) == "" {
			files, err = a.gooruClient.GetAllFilesInfo()
		} else {
			files, err = a.gooruClient.GetFilesInfoByQuery(query, false)
		}

		// ---- UI Update: Phase 2 (Display Results) ----
		// This part is scheduled to run back on the main UI thread.
		a.tviewApp.QueueUpdateDraw(func() {
			a.results.Clear() // Clear the "Searching..." message
			for i, header := range headers { // Re-add headers
				cell := tview.NewTableCell(header).SetTextColor(tcell.ColorYellow).SetAlign(tview.AlignLeft).SetSelectable(false).SetExpansion(expansions[i])
				a.results.SetCell(0, i, cell)
			}

			if err != nil {
				a.results.SetCell(1, 0, tview.NewTableCell(fmt.Sprintf("Error: %v", err)).
					SetTextColor(tcell.ColorRed).
					SetExpansion(1).SetMaxWidth(0))
				return
			}

			a.currentFiles = files // Cache the results

			if len(files) == 0 {
				a.results.SetCell(1, 0, tview.NewTableCell("No results found.").
					SetTextColor(tcell.ColorGray).
					SetExpansion(1).SetMaxWidth(0))
			} else {
				for row, file := range files {
					rowData := []string{file.Path, humanReadableSize(file.Size), strings.ReplaceAll(file.Tags, ",", ", ")}
					for col, data := range rowData {
						cell := tview.NewTableCell(data).SetExpansion(expansions[col])
						a.results.SetCell(row+1, col, cell)
					}
				}
				a.results.Select(1, 0)
			}

			if setFocusOnResults {
				a.tviewApp.SetFocus(a.results)
			}
		})
		// ------------------------------------------------
	}()
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
	// Kick off the initial search in a background goroutine.
	// It will schedule UI updates via QueueUpdateDraw once the app is running.
	go a.runSearch("", false)

	// Set initial focus and run the application's main event loop.
	return a.tviewApp.SetFocus(a.input).Run()
}