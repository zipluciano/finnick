// Command finnick is a local UI and CLI for repositories that keep secrets with SOPS.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/zipluciano/finnick/internal/server"
	"github.com/zipluciano/finnick/internal/workspace"
	"github.com/zipluciano/finnick/web"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `finnick: a local UI for SOPS-encrypted secrets.

Usage:
  finnick ui [--dir DIR] [--port N] [--no-open]   open the UI in your browser
  finnick ls [--dir DIR] [--json]                 list encrypted files and their status
  finnick version

Run it anywhere inside a repository: it finds .sops.yaml in the current directory or
its parents, exactly like sops, and opens that repository. DIR defaults to ".".
`

func main() {
	workspace.PrepareAgeEnv()
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "ui":
		err = runUI(os.Args[2:])
	case "ls":
		err = runLs(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("finnick", version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runUI(args []string) error {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	dir := fs.String("dir", ".", "directory inside the repository")
	port := fs.Int("port", 0, "port on 127.0.0.1 (0 = random free port)")
	noOpen := fs.Bool("no-open", false, "do not open the browser")
	fs.Parse(args)

	ws, err := workspace.Open(*dir)
	if err != nil {
		return err
	}
	srv := server.New(ws, web.Assets(), version)
	ln, url, err := srv.Listen(*port)
	if err != nil {
		return err
	}
	httpSrv := srv.Serve(ln)

	fmt.Printf("finnick %s\n  repository: %s\n", version, ws.Root)
	if cfg := ws.Config(); cfg == nil {
		fmt.Println("  config:     no .sops.yaml found; listing encrypted files only")
	} else {
		fmt.Printf("  config:     %s\n", cfg.Path)
	}
	fmt.Printf("\n  Open: %s\n\n  The link works for this session only. Press Ctrl+C to stop.\n", url)
	if !*noOpen {
		openBrowser(url)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return httpSrv.Shutdown(ctx)
}

func runLs(args []string) error {
	fs := flag.NewFlagSet("ls", flag.ExitOnError)
	dir := fs.String("dir", ".", "directory inside the repository")
	asJSON := fs.Bool("json", false, "print JSON")
	fs.Parse(args)

	ws, err := workspace.Open(*dir)
	if err != nil {
		return err
	}
	files, err := ws.Scan(workspace.LocalIdentity())
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(files)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "FILE\tFORMAT\tKEYS\tRULE\tACCESS\tRECIPIENTS")
	for _, f := range files {
		rule := "-"
		if f.Rule >= 0 {
			rule = fmt.Sprint(f.Rule)
		}
		access := "locked"
		if f.CanDecrypt {
			access = "yes"
		}
		var names []string
		for _, g := range f.KeyGroups {
			for _, r := range g {
				if r.Name != "" {
					names = append(names, r.Name)
				} else {
					names = append(names, r.Type+":"+short(r.ID))
				}
			}
		}
		status := strings.Join(names, ", ")
		if f.Drift != nil {
			status += "  (out of sync with .sops.yaml)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\n", f.Path, f.Format, len(f.Keys), rule, access, status)
	}
	return tw.Flush()
}

func short(id string) string {
	if len(id) > 16 {
		return id[:10] + "…" + id[len(id)-4:]
	}
	return id
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
