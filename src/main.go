package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const (
	appTitle      = "kjagave"
	appVersion    = "20260315-0200"
	maxHistoryLen = 250
	cardImageW    = 160
	cardImageH    = 132
)

type SavedColor struct {
	Hex  string `json:"hex"`
	Name string `json:"name"`
}

type AppConfig struct {
	Favorites  []SavedColor `json:"favorites"`
	LastColor  string       `json:"lastColor"`
	LastScheme string       `json:"lastScheme"`
	Palette    string       `json:"palette"`
}

type SwatchCard struct {
	button *gtk.Button
	image  *gtk.Image
	label  *gtk.Label
	hex    string
	rgb    string
	hsv    string
}

type App struct {
	window *gtk.Window
	css    *gtk.CssProvider

	colorButton  *gtk.ColorButton
	hexEntry     *gtk.Entry
	schemeCombo  *gtk.ComboBoxText
	paletteCombo *gtk.ComboBoxText

	historyBackBtn *gtk.Button
	historyFwdBtn  *gtk.Button
	lightenBtn     *gtk.Button
	darkenBtn      *gtk.Button
	saturateBtn    *gtk.Button
	desaturateBtn  *gtk.Button

	swatchCards []SwatchCard

	paletteGrid   *gtk.Grid
	paletteScroll *gtk.ScrolledWindow

	favoritesStore *gtk.ListStore
	favoritesView  *gtk.TreeView
	renameFavBtn   *gtk.Button
	removeFavBtn   *gtk.Button
	clearFavBtn    *gtk.Button
	selectedIter   *gtk.TreeIter

	currentColor *gdk.RGBA
	currentHex   string

	savedColors []SavedColor
	history     []string
	historyPos  int

	configFile string
	config     AppConfig

	suppressColorSet bool
	rng              *rand.Rand
}

var schemeNames = []string{
	"Triads",
	"Complements",
	"Split Complements",
	"Tetrads",
	"Analogous",
	"Monochromatic",
}

var paletteNames = []string{
	"Web-safe (legacy)",
	"Material Design",
	"Tailwind CSS",
	"Flat UI",
	"Pastel",
	"Nord",
	"Dracula",
	"Solarized",
	"Gruvbox",
	"One Dark",
	"Monokai",
	"KiJiSH Dark Pastel Terminal",
}

func main() {
	gtk.Init(nil)

	configDir := filepath.Join(os.Getenv("HOME"), ".config")
	_ = os.MkdirAll(configDir, 0755)

	app := &App{
		configFile: filepath.Join(configDir, "kjagave.json"),
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
		historyPos: -1,
	}

	app.loadConfig()
	app.createUI()
	app.refreshFavoritesView()
	app.restoreStartupState()
	app.populatePaletteGrid()
	app.updateSchemePreview()
	app.updateActionStates()

	gtk.Main()
}

