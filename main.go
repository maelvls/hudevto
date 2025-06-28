package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/VictorAvelar/devto-api-go/devto"
	"github.com/charmbracelet/fang"
	"github.com/gohugoio/hugo/config"
	"github.com/gohugoio/hugo/config/allconfig"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugolib"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/schollz/closestmatch"
	"github.com/sethgrid/gencurl"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/maelvls/hudevto/logutil"
	"github.com/maelvls/undent"
)

func main() {
	rootCmd := mainCmd()
	err := fang.Execute(context.Background(), rootCmd, fang.WithErrorHandler(func(w io.Writer, styles fang.Styles, err error) {
		fang.DefaultErrorHandler(w, styles, err)
	}))
	if err != nil {
		os.Exit(1)
	}
}

func mainCmd() *cobra.Command {
	var rootDir, apiKeyFlag string
	cmd := &cobra.Command{
		Use:   "hudevto",
		Short: "Synchronize your Hugo posts with your DEV articles.",
		Long: undent.Undent(`
			hudevto allows you to synchronize your Hugo posts with your DEV articles. The
			synchronization is one way (Hugo to DEV). A Hugo post is only pushed when a
			change is detected. When pushed to DEV, the Hugo article is transformed a bit,
			e.g., relative image links are absolutified.

			For more information about the transformation, see:
			    https://github.com/maelvls/hudevto/blob/main/README.md
		`),
	}
	cmd.PersistentFlags().StringVar(&rootDir, "root", "", "Root directory of the Hugo project.")
	cmd.PersistentFlags().StringVar(&apiKeyFlag, "apikey", "", "The API key for Dev.to. You can also set DEVTO_APIKEY instead.")
	cmd.PersistentFlags().BoolVar(&logutil.EnableDebug, "debug", false, "Print debug information such as the HTTP requests that are being made in curl format.")

	cmd.AddCommand(statusCmd(), pushCmd(), previewCmd(), diffCmd(), devtoCmd())
	return cmd
}

func statusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [POST]",
		Short: "Show the status of each post (or a single post)",
		Long: undent.Undent(`
			Shows the status of each post (or of a single post). The status shows
      		whether it is mapped to a DEV article and if a push is required when the
      		Hugo post has changes that are not on DEV yet.
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiKey, err := getApiKey(cmd)
			if err != nil {
				return err
			}
			var pathToArticle string
			if len(args) > 0 {
				pathToArticle = args[0]
			}
			rootDir, err := getRootDir(cmd)
			if err != nil {
				return fmt.Errorf("--root: %w", err)
			}
			rootDir = filepath.Clean(rootDir)
			return PushArticlesFromHugoToDevto(rootDir, pathToArticle, false, false, true, apiKey)
		},
	}
	return cmd
}

func pushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push [POST]",
		Short: "Push the given Hugo Markdown post to DEV.",
		Long: undent.Undent(`
			Pushes the given Hugo Markdown post to DEV. If no post is given, then
			all posts are pushed. The post must be a Markdown file, i.e., *.md.
		`),
		Example: undent.Undent(`
			hudevto push ./content/post-1/index.md
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiKey, err := getApiKey(cmd)
			if err != nil {
				return fmt.Errorf("while getting API key: %w", err)
			}
			var pathToArticle string
			if len(args) > 0 {
				pathToArticle = args[0]
			}
			rootDir, err := getRootDir(cmd)
			if err != nil {
				return err
			}
			return PushArticlesFromHugoToDevto(rootDir, pathToArticle, false, false, false, apiKey)
		},
	}
	return cmd
}

func previewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview [POST]",
		Short: "Display a Markdown preview of the Hugo post as it would appear on DEV.",
		Long: undent.Undent(`
			Displays a Markdown preview of the Hugo post that has been converted into
			the DEV article Markdown format. You can use this command to check that
			the tranformations were correctly applied.
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiKey, err := getApiKey(cmd)
			if err != nil {
				return fmt.Errorf("while getting API key: %w", err)
			}
			var pathToArticle string
			if len(args) > 0 {
				pathToArticle = args[0]
			}
			if len(args) == 0 {
				return fmt.Errorf("no post given, please provide a post to preview")
			}
			rootDir, err := getRootDir(cmd)
			if err != nil {
				return err
			}
			return PushArticlesFromHugoToDevto(rootDir, pathToArticle, true, false, true, apiKey)
		},
	}
	return cmd
}

func diffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff [POST]",
		Short: "Display a diff between the Hugo post and the DEV article.",
		Long: undent.Undent(`
		 	Displays a diff between the Hugo post and the DEV article. It is useful
      		when you want to see what changes will be pushed.
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiKey, err := getApiKey(cmd)
			if err != nil {
				return fmt.Errorf("while getting API key: %w", err)
			}

			var pathToArticle string
			if len(args) > 0 {
				pathToArticle = args[0]
			}

			rootDir, err := getRootDir(cmd)
			if err != nil {
				return err
			}
			return PushArticlesFromHugoToDevto(rootDir, pathToArticle, false, true, true, apiKey)
		},
	}
	return cmd
}

func devtoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devto list",
		Short: "List all the articles you have on your DEV account.",
		Long: undent.Undent(`
			Lists all the articles you have on your DEV account.
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "list" {
				return fmt.Errorf("usage: hudevto devto list")
			}
			apiKey, err := getApiKey(cmd)
			if err != nil {
				return err
			}
			return PrintDevtoArticles(apiKey)
		},
	}
	return cmd
}

func getRootDir(cmd *cobra.Command) (string, error) {
	rootDir, err := cmd.Flags().GetString("root")
	if err != nil {
		return "", fmt.Errorf("--root: %w", err)
	}
	rootDir = filepath.Clean(rootDir)
	return rootDir, nil
}

func getApiKey(cmd *cobra.Command) (string, error) {
	apiKey := os.Getenv("DEVTO_APIKEY")

	apiKeyFlag, err := cmd.Flags().GetString("apikey")
	if err != nil {
		return "", fmt.Errorf("while getting --apikey flag: %w", err)
	}
	if apiKeyFlag != "" {
		apiKey = apiKeyFlag
	}
	if apiKey == "" {
		return "", fmt.Errorf("no API key given, either give it with --apikey or with DEVTO_APIKEY")
	}
	return apiKey, nil
}

// Updates all articles if pathToArticle is left empty. The pathToArticle must
// be a markdown file, i.e., *.md. The rootDir cannot be left empty; if you want
// to use the current working directory, use ".".
func PushArticlesFromHugoToDevto(rootDirOrEmpty, relPathToArticle string, showMarkdown, showDiff, dryRun bool, apiKey string) error {
	if rootDirOrEmpty == "" {
		panic("programmer mistake: PushArticlesFromHugoToDevto: rootDirOrEmpty cannot be empty")
	}
	rootDir := filepath.Clean(rootDirOrEmpty)
	if rootDir == "." {
		var err error
		rootDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("while getting current working directory: %w", err)
		}
	}
	logutil.Debugf("using rootDir='%s', rootDirOrEmpty='%s'", logutil.Gray(rootDir), logutil.Gray(rootDirOrEmpty))

	fs := hugofs.NewBasePathFs(hugofs.Os, rootDir)
	configs, err := allconfig.LoadConfig(allconfig.ConfigSourceDescriptor{
		Fs:       fs,
		Filename: "config.yaml",
	})
	if err != nil {
		return fmt.Errorf("while loading config: %w", err)
	}

	configProvider := config.New()
	configProvider.Set("workingDir", rootDir)
	configProvider.Set("publishDir", "unused")
	configProvider.Set("themesDir", filepath.Join(rootDir, "themes"))

	sites, err := hugolib.NewHugoSites(deps.DepsCfg{
		Fs:      hugofs.NewFromSourceAndDestination(fs, fs, configProvider),
		Configs: configs,
	})
	if err != nil {
		return fmt.Errorf("while creating sites: %w", err)
	}

	err = sites.Build(hugolib.BuildCfg{SkipRender: true})
	if err != nil {
		return fmt.Errorf("while processing content: %w", err)
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
	if relPathToArticle != "" {
		p := sites.GetContentPage("/" + relPathToArticle)
		if p == nil {
			return fmt.Errorf("not found: %s", path.Join(rootDirOrEmpty, logutil.Gray(relPathToArticle)))
		}

		pages = []page.Page{p}
	}

	for _, page := range pages {
		if page.Kind() != "page" {
			continue
		}

		pathToMD := rootDirOrEmpty + "/content" + page.Path()

		// The pathToMD might either be an .md file or a folder, so we need to
		// find the index.md file if it's a folder.
		if filepath.Ext(pathToMD) == "" {
			pathToMD = filepath.Join(pathToMD, "index.md")
		}

		draft := true
		draftRaw, err := page.Param("draft")
		if err == nil {
			draft = draftRaw.(bool)
		}
		if draft {
			continue
		}

		devtoSkip := false
		devtoSkipRaw, err := page.Param("devtoSkip")
		if devtoSkipRaw != nil && err == nil {
			var ok bool
			devtoSkip, ok = devtoSkipRaw.(bool)
			if !ok {
				logutil.Errorf("%s: field devtoSkip is expected to be a boolean, got '%T'",
					logutil.Gray(pathToMD),
					devtoSkipRaw,
				)
				continue
			}
		}
		if devtoSkip {
			logutil.Debugf("%s: field devtoSkip is true, skipping this post.",
				logutil.Gray(pathToMD),
			)
			continue
		}

		devtoPublished := false
		devtoPublishedRaw, err := page.Param("devtoPublished")
		if devtoPublishedRaw == nil {
			logutil.Errorf("%s: missing devtoPublished field",
				logutil.Gray(pathToMD),
			)
			continue
		}
		if err == nil {
			var ok bool
			devtoPublished, ok = devtoPublishedRaw.(bool)
			if !ok {
				logutil.Errorf("%s: field devtoPublished is expected to be a boolean, got '%T'",
					logutil.Gray(pathToMD),
					devtoPublishedRaw,
				)
				continue
			}
		}

		devtoIdRaw, err := page.Param("devtoId")
		if err != nil || devtoIdRaw == nil {
			if art, ok := articlesTitleMap[page.Title()]; ok {
				logutil.Errorf("%s missing devtoId field in front matter, might be %s: %s",
					logutil.Gray(pathToMD),
					logutil.Green(strconv.Itoa(int(art.ID))),
					logutil.Yel(addEditSegment(art.URL.String(), devtoPublished)),
				)
			} else {
				logutil.Errorf("%s missing devtoId field in front matter and title cannot be found on your devto account",
					logutil.Gray(pathToMD),
				)
			}
			continue
		}
		devtoId, ok := devtoIdRaw.(int)
		if !ok {
			logutil.Errorf("%s: field devtoId is expected to be an integer, got '%T'",
				logutil.Gray(pathToMD),
				devtoIdRaw,
			)
			continue
		}

		article, found := articlesIdMap[devtoId]
		if !found {
			if art, ok := articlesTitleMap[page.Title()]; ok {
				logutil.Errorf("%s: devtoId %s is unknown but title matches devtoId %s: %s",
					logutil.Gray(pathToMD),
					logutil.Red(strconv.Itoa(devtoId)),
					logutil.Green(strconv.Itoa(int(art.ID))),
					logutil.Yel(addEditSegment(art.URL.String(), devtoPublished)),
				)
			} else {
				logutil.Errorf("%s: devtoId %s is unknown and title cannot be found in your devto account",
					logutil.Gray(pathToMD),
					logutil.Red(strconv.Itoa(devtoId)),
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
				logutil.Yel(addEditSegment(article.URL.String(), devtoPublished)),
			))
			continue
		}

		img := ""
		var imgs []string
		imgsRaw, err := page.Param("images")
		_, isEmptyArray := imgsRaw.([]any)
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
			devtoPublished,
			strings.Join(page.Keywords(), ", "),
			page.Date().UTC().Format("20060102T15:04Z"),
			strings.Join([]string{}, ", "),
			page.Permalink(),
			img,
		)

		body := page.RawContent()
		body = convertHugoToLiquid(body)
		body = addPostURLInImages(body, page.Permalink())
		body = addPostURLInHTMLImages(body, page.Permalink())

		if len(sites.Sites) > 0 {
			body = convertAnchorIDs(pathToMD, body, sites.Sites[0].SanitizeAnchorName)
		} else {
			logutil.Errorf("%s: no site found, cannot convert anchor IDs",
				logutil.Gray(pathToMD),
			)
		}

		content += body

		if showMarkdown {
			fmt.Print(content)
			return nil
		}

		existing, ok := articlesIdMap[devtoId]
		if !ok {
			logutil.Errorf("%s: the article id %s was not fetched from either /articles/me/published or /articles/me/unpublished",
				logutil.Gray(pathToMD),
				logutil.Yel(strconv.Itoa(devtoId)),
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
			logutil.Infof("%s: found differences",
				logutil.Gray(pathToMD),
			)
			fmt.Println(FormatDiff(existing.BodyMarkdown, content))
			continue
		}

		if dryRun {
			publishedStr := logutil.Red("unpublished")
			if devtoPublished {
				publishedStr = logutil.Green("published")
			}
			fmt.Printf("%s: %s will be pushed %s to %s (devtoId: %d, devtoPublished: %t)\n",
				logutil.Yel("info"),
				logutil.Gray(pathToMD),
				publishedStr,
				logutil.Yel(addEditSegment(article.URL.String(), devtoPublished)),
				article.ID,
				devtoPublished,
			)
			continue
		}

	Update:
		art, err := UpdateArticle(httpClient, devtoId, Article{BodyMarkdown: content})
		switch {
		case isTooManyRequests(err):
			// As per https://docs.forem.com/api/#operation/updateArticle,
			// there is a limit of 30 requests per 30 seconds.
			time.Sleep(1 * time.Second)
			goto Update
		case err != nil:
			logutil.Errorf("%s: updating devto id %s: %s",
				logutil.Gray(pathToMD),
				logutil.Yel(strconv.Itoa(devtoId)),
				err,
			)
			continue
		}

		// After a successful update, add the devtoUrl to the front matter.
		if err := addDevtoUrlToFrontMatter(pathToMD, art.URL.String()); err != nil {
			logutil.Errorf("%s: failed to update front matter with devtoUrl: %s",
				logutil.Gray(pathToMD),
				err,
			)
		}

		publishedStr := logutil.Red("unpublished")
		if devtoPublished {
			publishedStr = logutil.Green("published")
		}
		fmt.Printf("%s: %s pushed %s to %s (devtoId: %d, devtoPublished: %t)\n",
			logutil.Green("success"),
			logutil.Gray(pathToMD),
			publishedStr,
			logutil.Yel(addEditSegment(art.URL.String(), devtoPublished)),
			art.ID,
			devtoPublished,
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

func selectArticle(articles []devto.Article, articleID uint32) (devto.Article, error) {
	for _, article := range articles {
		if article.ID == articleID {
			return article, nil
		}
	}
	return devto.Article{}, fmt.Errorf("article id %d not found", articleID)
}

// Get the published article using its ID. Note that it does not work for
// unpublished articles.
// https://developers.forem.com/api#operation/getArticleById
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

// https://developers.forem.com/api#operation/updateArticle
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
//	{{< youtube 30a0WrfaS2A >}}
//
// Devto liquid:
//
//	{% youtube 30a0WrfaS2A %}
//
// Ref: https://docs.dev.to/frontend/liquid-tags
func convertHugoToLiquid(in string) string {
	return hugoTag.ReplaceAllString(in, "{% $1 $2 %}")
}

// I want to be able to add the base post URL to each image. For example,
// imagine that the post is
//
//	https://maelvls.dev/you-should-write-comments/index.md
//
// then I need to replace the images, such as:
//
//	![My image](cover-you-should-write-comments.png)
//
// with:
//
//	![My image](/you-should-write-comments/cover-you-should-write-comments.png)
//	            <------ basePostURL ------>
//	           (basePostURL already includes the leading / and trailing /)
//
// Note: (?s) means multiline, (?U) means non-greedy.
var mdImg = regexp.MustCompile(`(?sU)\!\[([^\]]*)\]\((\S*)\)`)

func addPostURLInImages(in string, basePostURL string) string {
	return mdImg.ReplaceAllString(in, "![$1]("+basePostURL+"$2)")
}

// Since you can also embed `<img>` tags in markdown, these are also converted. For example,
//
//	<img alt="Super example" src="dnat-google-vpc-how-comes-back.svg" width="80%"/>
//
// is converted to:
//
//	<img alt="Super example" src="/you-should-write-comments/dnat-google-vpc-how-comes-back.svg" width="80%"/>
//
// Only the following image extensions are converted: png, PNG, jpeg, JPG, jpg,
// gif, GIF, svg, SVG.
//
// (?s) means multiline, (?U) means non-greedy.
var htmlImg = regexp.MustCompile(`(?sU)src="([^"]*(png|PNG|jpeg|JPG|jpg|gif|GIF|svg|SVG))"`)

