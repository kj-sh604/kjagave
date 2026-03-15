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
}

type App struct {
	window *gtk.Window

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

	paletteStore *gtk.ListStore
	paletteView  *gtk.TreeView

	favoritesStore *gtk.ListStore
	favoritesView  *gtk.TreeView
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
	"Web-safe colors",
	"Tango",
	"Visibone Core",
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
	app.populatePaletteList()
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
	app.window.SetDefaultSize(980, 680)
	app.window.Connect("destroy", func() {
		app.saveConfig()
		gtk.MainQuit()
	})

	root, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 8)
	root.SetMarginTop(8)
	root.SetMarginBottom(8)
	root.SetMarginStart(8)
	root.SetMarginEnd(8)

	toolbar, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	app.historyBackBtn, _ = gtk.ButtonNewWithLabel("Back")
	app.historyBackBtn.Connect("clicked", func() { app.navigateHistory(-1) })
	toolbar.PackStart(app.historyBackBtn, false, false, 0)

	app.historyFwdBtn, _ = gtk.ButtonNewWithLabel("Forward")
	app.historyFwdBtn.Connect("clicked", func() { app.navigateHistory(1) })
	toolbar.PackStart(app.historyFwdBtn, false, false, 0)

	randomBtn, _ := gtk.ButtonNewWithLabel("Random")
	randomBtn.Connect("clicked", func() { app.randomizeColor() })
	toolbar.PackStart(randomBtn, false, false, 0)

	app.lightenBtn, _ = gtk.ButtonNewWithLabel("Lighten")
	app.lightenBtn.Connect("clicked", func() { app.adjustSV(0, 5) })
	toolbar.PackStart(app.lightenBtn, false, false, 0)

	app.darkenBtn, _ = gtk.ButtonNewWithLabel("Darken")
	app.darkenBtn.Connect("clicked", func() { app.adjustSV(0, -5) })
	toolbar.PackStart(app.darkenBtn, false, false, 0)

	app.saturateBtn, _ = gtk.ButtonNewWithLabel("Saturate")
	app.saturateBtn.Connect("clicked", func() { app.adjustSV(5, 0) })
	toolbar.PackStart(app.saturateBtn, false, false, 0)

	app.desaturateBtn, _ = gtk.ButtonNewWithLabel("Desaturate")
	app.desaturateBtn.Connect("clicked", func() { app.adjustSV(-5, 0) })
	toolbar.PackStart(app.desaturateBtn, false, false, 0)

	pasteBtn, _ := gtk.ButtonNewWithLabel("Paste")
	pasteBtn.Connect("clicked", func() { app.pasteColorFromClipboard() })
	toolbar.PackStart(pasteBtn, false, false, 0)

	aboutBtn, _ := gtk.ButtonNewWithLabel("About")
	aboutBtn.Connect("clicked", func() { app.onAboutClicked() })
	toolbar.PackStart(aboutBtn, false, false, 0)

	root.PackStart(toolbar, false, false, 0)

	schemeRow, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	app.swatchCards = make([]SwatchCard, 0, 4)
	for i := 0; i < 4; i++ {
		card := app.newSwatchCard()
		cardIdx := i
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

	controlRow, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
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

	useHexBtn, _ := gtk.ButtonNewWithLabel("Use")
	useHexBtn.Connect("clicked", func() { app.applyHexEntry() })
	controlRow.PackStart(useHexBtn, false, false, 0)

	pickBtn, _ := gtk.ButtonNewWithLabel("Pick from Screen")
	pickBtn.Connect("clicked", func() {
		clr, err := app.pickColorFromScreen()
		if err == nil {
			app.setCurrentColor(clr, true)
		}
	})
	controlRow.PackStart(pickBtn, false, false, 0)

	copyBtn, _ := gtk.ButtonNewWithLabel("Copy")
	copyBtn.Connect("clicked", func() {
		clipboard, _ := gtk.ClipboardGet(gdk.SELECTION_CLIPBOARD)
		clipboard.SetText(app.currentHex)
	})
	controlRow.PackStart(copyBtn, false, false, 0)

	root.PackStart(controlRow, false, false, 0)

	lower, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)

	paletteFrame, _ := gtk.FrameNew("Palette")
	paletteBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 6)
	paletteBox.SetMarginTop(8)
	paletteBox.SetMarginBottom(8)
	paletteBox.SetMarginStart(8)
	paletteBox.SetMarginEnd(8)

	app.paletteCombo, _ = gtk.ComboBoxTextNew()
	for _, name := range paletteNames {
		app.paletteCombo.AppendText(name)
	}
	app.paletteCombo.SetActive(0)
	app.paletteCombo.Connect("changed", func() {
		app.populatePaletteList()
		app.saveConfig()
	})
	paletteBox.PackStart(app.paletteCombo, false, false, 0)

	paletteScroll, _ := gtk.ScrolledWindowNew(nil, nil)
	paletteScroll.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	paletteScroll.SetSizeRequest(430, 260)

	app.paletteStore, _ = gtk.ListStoreNew(gdk.PixbufGetType(), glib.TYPE_STRING)
	app.paletteView, _ = gtk.TreeViewNew()
	app.paletteView.SetModel(app.paletteStore)
	app.paletteView.SetHeadersVisible(false)
	paletteCol, _ := gtk.TreeViewColumnNew()
	palettePix, _ := gtk.CellRendererPixbufNew()
	paletteCol.PackStart(palettePix, false)
	paletteCol.AddAttribute(palettePix, "pixbuf", 0)
	paletteText, _ := gtk.CellRendererTextNew()
	paletteCol.PackStart(paletteText, true)
	paletteCol.AddAttribute(paletteText, "text", 1)
	app.paletteView.AppendColumn(paletteCol)
	palSel, _ := app.paletteView.GetSelection()
	palSel.SetMode(gtk.SELECTION_SINGLE)
	palSel.Connect("changed", app.onPaletteSelectionChanged)
	paletteScroll.Add(app.paletteView)
	paletteBox.PackStart(paletteScroll, true, true, 0)
	paletteFrame.Add(paletteBox)
	lower.PackStart(paletteFrame, true, true, 0)

	favFrame, _ := gtk.FrameNew("Favorites")
	favBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 6)
	favBox.SetMarginTop(8)
	favBox.SetMarginBottom(8)
	favBox.SetMarginStart(8)
	favBox.SetMarginEnd(8)

	favScroll, _ := gtk.ScrolledWindowNew(nil, nil)
	favScroll.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	favScroll.SetSizeRequest(260, 260)

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

	favScroll.Add(app.favoritesView)
	favBox.PackStart(favScroll, true, true, 0)

	favBtns, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	addFavBtn, _ := gtk.ButtonNewWithLabel("+")
	addFavBtn.Connect("clicked", func() { app.addCurrentToFavorites() })
	favBtns.PackStart(addFavBtn, true, true, 0)

	app.removeFavBtn, _ = gtk.ButtonNewWithLabel("-")
	app.removeFavBtn.Connect("clicked", func() { app.removeSelectedFavorite() })
	favBtns.PackStart(app.removeFavBtn, true, true, 0)

	app.clearFavBtn, _ = gtk.ButtonNewWithLabel("Clear")
	app.clearFavBtn.Connect("clicked", func() {
		app.savedColors = nil
		app.refreshFavoritesView()
		app.saveConfig()
	})
	favBtns.PackStart(app.clearFavBtn, true, true, 0)

	exportBtn, _ := gtk.ButtonNewWithLabel("Export GPL")
	exportBtn.Connect("clicked", func() { app.exportFavoritesGPLToDefaultPath() })
	favBtns.PackStart(exportBtn, true, true, 0)

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

