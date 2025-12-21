package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const (
	appTitle   = "kjagave"
	appVersion = "1.0"
)

type SavedColor struct {
	Hex  string `json:"hex"`
	Name string `json:"name"`
}

type App struct {
	window       *gtk.Window
	colorButton  *gtk.ColorButton
	currentColor *gdk.RGBA
	listStore    *gtk.ListStore
	treeView     *gtk.TreeView
	deleteBtn    *gtk.Button
	savedColors  []SavedColor
	configFile   string
	selectedIter *gtk.TreeIter
}

func main() {
	gtk.Init(nil)

	configDir := filepath.Join(os.Getenv("HOME"), ".config")
	os.MkdirAll(configDir, 0755)

	app := &App{
		configFile: filepath.Join(configDir, "kjagave.json"),
	}

	app.loadColors()
	app.createUI()
	app.populateList()

	gtk.Main()
}

func (app *App) createUI() {
	var err error

	// main window
	app.window, err = gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		log.Fatal("Unable to create window:", err)
	}
	app.window.SetTitle(appTitle)
	app.window.SetDefaultSize(550, 450)
	app.window.SetResizable(false)
	app.window.Connect("destroy", gtk.MainQuit)

	// vertical box
	mainBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 10)
	mainBox.SetMarginTop(15)
	mainBox.SetMarginBottom(15)
	mainBox.SetMarginStart(15)
	mainBox.SetMarginEnd(15)

	// color selection area
	colorBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 10)
	colorBox.SetHAlign(gtk.ALIGN_CENTER)

	label, _ := gtk.LabelNew("Select Color:")
	colorBox.PackStart(label, false, false, 0)

	// initialize color 
	app.currentColor = gdk.NewRGBA()
	app.currentColor.Parse("#69BAA7")

	app.colorButton, err = gtk.ColorButtonNewWithRGBA(app.currentColor)
	if err != nil {
		log.Fatal("Unable to create color button:", err)
	}
	app.colorButton.SetUseAlpha(true)
	app.colorButton.SetTitle("Choose a Color")
	app.colorButton.Connect("color-set", func() {
		app.currentColor = app.colorButton.GetRGBA()
	})

	colorBox.PackStart(app.colorButton, false, false, 0)

	hexEntry, _ := gtk.EntryNew()
	hexEntry.SetEditable(false)
	hexEntry.SetWidthChars(10)
	hexEntry.SetText(rgbaToHex(app.currentColor))
	colorBox.PackStart(hexEntry, false, false, 0)

	// color picker button
	pickerBtn, _ := gtk.ButtonNewWithLabel("Pick from Screen")
	pickerBtn.Connect("clicked", func() {
		if color, err := app.pickColorFromScreen(); err == nil {
			app.colorButton.SetRGBA(color)
			app.currentColor = color
			hexEntry.SetText(rgbaToHex(color))
		}
	})
	colorBox.PackStart(pickerBtn, false, false, 0)

	// bump hex entry when color changes
	app.colorButton.Connect("color-set", func() {
		app.currentColor = app.colorButton.GetRGBA()
		hexEntry.SetText(rgbaToHex(app.currentColor))
	})

	mainBox.PackStart(colorBox, false, false, 0)

	expander, _ := gtk.ExpanderNew("Saved Colors")
	expander.SetExpanded(true)
	expanderBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 5)
	expanderBox.SetMarginTop(5)
	expanderBox.SetMarginBottom(5)
	btnBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 5)
	btnBox.SetHAlign(gtk.ALIGN_END)
	app.deleteBtn, _ = gtk.ButtonNewWithLabel("Delete")
	app.deleteBtn.SetSensitive(false)
	app.deleteBtn.Connect("clicked", app.onDeleteClicked)
	btnBox.PackStart(app.deleteBtn, false, false, 0)
	saveBtn, _ := gtk.ButtonNewWithLabel("Save...")
	saveBtn.Connect("clicked", app.onSaveClicked)
	btnBox.PackStart(saveBtn, false, false, 0)
	expanderBox.PackStart(btnBox, false, false, 0)

	// treeview
	scrolled, _ := gtk.ScrolledWindowNew(nil, nil)
	scrolled.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	scrolled.SetSizeRequest(-1, 180)

	app.listStore, _ = gtk.ListStoreNew(gdk.PixbufGetType(), glib.TYPE_STRING, glib.TYPE_STRING)
	app.treeView, _ = gtk.TreeViewNew()
	app.treeView.SetModel(app.listStore)
	app.treeView.SetHeadersVisible(true)

	// color column with swatch
	colorCol, _ := gtk.TreeViewColumnNew()
	colorCol.SetTitle("Color")
	colorCol.SetSortColumnID(1)

	pixbufRenderer, _ := gtk.CellRendererPixbufNew()
	colorCol.PackStart(pixbufRenderer, false)
	colorCol.AddAttribute(pixbufRenderer, "pixbuf", 0)

	textRenderer, _ := gtk.CellRendererTextNew()
	colorCol.PackStart(textRenderer, true)
	colorCol.AddAttribute(textRenderer, "text", 1)

	app.treeView.AppendColumn(colorCol)

	// name column
	nameRenderer, _ := gtk.CellRendererTextNew()
	nameCol, _ := gtk.TreeViewColumnNewWithAttribute("Name", nameRenderer, "text", 2)
	nameCol.SetSortColumnID(2)
	app.treeView.AppendColumn(nameCol)

	selection, _ := app.treeView.GetSelection()
	selection.SetMode(gtk.SELECTION_SINGLE)
	selection.Connect("changed", app.onSelectionChanged)

	scrolled.Add(app.treeView)
	expanderBox.PackStart(scrolled, true, true, 0)
	expander.Add(expanderBox)
	mainBox.PackStart(expander, true, true, 0)

	// bottom button box
	bottomBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 5)
	bottomBox.SetHAlign(gtk.ALIGN_END)

	copyBtn, _ := gtk.ButtonNewWithLabel("Copy to Clipboard")
	copyBtn.Connect("clicked", app.onCopyClicked)
	bottomBox.PackStart(copyBtn, false, false, 0)

	aboutBtn, _ := gtk.ButtonNewWithLabel("About")
	aboutBtn.Connect("clicked", app.onAboutClicked)
	bottomBox.PackStart(aboutBtn, false, false, 0)

	mainBox.PackStart(bottomBox, false, false, 0)

	app.window.Add(mainBox)
	app.window.ShowAll()
}

