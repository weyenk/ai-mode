package main

import (
	"os"

	"ai-mode/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:]))
}