func (app *App) newSwatchCard() SwatchCard {
	button, _ := gtk.ButtonNew()
	button.SetSizeRequest(220, 240)

	vbox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 4)
	vbox.SetMarginTop(8)
	vbox.SetMarginBottom(8)
	vbox.SetMarginStart(8)
	vbox.SetMarginEnd(8)

	image, _ := gtk.ImageNew()
	image.SetFromPixbuf(solidPixbuf("#000000", 220, 150))
	vbox.PackStart(image, false, false, 0)

	label, _ := gtk.LabelNew("")
	label.SetJustify(gtk.JUSTIFY_CENTER)
	label.SetHAlign(gtk.ALIGN_CENTER)
	vbox.PackStart(label, false, false, 0)

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
			app.swatchCards[i].button.Hide()
			continue
		}
		app.swatchCards[i].button.Show()
		hex := rgbaToHex(colors[i])
		h, s, v := rgbToHSV(colors[i])
		r := int(math.Round(colors[i].GetRed() * 255))
		g := int(math.Round(colors[i].GetGreen() * 255))
		b := int(math.Round(colors[i].GetBlue() * 255))
		app.swatchCards[i].hex = hex
		app.swatchCards[i].image.SetFromPixbuf(solidPixbuf(hex, 220, 150))
		app.swatchCards[i].label.SetText(fmt.Sprintf("%s\nrgb(%d, %d, %d)\nhsv(%d, %d, %d)", hex, r, g, b, int(h), int(s), int(v)))
	}
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