func (app *App) createUI() {
	var err error
	app.window, err = gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		log.Fatal("Unable to create window:", err)
	}
	app.window.SetTitle(appTitle)
	app.window.SetDefaultSize(640, 480)
	app.window.Connect("destroy", func() {
		app.saveConfig()
		gtk.MainQuit()
	})

	root, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 2)
	root.SetMarginTop(2)
	root.SetMarginBottom(2)
	root.SetMarginStart(2)
	root.SetMarginEnd(2)

	menuBar := app.buildMenuBar()
	root.PackStart(menuBar, false, false, 0)
	app.initCompactButtonCSS()

	toolbar, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 3)
	app.historyBackBtn, _ = gtk.ButtonNewWithLabel("Back")
	app.setButtonIcon(app.historyBackBtn, "go-previous")
	app.historyBackBtn.Connect("clicked", func() { app.navigateHistory(-1) })
	toolbar.PackStart(app.historyBackBtn, false, false, 0)

	app.historyFwdBtn, _ = gtk.ButtonNewWithLabel("Forward")
	app.setButtonIcon(app.historyFwdBtn, "go-next")
	app.historyFwdBtn.Connect("clicked", func() { app.navigateHistory(1) })
	toolbar.PackStart(app.historyFwdBtn, false, false, 0)

	randomBtn, _ := gtk.ButtonNewWithLabel("Random")
	app.setButtonIcon(randomBtn, "view-refresh")
	randomBtn.Connect("clicked", func() { app.randomizeColor() })
	toolbar.PackStart(randomBtn, false, false, 0)

	app.lightenBtn, _ = gtk.ButtonNewWithLabel("Lighten")
	app.setButtonIcon(app.lightenBtn, "go-up")
	app.lightenBtn.Connect("clicked", func() { app.adjustSV(0, 5) })
	toolbar.PackStart(app.lightenBtn, false, false, 0)

	app.darkenBtn, _ = gtk.ButtonNewWithLabel("Darken")
	app.setButtonIcon(app.darkenBtn, "go-down")
	app.darkenBtn.Connect("clicked", func() { app.adjustSV(0, -5) })
	toolbar.PackStart(app.darkenBtn, false, false, 0)

	app.saturateBtn, _ = gtk.ButtonNewWithLabel("Saturate")
	app.setButtonIcon(app.saturateBtn, "list-add")
	app.saturateBtn.Connect("clicked", func() { app.adjustSV(5, 0) })
	toolbar.PackStart(app.saturateBtn, false, false, 0)

	app.desaturateBtn, _ = gtk.ButtonNewWithLabel("Desaturate")
	app.setButtonIcon(app.desaturateBtn, "list-remove")
	app.desaturateBtn.Connect("clicked", func() { app.adjustSV(-5, 0) })
	toolbar.PackStart(app.desaturateBtn, false, false, 0)

	pasteBtn, _ := gtk.ButtonNewWithLabel("Paste")
	app.setButtonIcon(pasteBtn, "edit-paste")
	pasteBtn.Connect("clicked", func() { app.pasteColorFromClipboard() })
	toolbar.PackStart(pasteBtn, false, false, 0)

	aboutBtn, _ := gtk.ButtonNewWithLabel("About")
	app.setButtonIcon(aboutBtn, "help-about")
	aboutBtn.Connect("clicked", func() { app.onAboutClicked() })
	toolbar.PackStart(aboutBtn, false, false, 0)

	root.PackStart(toolbar, false, false, 0)

	schemeRow, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 2)
	app.swatchCards = make([]SwatchCard, 0, 4)
	for i := 0; i < 4; i++ {
		card := app.newSwatchCard()
		cardIdx := i
		card.button.Connect("button-press-event", func(_ *gtk.Button, ev *gdk.Event) bool {
			if ev == nil {
				return false
			}
			evBtn := gdk.EventButtonNewFromEvent(ev)
			if evBtn == nil || evBtn.Button() != 3 {
				return false
			}
			app.showSwatchContextMenu(cardIdx, ev)
			return true
		})
		card.button.Connect("clicked", func() {
			hex := app.swatchCards[cardIdx].hex
			if hex == "" {
				return
			}
			rgba := gdk.NewRGBA()
			if !rgba.Parse(hex) {
				return
			}
			app.setCurrentColor(rgba, true)
		})
		app.swatchCards = append(app.swatchCards, card)
		schemeRow.PackStart(card.button, true, true, 0)
	}
	root.PackStart(schemeRow, false, false, 0)

	controlRow, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
	app.currentColor = gdk.NewRGBA()
	app.currentColor.Parse("#0066FF")

	app.colorButton, err = gtk.ColorButtonNewWithRGBA(app.currentColor)
	if err != nil {
		log.Fatal("Unable to create color button:", err)
	}
	app.colorButton.SetUseAlpha(false)
	app.colorButton.SetTitle("Pick a Color")
	app.colorButton.Connect("color-set", func() {
		if app.suppressColorSet {
			return
		}
		app.setCurrentColor(app.colorButton.GetRGBA(), true)
	})
	controlRow.PackStart(app.colorButton, false, false, 0)

	app.schemeCombo, _ = gtk.ComboBoxTextNew()
	for _, schemeName := range schemeNames {
		app.schemeCombo.AppendText(schemeName)
	}
	app.schemeCombo.SetActive(0)
	app.schemeCombo.Connect("changed", func() {
		app.updateSchemePreview()
		app.saveConfig()
	})
	controlRow.PackStart(app.schemeCombo, false, false, 0)

	app.hexEntry, _ = gtk.EntryNew()
	app.hexEntry.SetWidthChars(11)
	app.hexEntry.Connect("activate", func() { app.applyHexEntry() })
	controlRow.PackStart(app.hexEntry, false, false, 0)

	pickBtn, _ := gtk.ButtonNewWithLabel("Pick from Screen")
	app.setButtonIcon(pickBtn, "color-select")
	pickBtn.Connect("clicked", func() {
		clr, err := app.pickColorFromScreen()
		if err == nil {
			app.setCurrentColor(clr, true)
		}
	})
	controlRow.PackStart(pickBtn, false, false, 0)

	copyBtn, _ := gtk.ButtonNewWithLabel("Copy")
	app.setButtonIcon(copyBtn, "edit-copy")
	copyBtn.Connect("clicked", func() {
		clipboard, _ := gtk.ClipboardGet(gdk.SELECTION_CLIPBOARD)
		clipboard.SetText(app.currentHex)
	})
	controlRow.PackStart(copyBtn, false, false, 0)

	root.PackStart(controlRow, false, false, 0)

	lower, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)

	paletteFrame, _ := gtk.FrameNew("Palette")
	paletteBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 2)
	paletteBox.SetMarginTop(2)
	paletteBox.SetMarginBottom(2)
	paletteBox.SetMarginStart(2)
	paletteBox.SetMarginEnd(2)

	app.paletteScroll, _ = gtk.ScrolledWindowNew(nil, nil)
	app.paletteScroll.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	app.paletteScroll.SetSizeRequest(430, 150)
	app.paletteGrid, _ = gtk.GridNew()
	app.paletteGrid.SetRowSpacing(0)
	app.paletteGrid.SetColumnSpacing(0)
	app.paletteScroll.Add(app.paletteGrid)
	paletteBox.PackStart(app.paletteScroll, true, true, 0)

	app.paletteCombo, _ = gtk.ComboBoxTextNew()
	for _, name := range paletteNames {
		app.paletteCombo.AppendText(name)
	}
	app.paletteCombo.SetActive(0)
	app.paletteCombo.Connect("changed", func() {
		app.populatePaletteGrid()
		app.saveConfig()
	})
	paletteBox.PackStart(app.paletteCombo, false, false, 0)
	paletteFrame.Add(paletteBox)
	lower.PackStart(paletteFrame, true, true, 0)

	favFrame, _ := gtk.FrameNew("Favorites")
	favBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 2)
	favBox.SetMarginTop(2)
	favBox.SetMarginBottom(2)
	favBox.SetMarginStart(2)
	favBox.SetMarginEnd(2)

	favScroll, _ := gtk.ScrolledWindowNew(nil, nil)
	favScroll.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	favScroll.SetSizeRequest(230, 180)

	app.favoritesStore, _ = gtk.ListStoreNew(gdk.PixbufGetType(), glib.TYPE_STRING, glib.TYPE_STRING)
	app.favoritesView, _ = gtk.TreeViewNew()
	app.favoritesView.SetModel(app.favoritesStore)
	app.favoritesView.SetHeadersVisible(false)

	favCol, _ := gtk.TreeViewColumnNew()
	favPix, _ := gtk.CellRendererPixbufNew()
	favCol.PackStart(favPix, false)
	favCol.AddAttribute(favPix, "pixbuf", 0)
	favHexText, _ := gtk.CellRendererTextNew()
	favCol.PackStart(favHexText, true)
	favCol.AddAttribute(favHexText, "text", 1)
	app.favoritesView.AppendColumn(favCol)

	favNameText, _ := gtk.CellRendererTextNew()
	favNameCol, _ := gtk.TreeViewColumnNewWithAttribute("", favNameText, "text", 2)
	app.favoritesView.AppendColumn(favNameCol)

	favSel, _ := app.favoritesView.GetSelection()
	favSel.SetMode(gtk.SELECTION_SINGLE)
	favSel.Connect("changed", app.onFavoriteSelectionChanged)
	app.favoritesView.Connect("row-activated", func() {
		app.renameSelectedFavorite()
	})

	favScroll.Add(app.favoritesView)
	favBox.PackStart(favScroll, true, true, 0)

	favBtns, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	addFavBtn, _ := gtk.ButtonNewWithLabel("+")
	app.setButtonIcon(addFavBtn, "list-add")
	addFavBtn.Connect("clicked", func() { app.addCurrentToFavorites() })
	favBtns.PackStart(addFavBtn, true, true, 0)

	app.removeFavBtn, _ = gtk.ButtonNewWithLabel("-")
	app.setButtonIcon(app.removeFavBtn, "list-remove")
	app.removeFavBtn.Connect("clicked", func() { app.removeSelectedFavorite() })
	favBtns.PackStart(app.removeFavBtn, true, true, 0)

	app.renameFavBtn, _ = gtk.ButtonNewWithLabel("Rename")
	app.setButtonIcon(app.renameFavBtn, "document-edit")
	app.renameFavBtn.Connect("clicked", func() { app.renameSelectedFavorite() })
	favBtns.PackStart(app.renameFavBtn, true, true, 0)

	app.clearFavBtn, _ = gtk.ButtonNewWithLabel("Clear")
	app.setButtonIcon(app.clearFavBtn, "edit-clear")
	app.clearFavBtn.Connect("clicked", func() {
		app.savedColors = nil
		app.refreshFavoritesView()
		app.saveConfig()
	})
	favBtns.PackStart(app.clearFavBtn, true, true, 0)

	favBox.PackStart(favBtns, false, false, 0)
	favFrame.Add(favBox)
	lower.PackStart(favFrame, false, false, 0)

	root.PackStart(lower, true, true, 0)

	status, _ := gtk.LabelNew("Choose a color and a scheme type")
	status.SetHAlign(gtk.ALIGN_START)
	root.PackStart(status, false, false, 0)

	app.window.Add(root)
	app.window.ShowAll()
}

