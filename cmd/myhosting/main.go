package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/oneworkspacestudio/my-hosting/internal/hosting"
	"github.com/oneworkspacestudio/my-hosting/internal/webui"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	data := flag.String("data", "data", "Private data folder on local NTFS")
	listen := flag.String("listen", "127.0.0.1:9007", "Loopback management listener")
	origin := flag.String("origin", "", "Public dashboard address, optionally including /hosting")
	issuer := flag.String("issuer", os.Getenv("WORKSPACE_OIDC_ISSUER"), "MyAccounts issuer (discovery)")
	client := flag.String("client-id", os.Getenv("WORKSPACE_OIDC_CLIENT_ID"), "Registered OIDC client ID")
	noBrowser := flag.Bool("no-browser", false, "Run without opening a browser")
	email := flag.String("email", "", "Standalone bootstrap email")
	passwordStdin := flag.Bool("password-stdin", false, "Read bootstrap/backup password from standard input")
	backup := flag.String("backup", "", "Encrypted backup to restore")
	restoreTo := flag.String("restore-to", "", "New NTFS private-data folder for restore")
	version := flag.Bool("version", false, "Print version")
	flag.Parse()
	if *version || flag.Arg(0) == "version" {
		fmt.Println(hosting.Version)
		return nil
	}
	abs, e := filepath.Abs(*data)
	if e != nil {
		return e
	}
	if *origin == "" {
		*origin = "http://" + *listen
	}
	cfg := hosting.Config{Data: abs, Listen: *listen, Origin: strings.TrimRight(*origin, "/"), Issuer: *issuer, ClientID: *client, ClientSecret: os.Getenv("WORKSPACE_OIDC_CLIENT_SECRET"), Owner: os.Getenv("WORKSPACE_OWNER_SUBJECT")}
	password := func() (string, error) {
		if !*passwordStdin {
			return "", errors.New("Use --password-stdin; credentials are never accepted in command-line arguments")
		}
		b, e := io.ReadAll(io.LimitReader(os.Stdin, 2049))
		if e != nil {
			return "", e
		}
		if len(b) > 2048 {
			return "", errors.New("Password is too long")
		}
		return strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r"), nil
	}
	switch flag.Arg(0) {
	case "bootstrap":
		p, e := password()
		if e != nil {
			return e
		}
		if e = hosting.Bootstrap(cfg, *email, p); e != nil {
			return e
		}
		fmt.Println("Standalone owner created. Start MyHosting.exe with the same --data path.")
		return nil
	case "restore":
		p, e := password()
		if e != nil {
			return e
		}
		if *backup == "" || *restoreTo == "" {
			return errors.New("Supply --backup and a new --restore-to NTFS folder")
		}
		if e = hosting.Restore(*backup, *restoreTo, p); e != nil {
			return e
		}
		fmt.Println("Backup restored to the new folder. Configure this host's origin and identity provider before starting.")
		return nil
	case "", "run":
	default:
		return errors.New("Commands: run, bootstrap, restore, version. Flags precede the command.")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	return hosting.Run(ctx, cfg, webui.Files(), !*noBrowser)
}
