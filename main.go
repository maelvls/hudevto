package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/VictorAvelar/devto-api-go/devto"
	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugolib"
	"github.com/sethgrid/gencurl"
	"github.com/spf13/jwalterweatherman"
	"github.com/spf13/viper"

	"github.com/maelvls/hudevto/logutil"
)

var (
	rootDir = flag.String("root", "", "Root directory of the Hugo project.")
	apiKey  = flag.String("apikey", os.Getenv("DEVTO_APIKEY"), "The API key for Dev.to.")
	debug   = flag.Bool("debug", false, "Print debug information.")
)

func main() {
	flag.Parse()
	logutil.EnableDebug = *debug

	if *apiKey == "" {
		logutil.Errorf("no API key given, either give it with --apikey or with DEVTO_APIKEY")
	}

	switch flag.Arg(0) {
	case "push":
		err := PushArticlesFromHugoToDevto(filepath.Clean(*rootDir), *apiKey)
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
		logutil.Errorf("unknown command '%s', try list or push", flag.Arg(0))
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

	httpClient := http.DefaultClient
	httpClient.Transport = curlDebug(http.DefaultTransport, logutil.EnableDebug, apiKey)
	client, err := devto.NewClient(context.Background(), &devto.Config{
		APIKey: apiKey,
	}, httpClient, "https://dev.to")
	if err != nil {
		return fmt.Errorf("devto client: %w", err)
	}

	for _, page := range sites.Pages() {
		if page.Kind() != "page" {
			continue
		}

		idRaw, err := page.Param("devto")
		if err != nil || idRaw == nil {
			logutil.Debugf("⚠️  the page %s does not have the field 'devto' set in the front matter", rootDir+"/content/"+page.Path())
			continue
		}
		id := idRaw.(int)

		articles, err := client.Articles.ListAllMyArticles(context.Background(), nil)
		if err != nil {
			logutil.Errorf("%s: %s: unknown error: %s", logutil.Gray(strconv.Itoa(id)), rootDir+"/content/"+page.Path(), err)
			continue
		}
		articlesMap := make(map[int]*devto.ListedArticle)
		for i := range articles {
			art := &articles[i]
			articlesMap[int(art.ID)] = art
		}

		article, found := articlesMap[id]
		if !found {
			logutil.Errorf("%s is not a known devto article id: %s", logutil.Gray(strconv.Itoa(id)), rootDir+"/content/"+page.Path())
			continue
		}

		if article.Title != page.Title() {
			logutil.Errorf("titles do not match in %s:\ndevto: %s\nhugo:  %s", rootDir+"/content/"+page.Path(), article.Title, page.Title())
			continue
		}

		// img := ""
		// var imgs []string
		// imgsRaw, err := page.Param("images")
		// if err != nil {
		// 	imgs = imgsRaw.([]string)
		// }
		// if len(imgs) > 0 {
		// 	img = imgs[0]
		// }

		art, err := UpdateArticle(httpClient, id, Article{
			Title:        page.Title(),
			BodyMarkdown: page.RawContent(),
			// Description:  page.Description(),
			Published: page.Draft(),
			// MainImage:    img,
			// BodyMarkdown: page.RawContent(),
			// CanonicalURL: page.Permalink(),
			// Tags:         page.Keywords(),
		})
		if err != nil {
			logutil.Errorf("updating %s with the content in %s: %s", logutil.Gray(strconv.Itoa(id)), rootDir+"/content/"+page.Path(), err)
			continue
		}

		fmt.Printf("%s %s %s updated: %s\n",
			logutil.Gray(strconv.Itoa(int(article.ID))),
			logutil.Bold(rootDir+"/content/"+page.Path()),
			article.Title,
			logutil.Yel(art.URL.String()),
		)
	}
	return nil
}

func PrintDevtoArticles(apiKey string) error {
	httpClient := http.DefaultClient
	httpClient.Transport = curlDebug(http.DefaultTransport, logutil.EnableDebug, apiKey)
	client, err := devto.NewClient(context.Background(), &devto.Config{
		APIKey: apiKey,
	}, httpClient, "https://dev.to")
	if err != nil {
		return fmt.Errorf("devto client: %w", err)
	}

	articles, err := client.Articles.ListAllMyArticles(context.Background(), nil)
	for _, article := range articles {
		fmt.Printf("%s %s\n",
			logutil.Gray(strconv.Itoa(int(article.ID))),
			article.Title,
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

func isNotFound(err error) bool {
	var devtoErr *devto.ErrorResponse
	if !errors.As(err, &devtoErr) {
		return false
	}
	return devtoErr.Status == 404
}

func curlDebug(rt http.RoundTripper, debug bool, apiKey string) http.RoundTripper {
	return &transport{wrapped: rt, outputCurl: debug, apiKey: apiKey}
}

type transport struct {
	wrapped    http.RoundTripper
	outputCurl bool
	apiKey     string
}

func (rt transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if rt.outputCurl {
		logutil.Debugf("%s", gencurl.FromRequest(r))
	}

	r.Header.Set("Accept", "application/json")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Api-Key", rt.apiKey)

	return rt.wrapped.RoundTrip(r)
}

func UpdateArticle(client *http.Client, articleID int, article Article) (devto.Article, error) {
	articleReq := ArticleReq{Article: article}
	raw, err := json.Marshal(&articleReq)
	reader := bytes.NewReader(raw)

	path := fmt.Sprintf("/api/articles/%d", articleID)
	req, err := http.NewRequest("PUT", "https://dev.to"+path, reader)
	if err != nil {
		return devto.Article{}, fmt.Errorf("creating HTTP request for GET %s: %w", path, err)
	}

	httpResp, err := client.Do(req)
	if err != nil {
		return devto.Article{}, fmt.Errorf("while doing GET %s: %w", path, err)
	}
	defer httpResp.Body.Close()

	bytes, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return devto.Article{}, fmt.Errorf("while reading HTTP response for %s: %w", path, err)
	}

	switch httpResp.StatusCode {
	case 400, 401, 403, 422, 429, 500:
		err = bytesToError(bytes)
		return devto.Article{}, err
	case 200:
		// continue below
	default:
		return devto.Article{}, fmt.Errorf("unxpected HTTP status code %d for GET %s: %s", httpResp.StatusCode, path, bytes)
	}

	var art devto.Article
	err = json.Unmarshal(bytes, &art)
	if err != nil {
		return devto.Article{}, fmt.Errorf("while parsing JSON from the HTTP response for GET %s: %w", path, err)
	}

	return art, nil
}

type ArticleReq struct {
	Article Article `json:"article"`
}

type Article struct {
	Title        string   `json:"title"`
	Published    bool     `json:"published"`
	BodyMarkdown string   `json:"body_markdown"`
	Tags         []string `json:"tags"`
	Series       string   `json:"series"`
	CanonicalURL string   `json:"canonical_url"`
}

type DevtoError struct {
	Err    string `json:"error"`
	Status int    `json:"status"`
}

func (e DevtoError) Error() string { return e.Err }

func bytesToError(bytes []byte) error {
	var errResp DevtoError
	if err := json.Unmarshal(bytes, &errResp); err != nil {
		return fmt.Errorf("(raw body) %s", string(bytes))
	}

	return errResp
}
