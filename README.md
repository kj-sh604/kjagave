![kjagave logo](./archlinux/kjagave.png)

# kjagave 

a GTK3 color scheme generator inspired by classic Agave, rewritten in Go

<img width="554" height="467" alt="recent screenshot" src="https://github.com/user-attachments/assets/efbea3f0-9d1f-45b1-865b-bb82a3ddc443" /><br>


![screenshot of kjagave](./pics/readme-greeter.png)

## features

- Agave-style scheme generator with:
	- Triads
	- Complements
	- Split Complements
	- Tetrads
	- Analogous
	- Monochromatic
- large scheme preview cards showing hex, rgb, and hsv
- lighten, darken, saturate, desaturate quick actions
- color history navigation (back/forward)
- random color generation
- palette browser with built-in web-safe, tango, and visibone-style sets
- favorites panel with add/remove/clear
- favorites export to GIMP `.gpl` (`~/.config/kjagave-favorites.gpl`)
- clipboard copy/paste for hex colors
- screen picker support on X11 (`xcolor` or `grabc`)

## requirements

- go 1.21 or higher
- gtk3 development libraries
- `gotk3` go bindings
- `xcolor`

## installation

### arch linux

```bash
cd archlinux
makepkg -si
```

see `archlinux/README.md` for more details.

### manual build

```sh
cd src
go mod download
go mod download github.com/gotk3/gotk3
go build -o kjagave main.go
```

## running

```bash
./kjagave
```

## usage

1. pick a base color with the color button, hex entry, palette list, or screen picker
2. choose a scheme type from the combo box
3. click any preview card to promote that scheme color to the active base color
4. use toolbar actions (`Back`, `Forward`, `Random`, `Lighten`, `Darken`, `Saturate`, `Desaturate`, `Paste`) to iterate quickly
5. add colors to favorites with `+`, remove with `-`, and export favorites with `Export GPL`

state and favorites are stored in `~/.config/kjagave.json`.
