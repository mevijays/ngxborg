package cli

import (
	"context"
	"flag"

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
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := system.RequireRoot(); err != nil {
		return err
	}
	return webui.Serve(ctx, *addr)
}
