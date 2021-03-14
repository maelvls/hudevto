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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/VictorAvelar/devto-api-go/devto"
	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugolib"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/sethgrid/gencurl"
	"github.com/spf13/jwalterweatherman"
	"github.com/spf13/viper"

	"github.com/maelvls/hudevto/logutil"
)

var (
	rootDir    = flag.String("root", "", "Root directory of the Hugo project.")
	apiKeyFlag = flag.String("apikey", "", "The API key for Dev.to. You can also set DEVTO_APIKEY instead.")
	debug      = flag.Bool("debug", false, "Print debug information such as the HTTP requests that are being made in curl format.")
)

func main() {
	flag.Parse()
	logutil.EnableDebug = *debug

	apiKey := os.Getenv("DEVTO_APIKEY")
	if *apiKeyFlag != "" {
		apiKey = *apiKeyFlag
	}
	if apiKey == "" {
		logutil.Errorf("no API key given, either give it with --apikey or with DEVTO_APIKEY")
	}

	switch flag.Arg(0) {
	case "push":
		err := PushArticlesFromHugoToDevto(filepath.Clean(*rootDir), flag.Arg(1), false, false, apiKey)
		if err != nil {
			logutil.Errorf(err.Error())
			os.Exit(1)
		}
	case "preview":
		err := PushArticlesFromHugoToDevto(filepath.Clean(*rootDir), flag.Arg(1), true, false, apiKey)
		if err != nil {
			logutil.Errorf(err.Error())
			os.Exit(1)
		}
	case "diff":
		err := PushArticlesFromHugoToDevto(filepath.Clean(*rootDir), flag.Arg(1), false, true, apiKey)
		if err != nil {
			logutil.Errorf(err.Error())
			os.Exit(1)
		}
	case "list":
		err := PrintDevtoArticles(apiKey)
		if err != nil {
			logutil.Errorf(err.Error())
			os.Exit(1)
		}
	default:
		logutil.Errorf("unknown command '%s'.", flag.Arg(0))
		fmt.Fprintf(os.Stderr, heredoc.Doc(`
			Usage:
			  hudevto preview
			  hudevto push
			  hudevto list
		`))
	}
}

// Updates all articles if pathToArticle is left empty.
func PushArticlesFromHugoToDevto(rootDir, pathToArticle string, showMarkdown, showDiff bool, apiKey string) error {
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

	articles, err := listAllMyArticles(client)
	if err != nil {
		return fmt.Errorf("listing all the user's articles: %w", err)
	}

	articlesIdMap := make(map[int]*devto.ListedArticle)
	articlesTitleMap := make(map[string]*devto.ListedArticle)
	for i := range articles {
		art := &articles[i]
		articlesIdMap[int(art.ID)] = art
		articlesTitleMap[art.Title] = art
	}

	pages := sites.Pages()
	if pathToArticle != "" {
		path := sites.AbsPathify(pathToArticle)
		if path == "" {
			return fmt.Errorf("%s not found", logutil.Gray(pathToArticle))
		}
		p := sites.GetContentPage(path)
		if p == nil {
			return fmt.Errorf("%s was found but does not seem to be a page", logutil.Gray(pathToArticle))
		}

		pages = []page.Page{p}
	}

	for _, page := range pages {
		if page.Kind() != "page" {
			continue
		}

		pathToMD := rootDir + "/content/" + page.Path()

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
				logutil.Gray(pathToMD),
			)
			continue
		}
		if err == nil {
			var ok bool
			published, ok = publishedRaw.(bool)
			if !ok {
				logutil.Errorf("%s: field devtoPublished is expected to be a boolean, got '%T'",
					logutil.Gray(pathToMD),
					publishedRaw,
				)
				continue
			}
		}

		idRaw, err := page.Param("devtoId")
		if err != nil || idRaw == nil {
			if art, ok := articlesTitleMap[page.Title()]; ok {
				logutil.Errorf("%s missing devtoId field in front matter, might be %s: %s",
					logutil.Gray(pathToMD),
					logutil.Green(strconv.Itoa(int(art.ID))),
					logutil.Yel(addEditSegment(art.URL.String(), published)),
				)
			} else {
				logutil.Errorf("%s missing devtoId field in front matter and title cannot be found on your devto account",
					logutil.Gray(pathToMD),
				)
			}
			continue
		}
		id, ok := idRaw.(int)
		if !ok {
			logutil.Errorf("%s: field devtoId is expected to be an integer, got '%T'",
				logutil.Gray(pathToMD),
				idRaw,
			)
			continue
		}

		article, found := articlesIdMap[id]
		if !found {
			if art, ok := articlesTitleMap[page.Title()]; ok {
				logutil.Errorf("%s: devtoId %s is unknown but title matches devtoId %s: %s",
					logutil.Gray(pathToMD),
					logutil.Red(strconv.Itoa(id)),
					logutil.Green(strconv.Itoa(int(art.ID))),
					logutil.Yel(addEditSegment(art.URL.String(), published)),
				)
			} else {
				logutil.Errorf("%s: devtoId %s is unknown and title cannot be found in your devto account",
					logutil.Gray(pathToMD),
					logutil.Red(strconv.Itoa(id)),
				)
			}
			continue
		}

		if article.Title != page.Title() {
			logutil.Errorf(heredoc.Docf(`
				there seems to be a title mismatch in %s.
				%s dev.to title
				%s hugo title
				%s
				%s
				To fix the mismatch, go to: %s`,
				pathToMD,
				logutil.Cyan("---"),
				logutil.Cyan("+++"),
				logutil.Cyan("- ")+logutil.Red(article.Title),
				logutil.Cyan("+ ")+logutil.Green(page.Title()),
				logutil.Yel(addEditSegment(article.URL.String(), published)),
			))
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
					logutil.Gray(pathToMD),
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

		body := page.RawContent()
		body = convertHugoToLiquid(body)
		body = addPostURLInImages(body, page.Permalink())

		content += body

		if showMarkdown {
			fmt.Print(content)
			return nil
		}

		existing, err := GetArticle(httpClient, id)
		if err != nil {
			logutil.Errorf("%s: fetching devto id %s: %s",
				logutil.Gray(pathToMD),
				logutil.Yel(strconv.Itoa(id)),
				err,
			)
			continue
		}

		if existing.BodyMarkdown == content {
			logutil.Infof("%s: no change, skipping",
				logutil.Gray(pathToMD),
			)
			continue
		}

		if showDiff && existing.BodyMarkdown != content {
			fmt.Println(FormatDiff(existing.BodyMarkdown, content))
			continue
		}

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
				logutil.Gray(pathToMD),
				logutil.Yel(strconv.Itoa(id)),
				err,
			)
			continue
		}

		publishedStr := logutil.Red("unpublished")
		if published {
			publishedStr = logutil.Green("published")
		}
		fmt.Printf("%s: %s pushed %s to %s (devtoId: %d, devtoPublished: %t)\n",
			logutil.Green("success"),
			logutil.Gray(pathToMD),
			publishedStr,
			logutil.Yel(addEditSegment(art.URL.String(), published)),
			art.ID,
			published,
		)
	}
	return nil
}