func (app *App) onPaletteSelectionChanged(selection *gtk.TreeSelection) {
	model, iter, ok := selection.GetSelected()
	if !ok {
		return
	}
	value, _ := model.ToTreeModel().GetValue(iter, 1)
	hex, _ := value.GetString()
	rgba := gdk.NewRGBA()
	if rgba.Parse(hex) {
		app.setCurrentColor(rgba, true)
	}
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

func (app *App) populatePaletteList() {
	app.paletteStore.Clear()
	for _, hex := range paletteByName(app.activePaletteName()) {
		iter := app.paletteStore.Append()
		app.paletteStore.Set(iter, []int{0, 1}, []interface{}{solidPixbuf(hex, 36, 14), hex})
	}
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

func (app *App) refreshFavoritesView() {
	app.favoritesStore.Clear()
	for _, color := range app.savedColors {
		iter := app.favoritesStore.Append()
		app.favoritesStore.Set(iter, []int{0, 1, 2}, []interface{}{solidPixbuf(color.Hex, 16, 14), strings.ToUpper(color.Hex), color.Name})
	}
	app.updateActionStates()
}

func (app *App) exportFavoritesGPLToDefaultPath() {
	if len(app.savedColors) == 0 {
		return
	}
	outPath := filepath.Join(filepath.Dir(app.configFile), "kjagave-favorites.gpl")
	lines := []string{"GIMP Palette", "Name: kjagave Favorites", "#"}
	for _, item := range app.savedColors {
		r, g, b := hexToRGB(item.Hex)
		lines = append(lines, fmt.Sprintf("%3d %3d %3d\t%s", r, g, b, item.Name))
	}
	_ = os.WriteFile(outPath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
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
	case "Tango":
		return []string{
			"#2E3436", "#555753", "#888A85", "#BABDB6", "#D3D7CF", "#EEEEEC",
			"#FCE94F", "#EDD400", "#C4A000", "#8AE234", "#73D216", "#4E9A06",
			"#729FCF", "#3465A4", "#204A87", "#AD7FA8", "#75507B", "#5C3566",
			"#EF2929", "#CC0000", "#A40000", "#FCAF3E", "#F57900", "#CE5C00",
		}
	case "Visibone Core":
		return []string{
			"#000000", "#333333", "#666666", "#999999", "#CCCCCC", "#FFFFFF",
			"#FF0000", "#FF9900", "#FFFF00", "#00FF00", "#00FFFF", "#0000FF",
			"#9900FF", "#FF00FF", "#FF0066", "#663300", "#CC6633", "#99CC33",
			"#6699CC", "#CC33CC", "#CC9999", "#33CCCC", "#336699", "#9966CC",
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

func hexToRGB(hex string) (int, int, int) {
	text := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(text) != 6 {
		return 0, 0, 0
	}
	var r, g, b int
	_, err := fmt.Sscanf(text, "%02x%02x%02x", &r, &g, &b)
	if err != nil {
		return 0, 0, 0
	}
	return r, g, b
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