func addPostURLInHTMLImages(in string, basePostURL string) string {
	return htmlImg.ReplaceAllString(in, `src="`+basePostURL+`$1"`)
}

// The convertAnchorIDs function reads Markdown, finds any anchor-based link of
// the form [foo](#foo) and converts the GitHub-style anchor IDs to Devto anchor
// IDs. This is because GitHub-style anchor IDs, which is what Hugo produces,
// are different from the ones produced by Devto. For example, take the
// following Markdown:
//
//	[`go get -u` vs. `go.mod` (= *_Problem_*)](#go-get--u-vs-gomod--_problem_)
//
// becomes
//
//	[`go get -u` vs. `go.mod` (= *_Problem_*)](#-raw-go-get-u-endraw-vs-raw-gomod-endraw-problem)

var linkWithOnlyAnchor = regexp.MustCompile(`\[([^\]]*)\]\(#([^\)]*)\)`)
var code = regexp.MustCompile("`([^`]*)`")
var whitespace = regexp.MustCompile(`\s+`)
var nonAlphaNumExceptDashAndSpace = regexp.MustCompile(`[^-a-zA-Z0-9]`)
var multipleDashes = regexp.MustCompile(`-{2,}`)

// only ATX headings are supported (headings of the form "# Title")
func convertAnchorIDs(pathToMD, in string, sanitizeAnchorName func(s string) string) string {
	inBytes := []byte(in)
	parsed := goldmark.DefaultParser().Parse(text.NewReader(inBytes))

	anchorToHeading := make(map[string]string)
	ast.Walk(parsed, func(node ast.Node, _ bool) (ast.WalkStatus, error) {
		headingNode, ok := node.(*ast.Heading)
		if ok {
			if headingNode.Lines().Len() != 1 {
				logutil.Errorf("unexpected heading: %s", headingNode.Text(inBytes))
				return ast.WalkContinue, nil
			}
			seg := headingNode.Lines().At(0)
			heading := string(seg.Value(inBytes))

			anchorToHeading[sanitizeAnchorName(heading)] = heading
		}
		return ast.WalkContinue, nil
	})

	return linkWithOnlyAnchor.ReplaceAllStringFunc(in, func(s string) string {
		matches := linkWithOnlyAnchor.FindStringSubmatch(s)
		if len(matches) != 3 {
			return s
		}

		// We ignore the "text" part, since we will use the headings that we
		// found when parsing the Markdown document.
		//
		//  [`go get -u` vs. `go.mod` (= *_problem_*)](#go-get--u-vs-gomod--_problem_)
		//   <-------------------------------------->  <-------------------------->
		//                text is ignored                         anchor
		anchor := matches[2]
		heading, found := anchorToHeading[anchor]
		if !found {
			possibleAnchors := make([]string, 0, len(anchorToHeading))
			for anchor := range anchorToHeading {
				possibleAnchors = append(possibleAnchors, anchor)
			}
			matcher := closestmatch.New(possibleAnchors, []int{2})

			logutil.Errorf("%s: anchor %q in link %s doesn't exist in the document. Did you mean %s?",
				logutil.Gray(pathToMD),
				anchor, s,
				matcher.Closest(anchor),
			)

			return s
		}

		// Rule 1: `foo` is converted to `raw-foo-endraw-`.
		heading = code.ReplaceAllString(heading, "-raw-$1-endraw-")

		// Rule 2: whitespaces (spaces and tabs) are replaced with a dash (-).
		heading = whitespace.ReplaceAllString(heading, "-")

		// Rule 3: all other non-alphanumeric characters are removed.
		heading = nonAlphaNumExceptDashAndSpace.ReplaceAllString(heading, "")

		// Rule 4: two dashes or more are combined into a single dash.
		heading = multipleDashes.ReplaceAllString(heading, "-")

		// Rule 5: the anchor ID is lowercase.
		heading = strings.ToLower(heading)

		// Replace the anchor ID.
		return strings.Replace(s, "#"+matches[2], "#"+heading, 1)
	})
}

