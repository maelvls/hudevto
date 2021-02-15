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
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
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

	articles, err := client.Articles.ListMyUnpublishedArticles(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("fetching unpublished articles: %s", err)
	}
	articlesPub, err := client.Articles.ListMyPublishedArticles(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("fetching published articles: %s", err)
	}
	articles = append(articles, articlesPub...)
	articlesIdMap := make(map[int]*devto.ListedArticle)
	articlesTitleMap := make(map[string]*devto.ListedArticle)
	for i := range articles {
		art := &articles[i]
		articlesIdMap[int(art.ID)] = art
		articlesTitleMap[art.Title] = art
	}

	for _, page := range sites.Pages() {
		if page.Kind() != "page" {
			continue
		}

		draft := true
		draftRaw, err := page.Param("draft")
		if err == nil {
			draft = draftRaw.(bool)
		}
		if draft {
			continue
		}

		published := false
		publishedRaw, err := page.Param("devtoPublished")
		if publishedRaw == nil {
			logutil.Errorf("%s: missing devtoPublished field",
				logutil.Gray(rootDir+"/content/"+page.Path()),
			)
			continue
		}
		if err == nil {
			var ok bool
			published, ok = publishedRaw.(bool)
			if !ok {
				logutil.Errorf("%s: field devtoPublished is expected to be a boolean, got '%T'",
					logutil.Gray(rootDir+"/content/"+page.Path()),
					publishedRaw,
				)
				continue
			}
		}

		idRaw, err := page.Param("devtoId")
		if err != nil || idRaw == nil {
			if art, ok := articlesTitleMap[page.Title()]; ok {
				logutil.Errorf("%s missing devtoId field in front matter, might be %s: %s",
					logutil.Gray(rootDir+"/content/"+page.Path()),
					logutil.Green(strconv.Itoa(int(art.ID))),
					logutil.Yel(addEditSegment(art.URL.String(), published)),
				)
			} else {
				logutil.Errorf("%s missing devtoId field in front matter and title cannot be found on your devto account",
					logutil.Gray(rootDir+"/content/"+page.Path()),
				)
			}
			continue
		}
		id := idRaw.(int)

		article, found := articlesIdMap[id]
		if !found {
			if art, ok := articlesTitleMap[page.Title()]; ok {
				logutil.Errorf("%s: devtoId %s is unknown but title matches devtoId %s: %s",
					logutil.Gray(rootDir+"/content/"+page.Path()),
					logutil.Red(strconv.Itoa(id)),
					logutil.Green(strconv.Itoa(int(art.ID))),
					logutil.Yel(addEditSegment(art.URL.String(), published)),
				)
			} else {
				logutil.Errorf("%s: devtoId %s is unknown and title cannot be found in your devto account",
					logutil.Gray(rootDir+"/content/"+page.Path()),
					logutil.Red(strconv.Itoa(id)),
				)
			}
			continue
		}

		if article.Title != page.Title() {
			logutil.Errorf("titles do not match in %s:\ndevto: %s\nhugo:  %s",
				rootDir+"/content/"+page.Path(),
				article.Title,
				page.Title(),
			)
			continue
		}

		img := ""
		var imgs []string
		imgsRaw, err := page.Param("images")
		_, isEmptyArray := imgsRaw.([]interface{})
		if imgsRaw != nil && !isEmptyArray && err == nil {
			var ok bool
			imgs, ok = imgsRaw.([]string)
			if !ok {
				logutil.Errorf("%s: field images is expected to be an array of strings, got '%T'",
					logutil.Gray(rootDir+"/content/"+page.Path()),
					imgsRaw,
				)
				continue
			}
		}
		if len(imgs) > 0 {
			img = sites.AbsURL(imgs[0], false)
		}

		content := heredoc.Docf(`
			---
			title: "%s"
			description: "%s"
			published: %t
			tags: "%s"
			date: %s
			series: "%s"
			canonical_url: "%s"
			cover_image: "%s"
			---
			`,
			page.Title(),
			page.Description(),
			published,
			strings.Join(page.Keywords(), ", "),
			page.Date().UTC().Format("20060102T15:04Z"),
			strings.Join([]string{}, ", "),
			page.Permalink(),
			img,
		)

		content += page.RawContent()

	Update:
		art, err := UpdateArticle(httpClient, id, Article{BodyMarkdown: content})
		switch {
		case isTooManyRequests(err):
			// As per https://docs.forem.com/api/#operation/updateArticle,
			// there is a limit of 30 requests per 30 seconds.
			time.Sleep(1 * time.Second)
			goto Update
		case err != nil:
			logutil.Errorf("%s: updating devto id %s: %s",
				logutil.Gray(rootDir+"/content/"+page.Path()),
				logutil.Yel(strconv.Itoa(id)),
				err,
			)
			continue
		}

		fmt.Printf("%s: %s pushed to %s (devtoId: %d, devtoPublished: %t)\n",
			logutil.Green("success"),
			logutil.Gray(rootDir+"/content/"+page.Path()),
			logutil.Yel(addEditSegment(art.URL.String(), published)),
			art.ID,
			published,
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

	articles, err := client.Articles.ListMyUnpublishedArticles(context.Background(), &devto.MyArticlesOptions{})
	for _, article := range articles {
		fmt.Printf("%s %s %s\n",
			logutil.Gray(strconv.Itoa(int(article.ID))),
			logutil.Red(article.URL.String()+"/edit"),
			article.Title,
		)
	}
	if err != nil {
		return fmt.Errorf("listing all unpublished articles on dev.to: %w", err)
	}

	articles, err = client.Articles.ListMyPublishedArticles(context.Background(), &devto.MyArticlesOptions{})
	for _, article := range articles {
		fmt.Printf("%s %s %s\n",
			logutil.Gray(strconv.Itoa(int(article.ID))),
			logutil.Green(article.URL.String()),
			article.Title,
		)
	}
	if err != nil {
		return fmt.Errorf("listing all published articles on dev.to: %w", err)
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
	if err != nil {
		panic("unexpected: " + err.Error())
	}
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
	case 200:
		// continue below
	default:
		err = parseDevtoError(httpResp.StatusCode, bytes)
		return devto.Article{}, err
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
	BodyMarkdown string `json:"body_markdown"`
}

type DevtoError struct {
	Err    string `json:"error"`
	Status int    `json:"status"`
}

func (e DevtoError) Error() string { return e.Err }

func parseDevtoError(status int, bytes []byte) error {
	var errResp DevtoError
	if err := json.Unmarshal(bytes, &errResp); err != nil {
		return DevtoError{Status: status, Err: strings.TrimSpace(string(bytes))}
	}

	return errResp
}

func isTooManyRequests(err error) bool {
	if err == nil {
		return false
	}
	devtoErr, ok := err.(DevtoError)
	if !ok {
		return false
	}
	return devtoErr.Status == 429
}

// We want to have "/edit" at the end of URLs that are not yet published
// since these cannot be accessed without "/edit".
func addEditSegment(articleURL string, published bool) string {
	if !published {
		articleURL += "/edit"
	}
	return articleURL
}
