package md2png

//go:generate go run github.com/arran4/go-subcommand/cmd/gosubc@v0.0.29 generate --timestamp=false --project-provenance=false
//go:generate rm -f cmd/md2png/root_test.go cmd/md2view/root_test.go
//go:generate sed -i "/go:generate/d" cmd/md2png/main.go cmd/md2view/main.go
