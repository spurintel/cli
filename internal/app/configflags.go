package app

import (
	"github.com/spf13/cobra"

	"github.com/spurintel/cli/internal/conf"
	"github.com/spurintel/cli/internal/spur"
)

// configFlags backs the root command's persistent config flags. It loads the
// TOML config, applies the selected profile, and exposes the resolved settings
// (and a preconfigured client) to the subcommands.
type configFlags struct {
	path     string // --config; empty means the default location
	profile  string // --profile; empty means no profile
	resolved conf.Resolved
}

// register binds --config and --profile onto cmd's persistent flag set.
func (f *configFlags) register(cmd *cobra.Command) {
	pf := cmd.PersistentFlags()
	pf.StringVar(&f.path, "config", "",
		"path to the TOML config file (default: $XDG_CONFIG_HOME/spur/config.toml)")
	pf.StringVar(&f.profile, "profile", "",
		"select a named profile from the config")
}

// resolve loads the config and merges the selected profile over the built-in
// production defaults. An explicit --config that is missing is an error; the
// default path being absent is not (zero-config falls back to defaults).
func (f *configFlags) resolve() error {
	var (
		cfg conf.Config
		err error
	)
	if f.path != "" {
		cfg, err = conf.Load(f.path)
	} else {
		cfg, err = conf.LoadDefault()
	}
	if err != nil {
		return err
	}

	r, err := cfg.Resolve(f.profile, conf.Resolved{
		BaseURL:        spur.BaseURL,
		FeedsBaseURL:   spur.FeedsBaseURL,
		ExportsBaseURL: spur.ExportsBaseURL,
	})
	if err != nil {
		return err
	}
	f.resolved = r
	return nil
}

// newClient builds a Spur client with the resolved base URL applied, so an
// endpoint override in the config points every request at the chosen host.
func (f *configFlags) newClient(token string) *spur.Client {
	c := spur.NewClient(token)
	if f.resolved.BaseURL != "" {
		c.Base = f.resolved.BaseURL
	}
	if f.resolved.FeedsBaseURL != "" {
		c.FeedsBase = f.resolved.FeedsBaseURL
	}
	if f.resolved.ExportsBaseURL != "" {
		c.ExportsBase = f.resolved.ExportsBaseURL
	}
	return c
}