func (app *App) buildMenuBar() *gtk.MenuBar {
	menuBar, _ := gtk.MenuBarNew()

	fileTop, _ := gtk.MenuItemNewWithLabel("File")
	fileMenu, _ := gtk.MenuNew()
	random, _ := gtk.MenuItemNewWithLabel("Random")
	random.Connect("activate", func() { app.randomizeColor() })
	fileMenu.Append(random)
	quit, _ := gtk.MenuItemNewWithLabel("Quit")
	quit.Connect("activate", func() {
		app.saveConfig()
		gtk.MainQuit()
	})
	fileMenu.Append(quit)
	fileTop.SetSubmenu(fileMenu)
	menuBar.Append(fileTop)

	editTop, _ := gtk.MenuItemNewWithLabel("Edit")
	editMenu, _ := gtk.MenuNew()
	paste, _ := gtk.MenuItemNewWithLabel("Paste")
	paste.Connect("activate", func() { app.pasteColorFromClipboard() })
	editMenu.Append(paste)
	editTop.SetSubmenu(editMenu)
	menuBar.Append(editTop)

	favTop, _ := gtk.MenuItemNewWithLabel("Favorites")
	favMenu, _ := gtk.MenuNew()
	add, _ := gtk.MenuItemNewWithLabel("Add Current")
	add.Connect("activate", func() { app.addCurrentToFavorites() })
	favMenu.Append(add)
	rename, _ := gtk.MenuItemNewWithLabel("Rename Selected")
	rename.Connect("activate", func() { app.renameSelectedFavorite() })
	favMenu.Append(rename)
	favTop.SetSubmenu(favMenu)
	menuBar.Append(favTop)

	helpTop, _ := gtk.MenuItemNewWithLabel("Help")
	helpMenu, _ := gtk.MenuNew()
	about, _ := gtk.MenuItemNewWithLabel("About")
	about.Connect("activate", func() { app.onAboutClicked() })
	helpMenu.Append(about)
	helpTop.SetSubmenu(helpMenu)
	menuBar.Append(helpTop)

	return menuBar
}

