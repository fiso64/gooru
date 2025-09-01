package tui

import (
	"image"
	"os"
	"sync"

	// Import decoders for common image formats
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/gdamore/tcell/v2"
	"github.com/nfnt/resize"
	"github.com/rivo/tview"
)

// Cell holds the data for a single character cell in the terminal.
type Cell struct {
	Char  rune
	Style tcell.Style
}

// ImageView is a tview primitive that displays an image using half-block characters.
type ImageView struct {
	*tview.Box
	mu            sync.Mutex
	originalImage image.Image // The full-resolution image.
	cellCache     [][]Cell    // A cache of the fully rendered cells.
	isRendering   bool        // A flag to prevent launching multiple render goroutines.
	app           *tview.Application
}

// NewImageView returns a new ImageView.
func NewImageView(app *tview.Application) *ImageView {
	return &ImageView{
		Box: tview.NewBox(),
		app: app,
	}
}

// SetImage loads an image from a file path in a separate goroutine and triggers a redraw.
func (iv *ImageView) SetImage(path string) {
	// Spawn a goroutine to handle slow file I/O.
	go func() {
		var img image.Image // Will be nil if loading fails.
		if path != "" {
			file, err := os.Open(path)
			if err == nil {
				defer file.Close()
				img, _, _ = image.Decode(file) // Silently ignore decode errors.
			}
		}

		// After I/O is done, queue a fast state update on the main UI thread.
		iv.app.QueueUpdateDraw(func() {
			iv.mu.Lock()
			// Set the new image and invalidate the cache. The actual rendering
			// will be deferred until the next Draw() call.
			iv.originalImage = img
			iv.cellCache = nil
			iv.mu.Unlock()
		})
	}()
}

// render calculates the terminal cells for a given image and dimensions.
// This is a pure function; it does not modify the ImageView state.
func renderToCells(img image.Image, width, height int) [][]Cell {
	// Step 1: Resize (expensive)
	viewW, viewH := uint(width), uint(height*2)
	origBounds := img.Bounds()
	origW, origH := uint(origBounds.Dx()), uint(origBounds.Dy())
	var newW, newH uint
	if origW > 0 && origH > 0 {
		if float64(origW)/float64(origH) > float64(viewW)/float64(viewH) {
			newW, newH = viewW, uint(float64(origH)*(float64(viewW)/float64(origW)))
		} else {
			newH, newW = viewH, uint(float64(origW)*(float64(viewH)/float64(origH)))
		}
	}
	if newW == 0 {
		newW = 1
	}
	if newH == 0 {
		newH = 1
	}
	resized := resize.Resize(newW, newH, img, resize.Lanczos3)
	bounds, imgW, imgH := resized.Bounds(), resized.Bounds().Dx(), resized.Bounds().Dy()

	// Step 2: Calculate all cell styles (also expensive)
	imgCellHeight := (imgH + 1) / 2
	newCache := make([][]Cell, imgCellHeight)
	for r := 0; r < imgCellHeight; r++ {
		newCache[r] = make([]Cell, imgW)
		for c := 0; c < imgW; c++ {
			topColor, bottomColor := tcell.ColorDefault, tcell.ColorDefault
			if (r * 2) < imgH {
				pr, pg, pb, _ := resized.At(bounds.Min.X+c, bounds.Min.Y+r*2).RGBA()
				topColor = tcell.NewRGBColor(int32(pr>>8), int32(pg>>8), int32(pb>>8))
			}
			if (r*2 + 1) < imgH {
				pr, pg, pb, _ := resized.At(bounds.Min.X+c, bounds.Min.Y+r*2+1).RGBA()
				bottomColor = tcell.NewRGBColor(int32(pr>>8), int32(pg>>8), int32(pb>>8))
			}
			newCache[r][c] = Cell{
				Char:  '▀',
				Style: tcell.StyleDefault.Foreground(topColor).Background(bottomColor),
			}
		}
	}
	return newCache
}

// Draw renders the image. It draws from a cache and triggers a background
// re-render if the cache is invalid. This method is fast and will not deadlock.
func (iv *ImageView) Draw(screen tcell.Screen) {
	iv.Box.Draw(screen)
	iv.mu.Lock()

	boxX, boxY, boxWidth, boxHeight := iv.GetInnerRect()

	// If there's no image, show the placeholder.
	if iv.originalImage == nil {
		iv.mu.Unlock()
		if boxWidth > 0 && boxHeight > 0 {
			tview.Print(screen, "No preview", boxX, boxY+(boxHeight/2), boxWidth, tview.AlignCenter, tcell.ColorGray)
		}
		return
	}

	// If the cache is invalid and we are not already rendering,
	// start a background rendering job.
	if iv.cellCache == nil && !iv.isRendering {
		if boxWidth > 0 && boxHeight > 0 {
			iv.isRendering = true
			imgToRender := iv.originalImage

			// Launch the slow rendering in a goroutine.
			go func() {
				newCache := renderToCells(imgToRender, boxWidth, boxHeight)
				// Queue the result to be applied on the main thread.
				iv.app.QueueUpdateDraw(func() {
					iv.mu.Lock()
					// Only update if the image hasn't changed in the meantime.
					if iv.originalImage == imgToRender {
						iv.cellCache = newCache
					}
					iv.isRendering = false
					iv.mu.Unlock()
				})
			}()
		}
	}

	// Draw from the cache if it's available.
	if iv.cellCache != nil {
		imgCellHeight := len(iv.cellCache)
		if imgCellHeight > 0 {
			imgW := len(iv.cellCache[0])
			offsetX := (boxWidth - imgW) / 2
			offsetY := (boxHeight - imgCellHeight) / 2

			for r, rowData := range iv.cellCache {
				y := boxY + offsetY + r
				if y < boxY || y >= boxY+boxHeight {
					continue
				}
				for c, cellData := range rowData {
					x := boxX + offsetX + c
					if x < boxX || x >= boxX+boxWidth {
						continue
					}
					screen.SetContent(x, y, cellData.Char, nil, cellData.Style)
				}
			}
		}
	} else {
		// If cache is not ready yet, we can draw a "Loading..." message.
		tview.Print(screen, "Loading...", boxX, boxY+(boxHeight/2), boxWidth, tview.AlignCenter, tcell.ColorGray)
	}
	iv.mu.Unlock()
}