// addDevtoUrlToFrontMatter adds or updates the devtoUrl field in the front matter of a Markdown file
func addDevtoUrlToFrontMatter(filePath string, url string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	contentStr := string(content)

	// Check if there's YAML front matter (between --- markers).
	frontMatterRegex := regexp.MustCompile(`(?s)^---\n(.*?)\n---`)
	match := frontMatterRegex.FindStringSubmatch(contentStr)
	if len(match) < 2 {
		return fmt.Errorf("front matter not found in %s", filePath)
	}

	frontMatter := match[1]

	// Check if devtoUrl already exists
	devtoUrlRegex := regexp.MustCompile(`(?m)^devtoUrl:\s*.*$`)
	if devtoUrlRegex.MatchString(frontMatter) {
		// Replace existing devtoUrl
		updatedFrontMatter := devtoUrlRegex.ReplaceAllString(frontMatter, fmt.Sprintf("devtoUrl: %s", url))
		updatedContent := frontMatterRegex.ReplaceAllString(contentStr, fmt.Sprintf("---\n%s\n---", updatedFrontMatter))
		return os.WriteFile(filePath, []byte(updatedContent), 0644)
	}

	// Add new devtoUrl field
	updatedFrontMatter := fmt.Sprintf("%s\ndevtoUrl: %s", frontMatter, url)
	updatedContent := frontMatterRegex.ReplaceAllString(contentStr, fmt.Sprintf("---\n%s\n---", updatedFrontMatter))
	return os.WriteFile(filePath, []byte(updatedContent), 0644)
}

func errWorkDirTooShort(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), "workingDir is too short") {
		return true
	}

	return false
}