// Returns all the user's unpublished articles and then the published
// articles.
func listAllMyArticles(client *devto.Client) ([]devto.ListedArticle, error) {
	// The max. number of items per page is 1000, see:
	// https://docs.forem.com/api/#tag/articles.
	articlesUnpublished, err := client.Articles.ListMyUnpublishedArticles(context.Background(), &devto.MyArticlesOptions{PerPage: 1000})
	if err != nil {
		return nil, fmt.Errorf("fetching unpublished articles: %s", err)
	}
	articlesPublished, err := client.Articles.ListMyPublishedArticles(context.Background(), &devto.MyArticlesOptions{PerPage: 1000})
	if err != nil {
		return nil, fmt.Errorf("fetching published articles: %s", err)
	}
	return append(articlesUnpublished, articlesPublished...), nil
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

	articles, err := listAllMyArticles(client)
	for _, article := range articles {

		publishedStr := logutil.Red("unpublished")
		if article.Published {
			publishedStr = logutil.Green("published")
		}
		fmt.Printf("%s: %s at %s (%s)\n",
			logutil.Gray(strconv.Itoa(int(article.ID))),
			publishedStr,
			logutil.Yel(addEditSegment(article.URL.String(), article.Published)),
			article.Title,
		)
	}
	if err != nil {
		return fmt.Errorf("listing user's articles on dev.to: %w", err)
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

func GetArticle(client *http.Client, articleID int) (devto.Article, error) {
	path := fmt.Sprintf("/api/articles/%d", articleID)
	req, err := http.NewRequest("GET", "https://dev.to"+path, nil)
	if err != nil {
		return devto.Article{}, fmt.Errorf("creating HTTP request for GET %s: %w", path, err)
	}

	httpResp, err := client.Do(req)
	if err != nil {
		return devto.Article{}, fmt.Errorf("while doing %s %s: %w", req.Method, path, err)
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
		return devto.Article{}, fmt.Errorf("while parsing JSON from the HTTP response for %s %s: %w", req.Method, path, err)
	}

	return art, nil
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
		return devto.Article{}, fmt.Errorf("creating HTTP request for %s %s: %w", req.Method, path, err)
	}

	httpResp, err := client.Do(req)
	if err != nil {
		return devto.Article{}, fmt.Errorf("while doing %s %s: %w", req.Method, path, err)
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
		return devto.Article{}, fmt.Errorf("while parsing JSON from the HTTP response for %s %s: %w", req.Method, path, err)
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

// client.Articles.ListAllMyArticles was not actually listing all articles
// and would only show the unpublished ones. Also, it would only show the
// first 20.

var hugoTag = regexp.MustCompile("{{< ([a-z]+) (.*) >}}")

// Hugo tag:
//
//   {{< youtube 30a0WrfaS2A >}}
//
// Devto liquid:
//
//   {% youtube 30a0WrfaS2A %}
//
// Ref: https://docs.dev.to/frontend/liquid-tags
func convertHugoToLiquid(in string) string {
	return hugoTag.ReplaceAllString(in, "{% $1 $2 %}")
}

// I want to be able to add the base post URL to each image. For example,
// imagine that the post is
//
//  https://maelvls.dev/you-should-write-comments/index.md
//
// then I need to replace the images, such as:
//
//  ![My image](cover-you-should-write-comments.png)
//
// with:
//
//  ![My image](https://maelvls.dev/you-should-write-comments/cover-you-should-write-comments.png)
//
// Note: (?s) means multiline, (?U) means non-greedy.
var mdImg = regexp.MustCompile(`(?sU)\!\[([^\]]*)\]\((\S*)\)`)

func addPostURLInImages(in string, basePostURL string) string {
	return mdImg.ReplaceAllString(in, "![$1]("+basePostURL+"$2)")
}
