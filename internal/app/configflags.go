package app

import (
	"errors"
	"io/fs"

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
	warning  string // loose-permissions warning from resolve, shown by the pre-run hook
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
// default path being absent is not (zero-config falls back to defaults). A
// config file that stores a token with loose permissions records a warning —
// the token is a credential and should not be readable by other local users —
// which the root command's pre-run hook prints to stderr, styled.
func (f *configFlags) resolve() error {
	path := f.path
	var (
		cfg conf.Config
		err error
	)
	if path != "" {
		cfg, err = conf.Load(path)
	} else {
		path, err = conf.DefaultPath()
		if err != nil {
			return err
		}
		cfg, err = conf.Load(path)
		if errors.Is(err, fs.ErrNotExist) {
			cfg, err = conf.Config{}, nil
		}
	}
	if err != nil {
		return err
	}

	f.warning = conf.LoosePermissionsWarning(path, cfg)

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
