![kjagave logo](./archlinux/kjagave.png)

# kjagave 

GoLang rewrite of Agave (legacy colorscheme designer tool for GNOME2) with a modern GTK3 interface and additional features.

![screenshot of kjagave](./pics/readme-greeter.png)

## features

- agave-style scheme generator modes:
	- triads
	- complements
	- split complements
	- tetrads
	- analogous
	- monochromatic
- compact top swatch cards with centered overlay metadata:
	- hex
	- rgb
	- hsv
- dynamic overlay text contrast (light/dark text based on background luminance)
- toolbar actions: back, forward, random, lighten, darken, saturate, desaturate, paste
- click a top swatch to promote it to the active base color
- right-click on top swatches and palette swatches to copy:
	- hex
	- hsv
	- rgb
- 12 built-in palettes:
	- Web-safe (legacy)
	- Material Design
	- Tailwind CSS
	- Flat UI
	- Pastel
	- Nord
	- Dracula
	- Solarized
	- Gruvbox
	- One Dark
	- Monokai
	- KiJiSH Dark Pastel Terminal
- favorites panel with add/remove/rename/clear
- clipboard copy/paste support
- screen picker support on X11 via `xcolor` or `grabc`
- persisted state in `~/.config/kjagave.json` (last color, scheme, palette, favorites)

## requirements

- go 1.21 or higher
- gtk3 development libraries
- `gotk3` Go bindings
- optional for pick-from-screen: `xcolor` or `grabc`

## installation

### arch linux

```bash
cd archlinux
makepkg -si
```

### manual build

```sh
cd src
go build -o kjagave .
```

## running

```bash
./kjagave
```

## usage

1. pick a base color with the color button, hex entry, palette swatches, or screen picker
2. choose a scheme type
3. click a preview swatch to make it the active base color
4. right-click a swatch to copy hex/hsv/rgb
5. use favorites controls (`+`, `-`, rename, clear) to manage saved colors