func (app *App) populateList() {
	app.listStore.Clear()

	for _, color := range app.savedColors {
		pixbuf := app.createColorSwatch(color.Hex)
		iter := app.listStore.Append()
		app.listStore.Set(iter, []int{0, 1, 2}, []interface{}{pixbuf, color.Hex, color.Name})
	}
}

func (app *App) createColorSwatch(hexColor string) *gdk.Pixbuf {
	pixbuf, err := gdk.PixbufNew(gdk.COLORSPACE_RGB, false, 8, 16, 14)
	if err != nil {
		return nil
	}

	rgba := gdk.NewRGBA()
	rgba.Parse(hexColor)

	r := uint32(rgba.GetRed() * 255)
	g := uint32(rgba.GetGreen() * 255)
	b := uint32(rgba.GetBlue() * 255)

	pixels := pixbuf.GetPixels()
	rowstride := pixbuf.GetRowstride()
	nChannels := pixbuf.GetNChannels()

	for y := 0; y < 14; y++ {
		for x := 0; x < 16; x++ {
			offset := y*rowstride + x*nChannels
			pixels[offset] = byte(r)
			pixels[offset+1] = byte(g)
			pixels[offset+2] = byte(b)
		}
	}

	return pixbuf
}

func (app *App) onSelectionChanged(selection *gtk.TreeSelection) {
	model, iter, ok := selection.GetSelected()
	if !ok {
		app.deleteBtn.SetSensitive(false)
		app.selectedIter = nil
		return
	}

	app.selectedIter = iter
	app.deleteBtn.SetSensitive(true)

	value, _ := model.ToTreeModel().GetValue(iter, 1)
	hexColor, _ := value.GetString()

	rgba := gdk.NewRGBA()
	rgba.Parse(hexColor)
	app.colorButton.SetRGBA(rgba)
	app.currentColor = rgba
}

