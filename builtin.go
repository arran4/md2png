package md2png

import (
	"embed"
	"io/fs"
)

//go:embed skills/md2png/SKILL.md
var builtinSkillFS embed.FS

// BuiltinSkillFS returns the fs.FS for the builtin skill, scoped to its directory.
func BuiltinSkillFS() (fs.FS, error) {
	return fs.Sub(builtinSkillFS, "skills/md2png")
}
