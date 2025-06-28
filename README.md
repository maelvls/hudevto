# hudevto, the CLI for pushing and synchronizing your Hugo blog posts to Dev.to

![Screenshot of the hudevto push command](https://user-images.githubusercontent.com/2195781/108324642-737e7f80-71c8-11eb-9e4f-8f23fd14d644.png)

**Content:**

- [Install](#install)
- [Use it](#use-it)
  - [Step 1: Configure Devto with your blog's RSS feed](#step-1-configure-devto-with-your-blogs-rss-feed)
  - [Step 2: Add `devtoId` and `devtoPublished` to page's front matter](#step-2-add-devtoid-and-devtopublished-to-pages-front-matter)
  - [Step 3: push](#step-3-push)
  - [Transformations](#transformations)
  - [Features](#features)
    - [Preview and diff changes](#preview-and-diff-changes)
    - [List your dev.to articles](#list-your-devto-articles)
- [Notes](#notes)
  - [Hugo's hard breaks versus dev.to hard breaks](#hugos-hard-breaks-versus-devto-hard-breaks)
  - [Known errors](#known-errors)

## Install

```sh
# Requirement: Go is installed and $(go env GOPATH)/bin is in your PATH.
go install github.com/maelvls/hudevto@latest
```

## Use it

First, copy your dev.to token from your dev.to settings and set it as an
environment variable:

```sh
export DEVTO_APIKEY=$(lpass show dev.to -p)
```

### Step 1: Configure Devto with your blog's RSS feed

In order to operate, hudevto requires you to have your Devto account configured
with **Publish to DEV Community from your blog's RSS**. You can configure that
at <https://dev.to/settings/extensions>. Devto will create a draft article for
every Hugo post that you have published on your blog. For example, my RSS feed
is at <https://maelvls.dev/index.xml>, so I configured Devto to automatically
create a draft article for each of my posts.

For example, let's imagine that your Hugo blog has two articles:

```text
.
└── content
   ├── brick-chest.md
   └── powder-farmer
       └── index.md
```

Now, check that Devto has created the draft articles for each of your posts:

```console
$ hudevto devto list
365846: unpublished at https://dev.to/maelvls/brick-chest-temp-slug-3687644/edit (Brick Chest)
365847: unpublished at https://dev.to/maelvls/powder-farmer-temp-slug-8753044/edit (Powder Farmer)
```

### Step 2: Add `devtoId` and `devtoPublished` to page's front matter

Now, run `status` to see what you need to do next:

```console
error: content/brick-chest.md missing devtoId field in front matter, might be 365846: https://dev.to/maelvls/brick-chest-temp-slug-3687644/edit
error: content/powder-farmer/index.md missing devtoId field in front matter, might be 365847: https://dev.to/maelvls/powder-farmer-temp-slug-8753044/edit
```

Add the `devtoId` field to the front matter of each of your posts. For example,
open `./content/brick-chest.md` and add `devtoId: 365846` to the front matter:

```diff
 title: "Brick Chest: A game about building a chest with bricks"
+devtoId: 365846
 ---
```

Run `hudevto status` again to see the status of your posts:

```console
$ hudevto status
error: content/brick-chest.md: missing devtoPublished field
error: content/powder-farmer/index.md: missing devtoPublished field
```

Now, you need to add the `devtoPublished` field to the front matter of each
post. For example, in `./content/brick-chest.md`, add `devtoPublished: true`:

```diff
 title: "Brick Chest: A game about building a chest with bricks"
 devtoId: 365846
+devtoPublished: true
```

Now, run `hudevto status` again:

```console
info: content/brick-chest.md will be pushed published to https://dev.to/maelvls/brick-chest/edit (devtoId: 365846, devtoPublished: true)
info: content/powder-farmer/index.md will be pushed published to https://dev.to/maelvls/powder-farmer/edit (devtoId: 365847, devtoPublished: true)
```

### Step 3: push

Finally, you can push:

```sh
hudevto push
```

> [!NOTE]
>
> You can also use `devtoSkip: true` if you want `hudevto` to skip a given post.
>
> Here is the documentation for the front matter fields that `hudevto` knows
> about:
>
> ```yaml
> devtoId: 386001       # This is the Devto ID as seen in hudevto devto list.
> devtoSkip: false      # When true, hudevto will ignore this post.
> devtoPublished: true  # When false, the DEV article will stay a draft.
> devtoDraft: true      # When true, the post will be pushed as a draft.
> devtoUrl: https://... # Set by hudevto.
> ```

### Transformations

The Markdown for Hugo posts and dev.to articles have slight differences. Before
pushing to dev.to, `hudevto` does some transformations to the Markdown file. To
see the transformations before pushing the Hugo post to dev.to, you can use:

```sh
hudevto preview ./content/2020/avoid-gke-lb-using-hostport/index.md
hudevto diff ./content/2020/avoid-gke-lb-using-hostport/index.md
```

Here are the transformations that are made:

- **Front matter:** Updates the Markdown front matter. The front matter is used
  to configure the Devto post title and canonical URL.
- **Shortcodes:** the Hugo shortcodes are transformed into shortcodes that
  Devto knows about (called "Liquid tags"). For example, the following
  Hugo shortcode:

  ```md
  {{< youtube 30a0WrfaS2A >}}
  ```

  is changed to the Liquid tag:

  ```md
  {% youtube 30a0WrfaS2A %}
  ```

- **Absolute Markdown images:** Markdown image links are transformed to
  absolute URLs using the base URL of the post. That way, images keep working
  in Dev.to. ONLY WORKS if your images are stored along side your blog post,
  such as:

  ```console
  % ls --tree ./content/2020/avoid-gke-lb-using-hostport
  ./content/2020/avoid-gke-lb-using-hostport
  ├── cost-load-balancer-gke.png
  ├── cover-external-dns.png
  ├── how-service-controller-works-on-gke.png
  ├── index.md                                             # The actual blog post.
  └── packet-routing-with-akrobateo.png
  ```

- **Absolute HTML `<img>` tags:** The relative image links are "absolutified".
  This is needed so that Devto can access the images. For example, the following
  post:

  <https://maelvls.dev/you-should-write-comments/index.md>

  then I need to replace the relative image paths such as

  ```markdown
  ![My image](cover-you-should-write-comments.png)
  ```

  with:

  ```text
  ![My image](/you-should-write-comments/cover-you-should-write-comments.png)
              <----------------------->
                        url
               (from front matter)
  ```

  The prefix that gets added comes from the front matter of the Hugo post. Here
  is an example of front matter:

  ```yaml
  ---
  title: "Writing useful comments"
  date: 2021-06-05
  url: "/writing-useful-comments" # <--- THIS
  ---
  ```

  The `url` part is only added if you are storing the images alongside your
  post.

  Note that the images using the syntax `![]()` tag must span a single line.
  Otherwise, it won't be transformed.

  ```sh
  % ls --tree ./content/2020/avoid-gke-lb-using-hostport
  ./content/2020/avoid-gke-lb-using-hostport
  ├── cost-load-balancer-gke.png
  ├── cover-external-dns.png
  ├── how-service-controller-works-on-gke.png            # The image.
  ├── index.md                                           # The post.
  └── packet-routing-with-akrobateo.png
  ```

  If your images are stored in the `static` directory, it should still work.

  Since you can also embed `<img>` tags in markdown, these are also converted.
  For example:

  ```markdown
  <img src="dnat-google-vpc-how-comes-back.svg"/>
  ```

  becomes:

  ```text
  <img src="https://maelvls.dev/you-should-write-comments/dnat-google-vpc-how-comes-back.svg"/>
            <------------------><----------------------->
                   base_url                url
              (from config.yaml)    (from front matter)
  ```

  Like above, the HTML `<img>` tag must span a single line.

  Only the following image extensions are converted: png, PNG, jpeg, JPG, jpg,
  gif, GIF, svg, SVG.

- **Anchor IDs**: The GitHub-style anchor IDs are converted to Devto anchor IDs.
  This is because GitHub-style anchor IDs, which is what Hugo produces, are
  different from the ones produced by Devto. For example, take the following
  Markdown:

  ```markdown
  [`go get -u` vs. `go.mod` (= _*Problem*_)](#go-get--u-vs-gomod--_problem_)
  ```

  becomes:

  ```markdown
  [`go get -u` vs. `go.mod` (= _*Problem*_)](#-raw-go-get-u-endraw-vs-raw-gomod-endraw-problem)
  ```

> [!NOTE]
>
> Hugo uses soft breaks for new lines as per the CommonMark spec, but dev.to
> uses the "Markdown Here" conventions which use a hard break on new lines; to
> work around that, see the below
> [section](#hugos-hard-breaks-versus-devto-hard-breaks).

### Features

#### Preview and diff changes

You can look at all the changes that will be pushed to dev.to:

```sh
hudevto diff
```

If you want to render the Markdown file that will be pushed to dev.to, you can
use the `preview` command:

```sh
hudevto preview ./content/2020/avoid-gke-lb-using-hostport/index.md
```

#### List your dev.to articles

```sh
hudevto devto list
```

This is useful because I have dev.to configured with the RSS feed of my blog so
that dev.to automatically creates a draft of each of my new posts. Note that you
don't need to set up RSS mirroring in order to use `hudevto`.

```console
$ hudevto devto list
410260: unpublished at https://dev.to/maelvls/it-s-always-the-dns-fault-3lg3-temp-slug-8953915/edit (It's always the DNS' fault)
365847: unpublished at https://dev.to/maelvls/stuff-about-wireshark-28c-temp-slug-8030102/edit (Stuff about Wireshark)
365846: unpublished at https://dev.to/maelvls/how-client-server-ssh-authentication-works-5e7-temp-slug-7868012/edit (How client-server SSH authentication works)
313908: unpublished at https://dev.to/maelvls/about-3896-temp-slug-7318594/edit (About)
365849: published at https://dev.to/maelvls/epic-journey-with-statically-and-dynamically-linked-libraries-a-so-1khn (Epic journey with statically and dynamically-linked libraries (.a, .so))
331169: published at https://dev.to/maelvls/github-actions-with-a-private-terraform-module-5b85 (Github Actions with a private Terraform module)
317339: published at https://dev.to/maelvls/learning-kubernetes-controllers-496j (Learning Kubernetes Controllers)
```

## Notes

### Hugo's hard breaks versus dev.to hard breaks

One major difference between Hugo and dev.to markdown is that Hugo uses
soft breaks whenever it parses a new lines (as per the CommonMark spec); on
the other side, dev.to uses the "Markdown Here" conventions where a hard
break is used when a new line is parsed.

I was not able to find a way to do the transformation in `hudevto` itself.
What I currently do is to keep my hugo blog source with lines "unwrapped"
since I used to wrap my markdown files at 80 characters.

To "unwrap" all your markdown line from 80 chars to "no width limit", you
can use `prettier`:

```sh
npm i -g prettier
prettier --write --prose-wrap=never content/**/*.md
```

### Known errors

**Validation failed: Canonical url has already been taken** means that
another article of yours exists with the same `canonical_url` field in its
front matter; it often means that there is a duplicate article.

**Validation failed: Body markdown has already been taken** means that the
same markdown body already existings in one of your articles on dev.to.
Often means that there is a duplicate article.

**Validation failed: (<unknown>): could not find expected ':' while scanning a simple key at line 4 column 1**: you can use the command

```sh
hudevto preview ./content/2020/gh-actions-with-tf-private-repo/index.md
```

to see what is being uploaded to dev.to. I often got this error when trying
to do a multi-line description. I had to change from:

```yaml
description: |
  We often talk about avoiding unecessary comments that needlessly paraphrase
  what the code does. In this article, I gathered some thoughts about why
  writing comments is as important as writing the code itself.
```

to:

```yaml
description: "We often talk about avoiding unecessary comments that needlessly paraphrase what the code does. In this article, I gathered some thoughts about why writing comments is as important as writing the code itself."
```
