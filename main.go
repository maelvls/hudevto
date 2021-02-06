package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/VictorAvelar/devto-api-go/devto"
	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugolib"
	"github.com/spf13/jwalterweatherman"
	"github.com/spf13/viper"

	"github.com/maelvls/devto-to-hugo/logutil"
)

var (
	rootDir = flag.String("root", os.Getenv("PWD"), "Root directory of the Hugo project.")
	token   = flag.String("token", os.Getenv("DEVTO_TOKEN"), "The API key for Dev.to.")
)

func main() {
	flag.Parse()
	err := do(*rootDir, *token)
	if err != nil {
		logutil.Errorf(err.Error())
		os.Exit(1)
	}
}

func do(rootDir, token string) error {
	conf, err := loadHugoConfig(rootDir)
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

	err = sites.Build(hugolib.BuildCfg{SkipRender: true})
	if err != nil {
		return fmt.Errorf("Error Processing Source Content: %w", err)
	}

	if len(sites.Pages()) == 0 {
		logutil.Infof("no page found")
		return nil
	}

	for _, p := range sites.Pages() {
		if p.Kind() != "page" {
			logutil.Infof("not a page: %+v", p)
			continue
		}

		logutil.Infof("is a page: %+v", p)

		idRaw, err := p.Param("devto")
		if err != nil || idRaw == nil {
			logutil.Infof("⚠️  the page %s does not have the field 'devto' set in the front matter", p.Path())
			continue
		}
		id := idRaw.(int)

		logutil.Infof("page %s: devto = %v\n", p.Name(), id)

		client, err := devto.NewClient(context.Background(), &devto.Config{
			APIKey: token,
		}, http.DefaultClient, "https://dev.to")
		if err != nil {
			return fmt.Errorf("devto client: %w", err)
		}

		article, err := client.Articles.Find(context.Background(), uint32(id))
		if err != nil {
			logutil.Infof("skipping since the devto id %d is unknown in devto: %s", id, err)
			continue
		}

		fmt.Printf("%s\n", article.Description)
	}
	return nil
}

func loadHugoConfig(root string) (*viper.Viper, error) {
	if !filepath.IsAbs(root) {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(cwd, root)
	}

	config, _, err := hugolib.LoadConfig(hugolib.ConfigSourceDescriptor{
		Fs:         hugofs.Os,
		WorkingDir: root,
		Filename:   filepath.Join(root, "config.yaml"),
	})
	if err != nil {
		return nil, err
	}

	config.Set("workingDir", root)

	return config, nil
}
