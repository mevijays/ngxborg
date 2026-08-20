package cli

import (
	"context"
	"flag"
	"fmt"

	"ngxborg/internal/system"
	"ngxborg/internal/webui"
)

// cmdWeb runs the web UI in the foreground. `ngxborg install service`'s
// systemd unit is what actually invokes this in normal operation; running
// it directly is mainly for development and for `ngxborg web` run by hand
// to see startup errors immediately rather than through journalctl.
func cmdWeb(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	addr := fs.String("addr", ":8443", "listen address")
	insecure := fs.Bool("insecure", false, "run without TLS (plain HTTP, for use behind a reverse proxy)")
	tlsMode := fs.String("tls", "self-signed", "TLS mode: self-signed | custom | none")
	certPath := fs.String("tls-cert", "", "path to TLS certificate (required when --tls=custom)")
	keyPath := fs.String("tls-key", "", "path to TLS private key (required when --tls=custom)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Resolve effective TLS mode: --insecure overrides --tls to "none".
	effectiveTLS := *tlsMode
	if *insecure {
		effectiveTLS = "none"
	}

	// Validate TLS options.
	switch effectiveTLS {
	case "self-signed", "none":
		// No extra flags needed.
	case "custom":
		if *certPath == "" || *keyPath == "" {
			return fmt.Errorf("--tls=custom requires --tls-cert and --tls-key")
		}
	default:
		return fmt.Errorf("unknown --tls value %q: use self-signed, custom, or none", *tlsMode)
	}

	if err := system.RequireRoot(); err != nil {
		return err
	}

	if effectiveTLS == "custom" {
		return webui.ServeWithCerts(ctx, *addr, effectiveTLS, *certPath, *keyPath)
	}
	return webui.Serve(ctx, *addr, effectiveTLS)
}