func (app *App) initCompactButtonCSS() {
	app.css, _ = gtk.CssProviderNew()
	css := "button { padding: 1px 4px; min-height: 0; min-width: 0; } .palette-swatch { padding: 0; border-width: 0; border-radius: 0; } .swatch-overlay-label { text-shadow: none; }"
	_ = app.css.LoadFromData(css)
	if screen, err := gdk.ScreenGetDefault(); err == nil && screen != nil {
		gtk.AddProviderForScreen(screen, app.css, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
	}
}

func (app *App) setButtonIcon(btn *gtk.Button, iconName string) {
	img, err := gtk.ImageNewFromIconName(iconName, gtk.ICON_SIZE_BUTTON)
	if err != nil || img == nil {
		return
	}
	if label, err := btn.GetLabel(); err == nil && strings.TrimSpace(label) != "" {
		btn.SetTooltipText(label)
		btn.SetLabel("")
	}
	btn.SetImage(img)
	btn.SetAlwaysShowImage(true)
}

func (app *App) newSwatchCard() SwatchCard {
	button, _ := gtk.ButtonNew()
	button.SetSizeRequest(166, 138)

	vbox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	vbox.SetMarginTop(1)
	vbox.SetMarginBottom(1)
	vbox.SetMarginStart(1)
	vbox.SetMarginEnd(1)

	overlay, _ := gtk.OverlayNew()
	overlay.SetHExpand(true)
	overlay.SetVExpand(true)

	image, _ := gtk.ImageNew()
	image.SetFromPixbuf(solidPixbuf("#000000", cardImageW, cardImageH))
	overlay.Add(image)

	label, _ := gtk.LabelNew("")
	label.SetJustify(gtk.JUSTIFY_CENTER)
	label.SetHAlign(gtk.ALIGN_CENTER)
	label.SetVAlign(gtk.ALIGN_CENTER)
	if ctx, err := label.GetStyleContext(); err == nil {
		ctx.AddClass("swatch-overlay-label")
	}
	overlay.AddOverlay(label)
	vbox.PackStart(overlay, true, true, 0)

	button.Add(vbox)

	return SwatchCard{button: button, image: image, label: label}
}

func (app *App) setCurrentColor(rgba *gdk.RGBA, pushHistory bool) {
	hex := rgbaToHex(rgba)
	app.currentColor = rgba
	app.currentHex = hex
	app.hexEntry.SetText(hex)

	app.suppressColorSet = true
	app.colorButton.SetRGBA(rgba)
	app.suppressColorSet = false

	if pushHistory {
		app.pushHistory(hex)
	}

	app.config.LastColor = hex
	app.config.LastScheme = app.activeSchemeName()
	app.config.Palette = app.activePaletteName()
	app.updateSchemePreview()
	app.updateActionStates()
	app.saveConfig()
}

func (app *App) updateSchemePreview() {
	colors := generateScheme(app.currentColor, app.activeSchemeName())
	for i := 0; i < len(app.swatchCards); i++ {
		if i >= len(colors) {
			app.swatchCards[i].hex = ""
			app.swatchCards[i].rgb = ""
			app.swatchCards[i].hsv = ""
			app.swatchCards[i].button.Hide()
			continue
		}
		app.swatchCards[i].button.Show()
		app.swatchCards[i].button.SetSensitive(true)
		hex := rgbaToHex(colors[i])
		h, s, v := rgbToHSV(colors[i])
		r := int(math.Round(colors[i].GetRed() * 255))
		g := int(math.Round(colors[i].GetGreen() * 255))
		b := int(math.Round(colors[i].GetBlue() * 255))
		textColor := "#F5F5F5"
		if luminance(colors[i]) > 0.53 {
			textColor = "#111111"
		}
		rgbText := fmt.Sprintf("rgb(%d, %d, %d)", r, g, b)
		hsvText := fmt.Sprintf("hsv(%d, %d, %d)", int(h), int(s), int(v))
		app.swatchCards[i].hex = hex
		app.swatchCards[i].rgb = rgbText
		app.swatchCards[i].hsv = hsvText
		app.swatchCards[i].image.SetFromPixbuf(solidPixbuf(hex, cardImageW, cardImageH))
		app.swatchCards[i].label.SetMarkup(fmt.Sprintf("<span foreground=\"%s\" size=\"9000\"><b>%s</b>\n%s\n%s</span>", textColor, hex, rgbText, hsvText))
	}
}

func (app *App) copyTextToClipboard(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	clipboard, _ := gtk.ClipboardGet(gdk.SELECTION_CLIPBOARD)
	clipboard.SetText(text)
}

func (app *App) showSwatchContextMenu(cardIdx int, ev *gdk.Event) {
	if cardIdx < 0 || cardIdx >= len(app.swatchCards) {
		return
	}
	card := app.swatchCards[cardIdx]
	if card.hex == "" {
		return
	}
	app.showColorContextMenu(card.hex, card.rgb, card.hsv, ev)
}

func (app *App) showColorContextMenu(hex, rgbText, hsvText string, ev *gdk.Event) {
	if strings.TrimSpace(hex) == "" {
		return
	}

	menu, _ := gtk.MenuNew()
	copyHex, _ := gtk.MenuItemNewWithLabel("Copy HEX")
	copyHex.Connect("activate", func() {
		app.copyTextToClipboard(hex)
	})
	menu.Append(copyHex)

	copyHSV, _ := gtk.MenuItemNewWithLabel("Copy HSV")
	copyHSV.Connect("activate", func() {
		app.copyTextToClipboard(hsvText)
	})
	menu.Append(copyHSV)

	copyRGB, _ := gtk.MenuItemNewWithLabel("Copy RGB (RGV)")
	copyRGB.Connect("activate", func() {
		app.copyTextToClipboard(rgbText)
	})
	menu.Append(copyRGB)

	menu.ShowAll()
	menu.PopupAtPointer(ev)
}

func colorStringsFromHex(hex string) (string, string) {
	rgba := gdk.NewRGBA()
	if !rgba.Parse(hex) {
		return "", ""
	}
	r := int(math.Round(rgba.GetRed() * 255))
	g := int(math.Round(rgba.GetGreen() * 255))
	b := int(math.Round(rgba.GetBlue() * 255))
	h, s, v := rgbToHSV(rgba)
	return fmt.Sprintf("rgb(%d, %d, %d)", r, g, b), fmt.Sprintf("hsv(%d, %d, %d)", int(h), int(s), int(v))
}

func (app *App) applyHexEntry() {
	text, _ := app.hexEntry.GetText()
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if !strings.HasPrefix(text, "#") {
		text = "#" + text
	}
	rgba := gdk.NewRGBA()
	if !rgba.Parse(text) {
		app.hexEntry.SetText(app.currentHex)
		return
	}
	app.setCurrentColor(rgba, true)
}

func (app *App) randomizeColor() {
	r := app.rng.Intn(256)
	g := app.rng.Intn(256)
	b := app.rng.Intn(256)
	rgba := gdk.NewRGBA()
	rgba.SetRed(float64(r) / 255.0)
	rgba.SetGreen(float64(g) / 255.0)
	rgba.SetBlue(float64(b) / 255.0)
	rgba.SetAlpha(1)
	app.setCurrentColor(rgba, true)
}

func (app *App) adjustSV(ds, dv float64) {
	h, s, v := rgbToHSV(app.currentColor)
	s = clamp(s+ds, 0, 100)
	v = clamp(v+dv, 0, 100)
	app.setCurrentColor(hsvToRGBA(h, s, v), true)
}

func (app *App) pasteColorFromClipboard() {
	clipboard, _ := gtk.ClipboardGet(gdk.SELECTION_CLIPBOARD)
	text, err := clipboard.WaitForText()
	if err != nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if !strings.HasPrefix(text, "#") {
		text = "#" + text
	}
	rgba := gdk.NewRGBA()
	if rgba.Parse(text) {
		app.setCurrentColor(rgba, true)
	}
}

func (app *App) pushHistory(hex string) {
	if app.historyPos >= 0 && app.historyPos < len(app.history) && app.history[app.historyPos] == hex {
		return
	}
	if app.historyPos+1 < len(app.history) {
		app.history = app.history[:app.historyPos+1]
	}
	app.history = append(app.history, hex)
	if len(app.history) > maxHistoryLen {
		over := len(app.history) - maxHistoryLen
		app.history = app.history[over:]
	}
	app.historyPos = len(app.history) - 1
}

func (app *App) navigateHistory(step int) {
	if len(app.history) == 0 {
		return
	}
	next := app.historyPos + step
	if next < 0 || next >= len(app.history) {
		return
	}
	rgba := gdk.NewRGBA()
	if !rgba.Parse(app.history[next]) {
		return
	}
	app.historyPos = next
	app.setCurrentColor(rgba, false)
}

func (app *App) updateActionStates() {
	_, s, v := rgbToHSV(app.currentColor)
	app.historyBackBtn.SetSensitive(app.historyPos > 0)
	app.historyFwdBtn.SetSensitive(app.historyPos >= 0 && app.historyPos < len(app.history)-1)
	app.lightenBtn.SetSensitive(v < 100)
	app.darkenBtn.SetSensitive(v > 5)
	app.saturateBtn.SetSensitive(s < 100)
	app.desaturateBtn.SetSensitive(s > 5)
	app.renameFavBtn.SetSensitive(app.selectedIter != nil)
	app.removeFavBtn.SetSensitive(app.selectedIter != nil)
	app.clearFavBtn.SetSensitive(len(app.savedColors) > 0)
}

func (app *App) activeSchemeName() string {
	idx := app.schemeCombo.GetActive()
	if idx < 0 || idx >= len(schemeNames) {
		return schemeNames[0]
	}
	return schemeNames[idx]
}

func (app *App) activePaletteName() string {
	idx := app.paletteCombo.GetActive()
	if idx < 0 || idx >= len(paletteNames) {
		return paletteNames[0]
	}
	return paletteNames[idx]
}

func (app *App) onFavoriteSelectionChanged(selection *gtk.TreeSelection) {
	model, iter, ok := selection.GetSelected()
	if !ok {
		app.selectedIter = nil
		app.updateActionStates()
		return
	}
	app.selectedIter = iter
	value, _ := model.ToTreeModel().GetValue(iter, 1)
	hex, _ := value.GetString()
	rgba := gdk.NewRGBA()
	if rgba.Parse(hex) {
		app.setCurrentColor(rgba, true)
	}
	app.updateActionStates()
}

func (app *App) populatePaletteGrid() {
	children := app.paletteGrid.GetChildren()
	children.Foreach(func(item interface{}) {
		if widget, ok := item.(*gtk.Widget); ok {
			app.paletteGrid.Remove(widget)
		}
	})

	colors := paletteByName(app.activePaletteName())
	cols := 24
	for i, hex := range colors {
		btn, _ := gtk.ButtonNew()
		btn.SetRelief(gtk.RELIEF_NONE)
		btn.SetCanFocus(false)
		btn.SetSizeRequest(16, 11)
		if ctx, err := btn.GetStyleContext(); err == nil {
			ctx.AddClass("palette-swatch")
		}
		img, _ := gtk.ImageNewFromPixbuf(solidPixbuf(hex, 16, 11))
		btn.Add(img)
		h := hex
		btn.Connect("button-press-event", func(_ *gtk.Button, ev *gdk.Event) bool {
			if ev == nil {
				return false
			}
			evBtn := gdk.EventButtonNewFromEvent(ev)
			if evBtn == nil || evBtn.Button() != 3 {
				return false
			}
			rgbText, hsvText := colorStringsFromHex(h)
			app.showColorContextMenu(h, rgbText, hsvText, ev)
			return true
		})
		btn.Connect("clicked", func() {
			rgba := gdk.NewRGBA()
			if rgba.Parse(h) {
				app.setCurrentColor(rgba, true)
			}
		})
		app.paletteGrid.Attach(btn, i%cols, i/cols, 1, 1)
	}
	app.paletteGrid.ShowAll()
}

func (app *App) addCurrentToFavorites() {
	for _, item := range app.savedColors {
		if strings.EqualFold(item.Hex, app.currentHex) {
			return
		}
	}
	app.savedColors = append([]SavedColor{{Hex: app.currentHex, Name: app.currentHex}}, app.savedColors...)
	app.refreshFavoritesView()
	app.saveConfig()
}

func (app *App) removeSelectedFavorite() {
	if app.selectedIter == nil {
		return
	}
	value, _ := app.favoritesStore.GetValue(app.selectedIter, 1)
	hex, _ := value.GetString()
	for i, c := range app.savedColors {
		if strings.EqualFold(c.Hex, hex) {
			app.savedColors = append(app.savedColors[:i], app.savedColors[i+1:]...)
			break
		}
	}
	app.selectedIter = nil
	app.refreshFavoritesView()
	app.saveConfig()
}

func (app *App) renameSelectedFavorite() {
	if app.selectedIter == nil {
		return
	}
	vHex, _ := app.favoritesStore.GetValue(app.selectedIter, 1)
	hex, _ := vHex.GetString()
	vName, _ := app.favoritesStore.GetValue(app.selectedIter, 2)
	currentName, _ := vName.GetString()

	dialog, _ := gtk.DialogNew()
	dialog.SetTitle("Rename Color")
	dialog.SetTransientFor(app.window)
	dialog.SetModal(true)
	box, _ := dialog.GetContentArea()
	box.SetMarginTop(10)
	box.SetMarginBottom(10)
	box.SetMarginStart(10)
	box.SetMarginEnd(10)
	box.SetSpacing(6)
	label, _ := gtk.LabelNew("Enter a new name:")
	label.SetHAlign(gtk.ALIGN_START)
	box.PackStart(label, false, false, 0)
	entry, _ := gtk.EntryNew()
	entry.SetText(currentName)
	entry.SetActivatesDefault(true)
	box.PackStart(entry, false, false, 0)
	dialog.AddButton("Cancel", gtk.RESPONSE_CANCEL)
	okBtn, _ := dialog.AddButton("OK", gtk.RESPONSE_OK)
	okBtn.SetCanDefault(true)
	okBtn.GrabDefault()
	dialog.ShowAll()
	if dialog.Run() == gtk.RESPONSE_OK {
		newName, _ := entry.GetText()
		newName = strings.TrimSpace(newName)
		if newName != "" {
			for i := range app.savedColors {
				if strings.EqualFold(app.savedColors[i].Hex, hex) {
					app.savedColors[i].Name = newName
					break
				}
			}
			app.refreshFavoritesView()
			app.saveConfig()
		}
	}
	dialog.Destroy()
}

func (app *App) refreshFavoritesView() {
	app.favoritesStore.Clear()
	for _, color := range app.savedColors {
		iter := app.favoritesStore.Append()
		app.favoritesStore.Set(iter, []int{0, 1, 2}, []interface{}{solidPixbuf(color.Hex, 16, 14), strings.ToUpper(color.Hex), color.Name})
	}
	app.updateActionStates()
}

func (app *App) onAboutClicked() {
	dialog, _ := gtk.AboutDialogNew()
	dialog.SetTransientFor(app.window)
	dialog.SetProgramName(appTitle)
	dialog.SetVersion(appVersion)
	dialog.SetComments("Agave-inspired GTK color scheme tool")
	dialog.SetAuthors([]string{"kj_sh604", "Agave inspiration: Jonathon Jongsma"})
	dialog.SetLicense("BSD Zero Clause License (0-clause BSD)")
	dialog.SetLogoIconName("applications-graphics")
	dialog.Run()
	dialog.Destroy()
}

func (app *App) pickColorFromScreen() (*gdk.RGBA, error) {
	cmd := exec.Command("xcolor", "--format", "hex")
	output, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("grabc")
		output, err = cmd.Output()
		if err != nil {
			dialog := gtk.MessageDialogNew(app.window, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR,
				gtk.BUTTONS_OK, "Color picker not found. Please install xcolor or grabc")
			dialog.Run()
			dialog.Destroy()
			return nil, err
		}
	}

	hex := strings.TrimSpace(string(output))
	if !strings.HasPrefix(hex, "#") {
		hex = "#" + hex
	}
	rgba := gdk.NewRGBA()
	if !rgba.Parse(hex) {
		return nil, fmt.Errorf("invalid color format: %s", hex)
	}
	return rgba, nil
}

