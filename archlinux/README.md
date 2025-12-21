# Arch Linux Package

This directory contains the PKGBUILD and related files for building and installing kjagave on Arch Linux.

## Building the Package

```bash
cd archlinux
makepkg -si
```

## Installing from AUR (future)

Once uploaded to the AUR:

```bash
yay -S kjagave
# or
paru -S kjagave
```

## Files

- `PKGBUILD` - Build script for Arch Linux
- `kjagave.desktop` - XDG desktop entry
- `kjagave.png` - Application icon
- `.SRCINFO` - Package metadata (generated with `makepkg --printsrcinfo`)

## Building from Local Repository

If you're testing locally before tagging a release:

```bash
# In the archlinux directory, edit PKGBUILD to use local source
# Replace the source line with:
# source=("${pkgname}::git+file://$(pwd)/..")

makepkg -si
```

## Note

The PKGBUILD expects a tagged release (v1.0) on GitHub. Make sure to create and push the tag before building:

```bash
git tag v1.0
git push origin v1.0
```
