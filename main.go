package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/VictorAvelar/devto-api-go/devto"
	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugolib"
	"github.com/spf13/jwalterweatherman"
	"github.com/spf13/viper"

	"github.com/maelvls/hudevto/logutil"
)

var (
	rootDir = flag.String("root", os.Getenv("PWD"), "Root directory of the Hugo project.")
	apiKey  = flag.String("apikey", os.Getenv("DEVTO_APIKEY"), "The API key for Dev.to.")
	debug   = flag.Bool("debug", false, "Print debug information.")
)

func main() {
	flag.Parse()
	logutil.EnableDebug = *debug

	logutil.Debugf("ok")

	switch flag.Arg(0) {
	case "update":
		err := PushArticlesFromHugoToDevto(*rootDir, *apiKey)
		if err != nil {
			logutil.Errorf(err.Error())
			os.Exit(1)
		}
	case "list":
		err := PrintDevtoArticles(*apiKey)
		if err != nil {
			logutil.Errorf(err.Error())
			os.Exit(1)
		}
	default:
		logutil.Errorf("unknown command %s, try list or update")
	}
}

func PushArticlesFromHugoToDevto(rootDir, apiKey string) error {
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
		logutil.Errorf("no page found")
		return nil
	}

	client, err := devto.NewClient(context.Background(), &devto.Config{
		APIKey: apiKey,
	}, http.DefaultClient, "https://dev.to")
	if err != nil {
		return fmt.Errorf("devto client: %w", err)
	}

	for _, p := range sites.Pages() {
		if p.Kind() != "page" {
			logutil.Debugf("not a page: %+v", p)
			continue
		}

		logutil.Debugf("is a page: %+v", p)

		idRaw, err := p.Param("devto")
		if err != nil || idRaw == nil {
			logutil.Infof("⚠️  the page %s does not have the field 'devto' set in the front matter", p.Path())
			continue
		}
		id := idRaw.(int)

		logutil.Infof("page %s: devto = %v\n", p.Title(), id)

		article, err := client.Articles.Find(context.Background(), uint32(id))
		if err != nil {
			logutil.Infof("skipping since the devto id %d is unknown in devto: %s", id, err)
			continue
		}

		fmt.Printf("title: %s, descr: %s\n", article.Title, article.Description)
		if article.Title != p.Title() {
			logutil.Errorf("the devto title does not match the hugo page:\ndevto: %s\nhugo:  %s", article.Title, p.Title())
			continue
		}
	}
	return nil
}

func PrintDevtoArticles(apiKey string) error {
	client, err := devto.NewClient(context.Background(), &devto.Config{
		APIKey: apiKey,
	}, http.DefaultClient, "https://dev.to")
	if err != nil {
		return fmt.Errorf("devto client: %w", err)
	}

	articles, err := client.Articles.ListAllMyArticles(context.Background(), nil)
	for _, article := range articles {
		fmt.Printf("%s %s\n",
			logutil.Gray(strconv.Itoa(int(article.ID))),
			logutil.Yel(article.Title),
		)
	}
	if err != nil {
		return fmt.Errorf("listing all your articles on dev.to: %w", err)
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
