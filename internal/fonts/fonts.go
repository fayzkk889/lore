// Package fonts embeds the TTF font files used for PDF generation.
// The fonts are bundled into the binary at compile time so that the
// lore binary is fully self-contained — no font files need to be
// present on the user's system.
//
// Fonts used:
//   - Inter (Regular + Bold) — body text, SIL Open Font License
//   - JetBrains Mono (Regular) — code blocks, SIL Open Font License
package fonts

import _ "embed"

// InterRegular is the Inter-Regular.ttf font file, embedded at compile time.
//
//go:embed Inter-Regular.ttf
var InterRegular []byte

// InterBold is the Inter-Bold.ttf font file, embedded at compile time.
//
//go:embed Inter-Bold.ttf
var InterBold []byte

// JetBrainsMonoRegular is the JetBrainsMono-Regular.ttf file, embedded at compile time.
//
//go:embed JetBrainsMono-Regular.ttf
var JetBrainsMonoRegular []byte
