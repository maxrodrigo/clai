// Clai is a UNIX-native CLI tool for AI text processing.
//
// It reads text from stdin or files, processes it through an AI model
// with a specified prompt, and writes the result to stdout.
//
// Usage:
//
//	clai [flags] <prompt> [files...]
//	clai prompt [list|show|path|add|update|remove|install]
//	clai strategy [list|show|path]
//	clai model [list]
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/maxrodrigo/clai/internal/commands"
	"github.com/maxrodrigo/clai/internal/config"
	"github.com/maxrodrigo/clai/internal/datadir"
	"github.com/maxrodrigo/clai/internal/output"
	"github.com/maxrodrigo/clai/internal/prompt"
	"github.com/maxrodrigo/clai/internal/source"
	"github.com/maxrodrigo/clai/internal/strategy"
	"github.com/maxrodrigo/clai/internal/version"

	// Register providers via init().
	_ "github.com/maxrodrigo/clai/internal/provider/anthropic"
	_ "github.com/maxrodrigo/clai/internal/provider/bedrock"
	_ "github.com/maxrodrigo/clai/internal/provider/google"
	_ "github.com/maxrodrigo/clai/internal/provider/openai"
)

func main() {
	out := output.New()
	in := &source.Input{
		Stdin:  os.Stdin,
		Stderr: output.WarnWriter(out.Stderr),
	}

	dataDir := datadir.Dir()
	version.DataDir = dataDir
	configDir := config.Dir()
	prompt.RegisterDefaultSources(dataDir, configDir, output.WarnWriter(out.Stderr))
	strategy.Init(dataDir, configDir, output.WarnWriter(out.Stderr))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	root := commands.NewRoot(out, in)
	root.SetContext(ctx)
	commands.Execute(root, out)
}
