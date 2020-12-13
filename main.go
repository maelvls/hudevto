package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugolib"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/spf13/jwalterweatherman"
	"github.com/spf13/viper"
)

func main() {
	_ = do()
}

func do() error {
	conf, err := loadHugoConfig(os.Getenv("PWD"))
	if err != nil {
		return err
	}

	sites, err := hugolib.NewHugoSites(deps.DepsCfg{
		Logger: loggers.NewBasicLogger(jwalterweatherman.LevelTrace),
		Fs:     hugofs.NewDefault(conf),
		Cfg:    conf,
	})
	if err != nil {
		return fmt.Errorf("Error creating sites: %w", err)
	}

	if err := sites.Build(hugolib.BuildCfg{SkipRender: true}); err != nil {
		return fmt.Errorf("Error Processing Source Content: %w", err)
	}

	dfs(sites.Pages())

	return nil
}

func dfs(pages page.Pages) {
	for _, p := range pages {
		if !p.IsPage() {
			continue
		}
		fmt.Printf("%s\n", p.Name())
	}
}

func loadHugoConfig(root string) (*viper.Viper, error) {

	if !filepath.IsAbs(root) {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(cwd, root)
	}

	cfgOpts := hugolib.ConfigSourceDescriptor{
		Fs: hugofs.Os,
		/* Path: c.h.source, */
		WorkingDir: root,
		Filename:   filepath.Join(root, "config.yaml"),
	}

	config, _, err := hugolib.LoadConfig(cfgOpts)

	config.Set("workingDir", root)

	return config, err
}
