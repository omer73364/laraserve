package cli

import "flag"

type serveOptions struct {
	fs            *flag.FlagSet
	setupLocal    *bool
	domainF       *string
	portF         *int
	host          *string
	httpF         *bool
	allow         *string
	noTrust       *bool
	noHosts       *bool
	copyF         *bool
	sandbox       *bool
	migrate       *bool
	mysqlF        *bool
	allowInsecure *bool
	phpVersionF   *string
	mysqlVersionF *string
	adminF        *bool
	hostPath      *bool
	verbose       *bool
	yes           *bool
}

func newServeOptions() *serveOptions {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	options := &serveOptions{fs: fs}
	options.setupLocal = fs.Bool("setup-local", false, "configure local DNS and trust without terminal prompts")
	options.domainF = fs.String("domain", "", "")
	options.portF = fs.Int("port", 0, "")
	options.host = fs.String("host", "127.0.0.1", "")
	options.httpF = fs.Bool("http", false, "")
	options.allow = fs.String("allow-domain", "", "")
	options.noTrust = fs.Bool("no-trust", false, "")
	options.noHosts = fs.Bool("no-hosts", false, "")
	options.copyF = fs.Bool("copy", false, "")
	options.sandbox = fs.Bool("sandbox", false, "")
	options.migrate = fs.Bool("migrate", false, "")
	options.mysqlF = fs.Bool("mysql", false, "")
	options.allowInsecure = fs.Bool("allow-insecure-dependencies", false, "allow Composer dependencies with known security advisories for legacy projects")
	options.phpVersionF = fs.String("php-version", "", "")
	options.mysqlVersionF = fs.String("mysql-version", "", "")
	options.adminF = fs.Bool("phpmyadmin", true, "")
	options.hostPath = fs.Bool("host-path", false, "")
	options.verbose = fs.Bool("verbose", false, "")
	options.yes = yesFlags(fs)
	return options
}

// Persist reusable settings, excluding one-shot schema/advisory consent and
// yes-to-all permission. Restart must never replay an explicit migration.
func rememberServeArgs(fs *flag.FlagSet, source string) []string {
	args := []string{source}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "migrate", "yes", "y", "allow-insecure-dependencies":
			return
		}
		args = append(args, "--"+f.Name+"="+f.Value.String())
	})
	return args
}

func rememberStartArgs(options *serveOptions) []string {
	var args []string
	options.fs.Visit(func(f *flag.Flag) { args = append(args, "--"+f.Name+"="+f.Value.String()) })
	return args
}
