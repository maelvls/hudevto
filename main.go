package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/VictorAvelar/devto-api-go/devto"
	"github.com/bep/logg"
	"github.com/gohugoio/hugo/config"
	"github.com/gohugoio/hugo/config/allconfig"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugolib"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/mgutz/ansi"
	"github.com/schollz/closestmatch"
	"github.com/sethgrid/gencurl"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/maelvls/hudevto/logutil"
	"github.com/maelvls/hudevto/pager"
)

var (
	rootDir    = flag.String("root", "", "Root directory of the Hugo project.")
	apiKeyFlag = flag.String("apikey", "", "The API key for Dev.to. You can also set DEVTO_APIKEY instead.")
	debug      = flag.Bool("debug", false, "Print debug information such as the HTTP requests that are being made in curl format.")
)

const usageErrMsg = `
usage:
  hudevto status [POST]
  hudevto (preview|diff) [POST]
  hudevto push [POST]
  hudevto devto list
  hudevto help

{{- if .WithOptionsSection}}

options:
{{ end -}}
`

const help = `
{{- section "NAME" }}

hudevto allows you to synchronize your Hugo posts with your DEV articles. The
synchronization is one way (Hugo to DEV). A Hugo post is only pushed when a
change is detected. When pushed to DEV, the Hugo article is transformed a bit,
e.g., relative image links are absolutified (see TRANSFORMATIONS).

{{ section "COMMANDS" }}

  hudevto status [POST]
      Shows the status of each post (or of a single post). The status shows
      whether it is mapped to a DEV article and if a push is required when the
      Hugo post has changes that are not on DEV yet.

  hudevto preview [POST]
      Displays a Markdown preview of the Hugo post that has been converted into
      the DEV article Markdown format. You can use this command to check that
      the tranformations were correctly applied.

  hudevto diff [POST]
      Displays a diff between the Hugo post and the DEV article. It is useful
      when you want to see what changes will be pushed.

  hudevto push [POST]
      Pushes the given Hugo Markdown post to DEV. If no post is given, then
      all posts are pushed.

  hudevto devto list
      Lists all the articles you have on your DEV account.

{{ section "IMPORTANT" }}

hudevto has been mainly built for pushing https://maelvls.dev, and the following
assumptions are made:

1. Each blog post is in its own folder and the article itself is in index.md,
   e.g. ./content/post-1/index.md.
2. The images are hosted along with the index.md file.
3. The base_url is set in config.yml.
4. Each article has the "url" field set in its front-matter.

{{ section "HOW TO USE IT" }}

In order to operate, hudevto requires you to have your DEV account configured
with "Publish to DEV Community from your blog's RSS". You can configure that at
{{ url "https://dev.to/settings/extensions" }}. DEV will create a draft article for
every Hugo post that you have published on your blog. For example, Let us
imagine that your Hugo blog layout is:

    .
    └── content
       ├── brick-chest.md
       ├── cloth-impossible.md
       └── powder-farmer.md

After configuring the RSS feed of your blog at {{ url "https://maelvls.dev/index.xml" }},
DEV should create one draft article per post. You can check that these articles
have been created on DEV with:

    {{ cmd "hudevto devto list" }}
    {{ out "386001: unpublished at https://dev.to/maelvls/brick-chest/edit" }}
    {{ out "386002: unpublished at https://dev.to/maelvls/cloth-impossible/edit" }}
    {{ out "386003: unpublished at https://dev.to/maelvls/powder-farmer/edit" }}

The next step is to map each article that you want to sync to DEV. Let us see
the state of the mapping:

    {{ cmd "hudevto status" }}
    {{ out "error: ./content/brick-chest.md: missing devtoId field in front matter, might be 386001: https://dev.to/maelvls/brick-chest/edit" }}
    {{ out "error: ./content/cloth-impossible.md: missing devtoId field in front matter, might be 386002: https://dev.to/maelvls/cloth-impossible/edit" }}
    {{ out "error: ./content/powder-farmer.md: missing devtoId field in front matter, might be 386003: https://dev.to/maelvls/powder-farmer/edit" }}

At this point, you need to open each of your Hugo post and add some fields to
their front matters. For example, in ./content/brick-chest.md, we add this:

    devtoId: 386001       {{ grey "# This is the DEV ID as seen in hudevto devto list" }}
    devtoPublished: true  {{ grey "# When false, the DEV article will stay a draft" }}
    devtoSkip: false      {{ grey "# When true, hudevto will ignore this post." }}

The status should have changed:

    {{ cmd "hudevto status" }}
    {{ out "info: ./content/brick-chest.md will be pushed published to https://dev.to/maelvls/brick-chest/edit" }}
    {{ out "info: ./content/cloth-impossible.md will be pushed published to https://dev.to/maelvls/cloth-impossible/edit" }}
    {{ out "info: ./content/powder-farmer.md will be pushed published to https://dev.to/maelvls/powder-farmer/edit" }}

Finally, you can push to DEV:

    {{ cmd "hudevto push" }}
    {{ out "success: ./content/brick-chest.md pushed to https://dev.to/maelvls/brick-chest-2588" }}
    {{ out "success: ./content/cloth-impossible.md pushed to https://dev.to/maelvls/cloth-impossible-95dc" }}
    {{ out "success: ./content/powder-farmer.md pushed to https://dev.to/maelvls/powder-farmer-6a18" }}

{{ section "TRANSFORMATIONS" }}
The Markdown for Hugo posts and {{ url "dev.to" }} articles have slight differences.
Before pushing to {{ url "dev.to" }}, hudevto does some transformations to the Markdown
file. To see the transformations before pushing the Hugo post to dev.to, use one of:

    {{ cmd "hudevto diff ./debug-k8s/index.md" }}

The transformations are:

1. ABSOLUTE MARKDOWN IMAGES: the relative image links are absolutified since
   dev.to needs the image path to be absolute (the base URL itself is not
   required).

   The following Hugo Markdown snippet:

     ![wireshark](wireshark.png)

    becomes:

      ![wireshark](/debug-k8s/wireshark.png)
                   <--(1)--->

    where (1) is the article's Hugo permalink to the ./debug-k8s/index.md post.
    Note that the ![]() tag must span a single line. Otherwise, it won't be
    transformed.

2. ABSOLUTE HTML IMG TAGS: unlike with Markdown images, the <img> HTML tags
   need to be absolute and needs to contain the base URL. For example, the
   following HTML:

        <img src="wireshark.png">

    gets transformed to:

        <img src="https://maelvls/debug-k8s/wireshark.png">

    The <img> tag must be on a single line, and the "src" value must end with
	one of the following extensions: png, PNG, jpeg, JPG, jpg, gif, GIF, svg,
	SVG.

3. SHORTCODES: Hugo shortcodes for embedding (like for embedding a Youtube video)
   are turned into Liquid tags that dev.to knows about.
4. ANCHOR IDS: Hugo and Devto have different anchor ID syntaxes.

{{ section "OPTIONS" }}
`