func (app *App) onSaveClicked() {
	dialog, _ := gtk.DialogNew()
	dialog.SetTitle("Save Color")
	dialog.SetTransientFor(app.window)
	dialog.SetModal(true)
	dialog.SetDefaultSize(300, -1)

	box, _ := dialog.GetContentArea()
	box.SetSpacing(10)
	box.SetMarginTop(10)
	box.SetMarginBottom(10)
	box.SetMarginStart(10)
	box.SetMarginEnd(10)

	// get current color
	hexColor := rgbaToHex(app.currentColor)

	label, _ := gtk.LabelNew(fmt.Sprintf("Color: %s", hexColor))
	box.PackStart(label, false, false, 0)

	entryLabel, _ := gtk.LabelNew("Color Name:")
	entryLabel.SetHAlign(gtk.ALIGN_START)
	box.PackStart(entryLabel, false, false, 0)

	entry, _ := gtk.EntryNew()
	entry.SetText("Untitled")
	entry.SetActivatesDefault(true)
	box.PackStart(entry, false, false, 0)

	dialog.AddButton("Cancel", gtk.RESPONSE_CANCEL)
	okBtn, _ := dialog.AddButton("OK", gtk.RESPONSE_OK)
	okBtn.SetCanDefault(true)
	okBtn.GrabDefault()

	dialog.ShowAll()

	response := dialog.Run()
	if response == gtk.RESPONSE_OK {
		text, _ := entry.GetText()
		app.savedColors = append([]SavedColor{{Hex: hexColor, Name: text}}, app.savedColors...)
		app.saveColors()
		app.populateList()
	}

	dialog.Destroy()
}

func (app *App) onDeleteClicked() {
	if app.selectedIter == nil {
		return
	}

	model := app.listStore.ToTreeModel()
	value, _ := model.GetValue(app.selectedIter, 1)
	hexColor, _ := value.GetString()

	// remove from saved colors
	for i, color := range app.savedColors {
		if color.Hex == hexColor {
			app.savedColors = append(app.savedColors[:i], app.savedColors[i+1:]...)
			break
		}
	}

	app.saveColors()
	app.populateList()
	app.deleteBtn.SetSensitive(false)
	app.selectedIter = nil
}

func (app *App) onCopyClicked() {
	hexColor := rgbaToHex(app.currentColor)

	clipboard, _ := gtk.ClipboardGet(gdk.SELECTION_CLIPBOARD)
	clipboard.SetText(hexColor)

	dialog := gtk.MessageDialogNew(app.window, gtk.DIALOG_MODAL, gtk.MESSAGE_INFO,
		gtk.BUTTONS_OK, fmt.Sprintf("Color %s copied to clipboard!", hexColor))
	dialog.Run()
	dialog.Destroy()
}

func (app *App) onAboutClicked() {
	dialog, _ := gtk.AboutDialogNew()
	dialog.SetTransientFor(app.window)
	dialog.SetProgramName(appTitle)
	dialog.SetVersion(appVersion)
	dialog.SetComments("A color picker with screen color grabbing support")
	dialog.SetAuthors([]string{"kjagave 2025", "Based on gcolor2 by Ned Haughton"})
	dialog.SetLicense("GPL-2.0")
	dialog.Run()
	dialog.Destroy()
}

func (app *App) pickColorFromScreen() (*gdk.RGBA, error) {
	// use xcolor for x11 color picking
	cmd := exec.Command("xcolor", "--format", "hex")
	output, err := cmd.Output()
	if err != nil {
		// fallback to grabc
		cmd = exec.Command("grabc")
		output, err = cmd.Output()
		if err != nil {
			dialog := gtk.MessageDialogNew(app.window, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR,
				gtk.BUTTONS_OK, "Color picker not found. Please install 'xcolor'")
			dialog.Run()
			dialog.Destroy()
			return nil, err
		}
	}

	hexColor := strings.TrimSpace(string(output))
	if !strings.HasPrefix(hexColor, "#") {
		hexColor = "#" + hexColor
	}

	rgba := gdk.NewRGBA()
	if !rgba.Parse(hexColor) {
		return nil, fmt.Errorf("invalid color format: %s", hexColor)
	}

	return rgba, nil
}

func (app *App) loadColors() {
	data, err := os.ReadFile(app.configFile)
	if err != nil {
		app.savedColors = []SavedColor{}
		return
	}

	if err := json.Unmarshal(data, &app.savedColors); err != nil {
		log.Printf("Error loading colors: %v", err)
		app.savedColors = []SavedColor{}
	}
}

func (app *App) saveColors() {
	data, err := json.MarshalIndent(app.savedColors, "", "  ")
	if err != nil {
		log.Printf("Error marshaling colors: %v", err)
		return
	}

	if err := os.WriteFile(app.configFile, data, 0644); err != nil {
		log.Printf("Error saving colors: %v", err)
	}
}

func rgbaToHex(rgba *gdk.RGBA) string {
	r := uint8(rgba.GetRed() * 255)
	g := uint8(rgba.GetGreen() * 255)
	b := uint8(rgba.GetBlue() * 255)
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}
