package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"laraserve/internal/detect"
	"laraserve/internal/hosts"
	"laraserve/internal/pack"
	"laraserve/internal/paths"
	"laraserve/internal/projects"
	"laraserve/internal/supervisor"
	"laraserve/internal/trust"
	"laraserve/internal/ui"
)

func cmdPack(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	out := fs.String("o", "", "")
	incEnv := fs.Bool("include-env", false, "")
	if err := parse(fs, args); err != nil {
		return err
	}
	target := "."
	if fs.NArg() > 0 {
		target = fs.Arg(0)
	}
	root, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	proj, err := detect.Inspect(root)
	if err != nil {
		return err
	}
	if !proj.HasVendor {
		return fmt.Errorf("vendor/ is missing: run `composer install` (and your asset build) first, then pack")
	}
	if _, err := os.Stat(filepath.Join(root, "public", "build")); err != nil {
		ui.Warn("public/build not found: if the app uses Vite/Mix, build assets before packing")
	}
	if *out == "" {
		*out = filepath.Base(root) + ".laraserve"
	}
	if err := pack.Create(root, *out, *incEnv); err != nil {
		return err
	}
	st, _ := os.Stat(*out)
	ui.OK("Created %s (%.1f MB)", *out, float64(st.Size())/1e6)
	if !*incEnv {
		ui.Info(".env was not included (use --include-env to add it)")
	}
	return nil
}

func cmdTrust(args []string) error {
	fs := flag.NewFlagSet("trust", flag.ContinueOnError)
	yes := yesFlags(fs)
	if err := parse(fs, args); err != nil {
		return err
	}
	ui.Yes = *yes
	c, err := load()
	if err != nil {
		return err
	}
	_, err = c.ensureTrust(nil, false, true)
	return err
}

func cmdUntrust(args []string) error {
	c, err := load()
	if err != nil {
		return err
	}
	if c.man.CA == nil {
		ui.Info("Nothing to remove: the local CA is not installed")
		return nil
	}
	if err := trust.Remove(c.man.CA); err != nil {
		return err
	}
	c.man.CA = nil
	ui.OK("Local CA removed from the system trust store")
	return c.man.Save()
}

func cmdClean(args []string) error {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	if err := parse(fs, args); err != nil {
		return err
	}
	c, err := load()
	if err != nil {
		return err
	}
	target := "."
	if fs.NArg() > 0 {
		target = fs.Arg(0)
	}
	abs, _ := filepath.Abs(target)
	if p, err := projects.New(c.home.Root).Find(abs); err == nil {
		if err := requireStopped(c.home.Root, p.ID); err != nil {
			return err
		}
	}
	found := false
	for id, p := range c.man.Projects {
		ws := filepath.Join(c.home.Workspaces(), paths.ProjectID(abs))
		if p.Path == abs || p.Path == ws {
			c.cleanProject(id)
			found = true
		}
	}
	if !found {
		ui.Info("laraserve has nothing recorded for %s", abs)
	}
	return c.man.Save()
}

func (c *ctx) cleanProject(id string) {
	p := c.man.Projects[id]
	if p == nil {
		return
	}
	if p.Vendor && !p.Workspace {
		_ = os.RemoveAll(filepath.Join(p.Path, "vendor"))
		ui.OK("Removed %s", filepath.Join(p.Path, "vendor"))
	}
	if p.Workspace {
		_ = os.RemoveAll(p.Path)
		ui.OK("Removed workspace %s", p.Path)
	}
	_ = os.RemoveAll(filepath.Join(c.home.Projects(), id))
	delete(c.man.Projects, id)
	for _, d := range p.Domains {
		used := false
		for _, o := range c.man.Projects {
			for _, od := range o.Domains {
				used = used || od == d
			}
		}
		if used {
			continue
		}
		for _, h := range c.man.Hosts {
			if h == d {
				if err := hosts.Remove(d); err != nil {
					ui.Warn("could not remove %s from the hosts file: %v", d, err)
				} else {
					c.man.RemoveHost(d)
					ui.OK("Removed %s from the hosts file", d)
				}
			}
		}
	}
}

func cmdUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	yes := yesFlags(fs)
	if err := parse(fs, args); err != nil {
		return err
	}
	ui.Yes = *yes
	c, err := load()
	if err != nil {
		return err
	}
	owners, err := supervisor.States(c.home.Root)
	if err != nil {
		return err
	}
	for _, state := range owners {
		if err := requireStopped(c.home.Root, state.ID); err != nil {
			return err
		}
	}
	registered, err := projects.New(c.home.Root).List()
	if err != nil {
		return err
	}
	for _, p := range registered {
		if err := requireStopped(c.home.Root, p.ID); err != nil {
			return err
		}
	}
	if !ui.Confirm(fmt.Sprintf("Remove everything laraserve created (including %s)?", c.home.Root)) {
		return nil
	}
	for id := range c.man.Projects {
		c.cleanProject(id)
	}
	for _, h := range append([]string{}, c.man.Hosts...) {
		if err := hosts.Remove(h); err != nil {
			ui.Warn("hosts: %v", err)
		}
	}
	if c.man.CA != nil {
		if err := trust.Remove(c.man.CA); err != nil {
			ui.Warn("could not remove the CA from the trust store: %v", err)
		} else {
			ui.OK("Local CA removed from the system trust store")
		}
	}
	if err := os.RemoveAll(c.home.Root); err != nil {
		return err
	}
	ui.OK("Removed %s. laraserve left nothing behind.", c.home.Root)
	return nil
}

func requireStopped(home, id string) error {
	state, err := supervisor.Status(home, id)
	if err != nil {
		return err
	}
	switch state.Status {
	case "starting", "running", "stopping", "unresponsive":
		return fmt.Errorf("stop project %s before removing its files", id)
	}
	return nil
}