func main() {
	printHelpWithPager := func() {
		t, err := template.New("test").Funcs(map[string]interface{}{
			"section": ansi.ColorFunc("black+hb"),
			"url":     ansi.ColorFunc("white+u"),
			"grey":    ansi.ColorFunc("white+d"),
			"yel":     ansi.ColorFunc("yellow"),
			"cmd": func(cmd string) string {
				return ansi.ColorFunc("white+d")("% ") + ansi.ColorFunc("yellow+b")(cmd)
			},
			"out": ansi.ColorFunc("white+d"),
		}).Parse(help)
		if err != nil {
			log.Fatal(err)
		}

		pager.Main(context.Background(), func(_ context.Context, out io.WriteCloser) int {
			err = t.Execute(out, nil)
			if err != nil {
				log.Fatal(err)
			}
			flag.CommandLine.SetOutput(out)
			flag.PrintDefaults()

			return 0
		})
	}

	printUsage := func(withOptions bool) func() {
		return func() {
			t, err := template.New("test").Parse(usageErrMsg)
			if err != nil {
				log.Fatal(err)
			}
			err = t.Execute(flag.CommandLine.Output(), struct{ WithOptionsSection bool }{
				WithOptionsSection: withOptions,
			})
			if err != nil {
				log.Fatal(err)
			}

			if withOptions {
				flag.PrintDefaults()
			}
		}
	}

	flag.Usage = printUsage(true)

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
		err := PushArticlesFromHugoToDevto(filepath.Clean(*rootDir), flag.Arg(1), false, false, false, apiKey)
		if err != nil {
			logutil.Errorf(err.Error())
			os.Exit(1)
		}
	case "preview":
		err := PushArticlesFromHugoToDevto(filepath.Clean(*rootDir), flag.Arg(1), true, false, true, apiKey)
		if err != nil {
			logutil.Errorf(err.Error())
			os.Exit(1)
		}
	case "diff":
		err := PushArticlesFromHugoToDevto(filepath.Clean(*rootDir), flag.Arg(1), false, true, true, apiKey)
		if err != nil {
			logutil.Errorf(err.Error())
			os.Exit(1)
		}
	case "status":
		err := PushArticlesFromHugoToDevto(filepath.Clean(*rootDir), flag.Arg(1), false, false, true, apiKey)
		if err != nil {
			logutil.Errorf(err.Error())
			os.Exit(1)
		}
	case "devto":
		if flag.Arg(1) == "list" {
			err := PrintDevtoArticles(apiKey)
			if err != nil {
				logutil.Errorf(err.Error())
				os.Exit(1)
			}
		} else {
			logutil.Errorf("usage: hudevto devto list")
			os.Exit(1)
		}
	case "list":
		logutil.Errorf("did you mean 'list'? Try the command:\n    hudevto devto list")
		os.Exit(1)
	case "help":
		printHelpWithPager()
		os.Exit(0)
	case "":
		logutil.Errorf("no command has been given.")
		printUsage(false)()
		os.Exit(124)
	default:
		logutil.Errorf("unknown command '%s'.", flag.Arg(0))
		printUsage(false)()
		os.Exit(124)
	}
}

// Updates all articles if pathToArticle is left empty.
func PushArticlesFromHugoToDevto(rootDir, pathToArticle string, showMarkdown, showDiff, dryRun bool, apiKey string) error {
	configs, err := loadHugoConfig(rootDir)
	if err != nil {
		return err
	}

	if rootDir == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current working directory: %w", err)
		}
		rootDir = cwd
	}

	log.Printf("with workingDir %s and publishDir %s", rootDir, filepath.Join(rootDir, "public"))
	configProvider := config.New()
	configProvider.Set("workingDir", rootDir)
	configProvider.Set("publishDir", filepath.Join(rootDir, "public"))

	sites, err := hugolib.NewHugoSites(deps.DepsCfg{
		LogLevel: logg.LevelWarn,
		Fs:       hugofs.NewDefault(configProvider),
		Configs:  configs,
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
			logutil.Infof("%s: field devtoSkip is true, skipping this post.",
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

func loadHugoConfig(root string) (*allconfig.Configs, error) {
	if !filepath.IsAbs(root) {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(cwd, root)
	}

	configs, err := allconfig.LoadConfig(allconfig.ConfigSourceDescriptor{
		Fs:       hugofs.Os,
		Filename: filepath.Join(root, "config.yaml"),
	})

	if err != nil {
		return nil, err
	}

	// config.Set("workingDir", root)

	return configs, nil
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
