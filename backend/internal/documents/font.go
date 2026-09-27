package documents

// PDF rendering of the sprint documents: the demand to the management company
// (Д3) and, later, the meeting protocol (Д5). The font with Cyrillic is embedded
// into the binary, so the distroless runtime image needs no system fonts.

import (
	_ "embed"
)

//go:embed assets/DejaVuSans.ttf
var fontSans []byte

// FontSans returns the embedded DejaVu Sans font bytes (free licence:
// Bitstream Vera + public domain additions).
func FontSans() []byte { return fontSans }