func (app *App) loadConfig() {
	data, err := os.ReadFile(app.configFile)
	if err != nil {
		app.savedColors = []SavedColor{}
		app.config = AppConfig{Favorites: []SavedColor{}, LastColor: "#0066FF", LastScheme: "Triads", Palette: "Web-safe colors"}
		return
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err == nil && (cfg.LastColor != "" || len(cfg.Favorites) > 0) {
		app.config = cfg
		app.savedColors = append([]SavedColor(nil), cfg.Favorites...)
		if app.savedColors == nil {
			app.savedColors = []SavedColor{}
		}
		if app.config.LastColor == "" {
			app.config.LastColor = "#0066FF"
		}
		if app.config.LastScheme == "" {
			app.config.LastScheme = "Triads"
		}
		if app.config.Palette == "" {
			app.config.Palette = "Web-safe colors"
		}
		return
	}

	var legacy []SavedColor
	if err := json.Unmarshal(data, &legacy); err == nil {
		app.savedColors = legacy
		app.config = AppConfig{Favorites: legacy, LastColor: "#0066FF", LastScheme: "Triads", Palette: "Web-safe colors"}
		return
	}

	app.savedColors = []SavedColor{}
	app.config = AppConfig{Favorites: []SavedColor{}, LastColor: "#0066FF", LastScheme: "Triads", Palette: "Web-safe colors"}
}

func (app *App) saveConfig() {
	app.config.Favorites = app.savedColors
	if app.currentHex != "" {
		app.config.LastColor = app.currentHex
	}
	if app.schemeCombo != nil {
		app.config.LastScheme = app.activeSchemeName()
	}
	if app.paletteCombo != nil {
		app.config.Palette = app.activePaletteName()
	}

	data, err := json.MarshalIndent(app.config, "", "  ")
	if err != nil {
		log.Printf("Error marshaling config: %v", err)
		return
	}
	if err := os.WriteFile(app.configFile, data, 0644); err != nil {
		log.Printf("Error saving config: %v", err)
	}
}

func (app *App) restoreStartupState() {
	rgba := gdk.NewRGBA()
	if app.config.LastColor != "" && rgba.Parse(app.config.LastColor) {
		app.currentColor = rgba
	}

	for i, name := range schemeNames {
		if name == app.config.LastScheme {
			app.schemeCombo.SetActive(i)
			break
		}
	}
	for i, name := range paletteNames {
		if name == app.config.Palette {
			app.paletteCombo.SetActive(i)
			break
		}
	}

	app.setCurrentColor(app.currentColor, true)
}

func generateScheme(base *gdk.RGBA, schemeName string) []*gdk.RGBA {
	h, s, v := rgbToHSV(base)
	mk := func(hue float64) *gdk.RGBA {
		return hsvToRGBA(wrapHue(hue), s, v)
	}

	switch schemeName {
	case "Complements":
		return []*gdk.RGBA{hsvToRGBA(h, s, v), mk(h + 180)}
	case "Split Complements":
		offset := 360.0 / 15.0
		return []*gdk.RGBA{hsvToRGBA(h, s, v), mk(h + 180 - offset), mk(h + 180 + offset)}
	case "Tetrads":
		offset := 90.0
		return []*gdk.RGBA{hsvToRGBA(h, s, v), mk(h + offset), mk(h + 180), mk(h + 180 + offset)}
	case "Analogous":
		offset := 360.0 / 12.0
		return []*gdk.RGBA{mk(h - offset), hsvToRGBA(h, s, v), mk(h + offset)}
	case "Monochromatic":
		c0 := hsvToRGBA(h, s, v)
		c1 := hsvToRGBA(h, s, v)
		c2 := hsvToRGBA(h, s, v)
		if s < 10 {
			c1 = hsvToRGBA(h, math.Mod(s+33, 100), v)
			c2 = hsvToRGBA(h, math.Mod(s+66, 100), v)
		} else {
			c1 = hsvToRGBA(h, s, math.Mod(v+33, 100))
			c2 = hsvToRGBA(h, s, math.Mod(v+66, 100))
		}
		out := []*gdk.RGBA{c0, c1, c2}
		sort.Slice(out, func(i, j int) bool { return luminance(out[i]) < luminance(out[j]) })
		return out
	case "Triads":
		fallthrough
	default:
		offset := 120.0
		return []*gdk.RGBA{hsvToRGBA(h, s, v), mk(h + offset), mk(h - offset)}
	}
}

func paletteByName(name string) []string {
	switch name {
	case "Web-safe (legacy)":
		vals := []int{0x00, 0x33, 0x66, 0x99, 0xCC, 0xFF}
		colors := make([]string, 0, 216)
		for _, r := range vals {
			for _, g := range vals {
				for _, b := range vals {
					colors = append(colors, fmt.Sprintf("#%02X%02X%02X", r, g, b))
				}
			}
		}
		return colors
	case "Material Design":
		return []string{
			"#F44336", "#E91E63", "#9C27B0", "#673AB7", "#3F51B5", "#2196F3",
			"#03A9F4", "#00BCD4", "#009688", "#4CAF50", "#8BC34A", "#CDDC39",
			"#FFEB3B", "#FFC107", "#FF9800", "#FF5722", "#795548", "#9E9E9E",
			"#607D8B", "#000000", "#FFFFFF", "#EF5350", "#EC407A", "#AB47BC",
			"#7E57C2", "#5C6BC0", "#42A5F5", "#29B6F6", "#26C6DA", "#26A69A",
			"#66BB6A", "#9CCC65", "#D4E157", "#FFEE58", "#FFCA28", "#FFA726",
			"#FF7043", "#8D6E63", "#BDBDBD", "#78909C", "#212121", "#FAFAFA",
			"#C62828", "#AD1457", "#6A1B9A", "#4527A0", "#283593", "#1565C0",
		}
	case "Tailwind CSS":
		return []string{
			"#EF4444", "#F97316", "#F59E0B", "#EAB308", "#84CC16", "#22C55E",
			"#10B981", "#14B8A6", "#06B6D4", "#0EA5E9", "#3B82F6", "#6366F1",
			"#8B5CF6", "#A855F7", "#D946EF", "#EC4899", "#F43F5E", "#64748B",
			"#DC2626", "#EA580C", "#D97706", "#CA8A04", "#65A30D", "#16A34A",
			"#059669", "#0D9488", "#0891B2", "#0284C7", "#2563EB", "#4F46E5",
			"#7C3AED", "#9333EA", "#C026D3", "#DB2777", "#E11D48", "#475569",
			"#991B1B", "#9A3412", "#92400E", "#854D0E", "#3F6212", "#14532D",
			"#064E3B", "#134E4A", "#164E63", "#075985", "#1E3A8A", "#312E81",
		}
	case "Flat UI":
		return []string{
			"#1ABC9C", "#16A085", "#2ECC71", "#27AE60", "#3498DB", "#2980B9",
			"#9B59B6", "#8E44AD", "#34495E", "#2C3E50", "#F1C40F", "#F39C12",
			"#E67E22", "#D35400", "#E74C3C", "#C0392B", "#ECF0F1", "#BDC3C7",
			"#95A5A6", "#7F8C8D", "#52B3D9", "#E8F8F5", "#D5F4E6", "#D6EAF8",
			"#E8DAEF", "#FADBD8", "#F9E79F", "#FAD7A0", "#F5B7B1", "#D7DBDD",
		}
	case "Pastel":
		return []string{
			"#FFB3BA", "#FFDFBA", "#FFFFBA", "#BAFFC9", "#BAE1FF", "#E0BBE4",
			"#FFDFD3", "#FEC8D8", "#D5AAFF", "#B4F8C8", "#A0E7E5", "#FFAEBC",
			"#FBE7C6", "#B4F8C8", "#A0C4FF", "#BDB2FF", "#FFC6FF", "#FDFFB6",
			"#CAFFBF", "#9BF6FF", "#A0C4FF", "#BDB2FF", "#FFC6FF", "#FFFFFC",
			"#FFD6A5", "#FDFFB6", "#CAFFBF", "#A8E6CF", "#FFD3B6", "#FFAAA5",
		}
	case "Nord":
		return []string{
			"#2E3440", "#3B4252", "#434C5E", "#4C566A", "#D8DEE9", "#E5E9F0",
			"#ECEFF4", "#8FBCBB", "#88C0D0", "#81A1C1", "#5E81AC", "#BF616A",
			"#D08770", "#EBCB8B", "#A3BE8C", "#B48EAD", "#4C566A", "#434C5E",
			"#3B4252", "#2E3440", "#ECEFF4", "#E5E9F0", "#D8DEE9", "#88C0D0",
		}
	case "Dracula":
		return []string{
			"#282A36", "#44475A", "#F8F8F2", "#6272A4", "#8BE9FD", "#50FA7B",
			"#FFB86C", "#FF79C6", "#BD93F9", "#FF5555", "#F1FA8C", "#21222C",
			"#191A21", "#6272A4", "#B45BCF", "#4D4F68", "#626483", "#62D6E8",
			"#EA51B2", "#EBFF87", "#00F769", "#B45BCF", "#7081D0", "#A1EFE4",
		}
	case "Solarized":
		return []string{
			"#002B36", "#073642", "#586E75", "#657B83", "#839496", "#93A1A1",
			"#EEE8D5", "#FDF6E3", "#B58900", "#CB4B16", "#DC322F", "#D33682",
			"#6C71C4", "#268BD2", "#2AA198", "#859900", "#002B36", "#073642",
			"#586E75", "#657B83", "#839496", "#93A1A1", "#EEE8D5", "#FDF6E3",
		}
	case "Gruvbox":
		return []string{
			"#282828", "#CC241D", "#98971A", "#D79921", "#458588", "#B16286",
			"#689D6A", "#A89984", "#928374", "#FB4934", "#B8BB26", "#FABD2F",
			"#83A598", "#D3869B", "#8EC07C", "#EBDBB2", "#FBF1C7", "#3C3836",
			"#504945", "#665C54", "#7C6F64", "#D65D0E", "#FE8019", "#BDAE93",
		}
	case "One Dark":
		return []string{
			"#282C34", "#ABB2BF", "#E06C75", "#D19A66", "#E5C07B", "#98C379",
			"#56B6C2", "#61AFEF", "#C678DD", "#BE5046", "#3B4048", "#4B5263",
			"#545862", "#565C64", "#5C6370", "#636D83", "#828997", "#2C323C",
			"#353B45", "#3E4451", "#4F5666", "#5F697A", "#6B7587", "#979EAB",
		}
	case "Monokai":
		return []string{
			"#272822", "#F8F8F2", "#F92672", "#E6DB74", "#A6E22E", "#66D9EF",
			"#AE81FF", "#FD971F", "#75715E", "#49483E", "#3E3D32", "#F8F8F0",
			"#F5F4F1", "#A59F85", "#FD5FF0", "#F4BF75", "#FFF59D", "#CFCFC2",
			"#A1EFE4", "#FFE792", "#CC6633", "#778899", "#9D550F", "#E69F66",
		}
	case "KiJiSH Dark Pastel Terminal":
		return []string{
			"#2C2C2C", "#DCDCDC", "#3F3F3F", "#D67979", "#60B48A", "#DFAF8F",
			"#9AB8D7", "#DC8CC3", "#8CD0D3", "#DCDCDC", "#709080", "#DCA3A3",
			"#72D5A3", "#F0DFAF", "#94BFF3", "#EC93D3", "#93E0E3", "#FFFFFF",
		}
	default:
		vals := []int{0x00, 0x33, 0x66, 0x99, 0xCC, 0xFF}
		colors := make([]string, 0, 216)
		for _, r := range vals {
			for _, g := range vals {
				for _, b := range vals {
					colors = append(colors, fmt.Sprintf("#%02X%02X%02X", r, g, b))
				}
			}
		}
		return colors
	}
}

func solidPixbuf(hex string, width, height int) *gdk.Pixbuf {
	pb, err := gdk.PixbufNew(gdk.COLORSPACE_RGB, false, 8, width, height)
	if err != nil {
		return nil
	}
	rgba := gdk.NewRGBA()
	rgba.Parse(hex)
	r := byte(rgba.GetRed() * 255)
	g := byte(rgba.GetGreen() * 255)
	b := byte(rgba.GetBlue() * 255)

	pixels := pb.GetPixels()
	rowstride := pb.GetRowstride()
	channels := pb.GetNChannels()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			off := y*rowstride + x*channels
			pixels[off] = r
			pixels[off+1] = g
			pixels[off+2] = b
		}
	}
	return pb
}

