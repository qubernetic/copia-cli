package main

import (
	"fmt"
	"os"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/internal/copiacmd"
	"github.com/qubernetic/copia-cli/internal/docs"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

func main() {
	outDir := "docs/site/docs/manual"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	navFile := "docs/site/commands-nav.json"
	if len(os.Args) > 2 {
		navFile = os.Args[2]
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output dir: %v\n", err)
		os.Exit(1)
	}

	ios := iostreams.System()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config: func() (*config.Config, error) {
			return &config.Config{Hosts: map[string]*config.HostConfig{}}, nil
		},
	}

	rootCmd := copiacmd.NewRootCmd(f)
	rootCmd.DisableAutoGenTag = true

	if err := docs.GenMarkdownTree(rootCmd, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating docs: %v\n", err)
		os.Exit(1)
	}

	if err := docs.WriteNav(rootCmd, navFile, "manual/"); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating navigation: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated docs in %s\n", outDir)
	fmt.Printf("Generated navigation in %s\n", navFile)
}