func rgbToHSV(rgba *gdk.RGBA) (float64, float64, float64) {
	r := rgba.GetRed()
	g := rgba.GetGreen()
	b := rgba.GetBlue()

	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	delta := mx - mn

	h := 0.0
	if delta > 0 {
		switch mx {
		case r:
			h = 60 * math.Mod((g-b)/delta, 6)
		case g:
			h = 60 * ((b-r)/delta + 2)
		case b:
			h = 60 * ((r-g)/delta + 4)
		}
	}
	if h < 0 {
		h += 360
	}

	s := 0.0
	if mx > 0 {
		s = (delta / mx) * 100
	}
	v := mx * 100
	return wrapHue(h), clamp(s, 0, 100), clamp(v, 0, 100)
}

func hsvToRGBA(h, s, v float64) *gdk.RGBA {
	h = wrapHue(h)
	s = clamp(s, 0, 100) / 100
	v = clamp(v, 0, 100) / 100

	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c

	var r1, g1, b1 float64
	switch {
	case h < 60:
		r1, g1, b1 = c, x, 0
	case h < 120:
		r1, g1, b1 = x, c, 0
	case h < 180:
		r1, g1, b1 = 0, c, x
	case h < 240:
		r1, g1, b1 = 0, x, c
	case h < 300:
		r1, g1, b1 = x, 0, c
	default:
		r1, g1, b1 = c, 0, x
	}

	rgba := gdk.NewRGBA()
	rgba.SetRed(r1 + m)
	rgba.SetGreen(g1 + m)
	rgba.SetBlue(b1 + m)
	rgba.SetAlpha(1)
	return rgba
}

func rgbaToHex(rgba *gdk.RGBA) string {
	r := int(math.Round(rgba.GetRed() * 255))
	g := int(math.Round(rgba.GetGreen() * 255))
	b := int(math.Round(rgba.GetBlue() * 255))
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

func wrapHue(h float64) float64 {
	v := math.Mod(h, 360)
	if v < 0 {
		v += 360
	}
	return v
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func luminance(rgba *gdk.RGBA) float64 {
	r := rgba.GetRed()
	g := rgba.GetGreen()
	b := rgba.GetBlue()
	return 0.2126*r + 0.7152*g + 0.0722*b
}
