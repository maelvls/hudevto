# hudevto, the CLI for pushing and synchronizing your Hugo blog posts to Dev.to

![Screenshot of the hudevto push command](https://user-images.githubusercontent.com/2195781/108324642-737e7f80-71c8-11eb-9e4f-8f23fd14d644.png)

**Content:**

- [Install](#install)
- [Usage](#usage)
- [Use it](#use-it)
  - [List your dev.to articles](#list-your-devto-articles)
  - [Preview the Markdown content that will be pushed to dev.to](#preview-the-markdown-content-that-will-be-pushed-to-devto)
  - [Push one blog post to dev.to](#push-one-blog-post-to-devto)
  - [Push all blog posts to dev.to](#push-all-blog-posts-to-devto)
- [Notes](#notes)
  - [Hugo's hard breaks versus dev.to hard breaks](#hugos-hard-breaks-versus-devto-hard-breaks)
  - [Known errors](#known-errors)

## Install

```sh
# Requirement: Go is installed and $(go env GOPATH)/bin is in your PATH.
(cd && GO111MODULE=on go get github.com/maelvls/hudevto@latest)
```

## Usage

```sh
% hudevto help
hudevto allows you to synchronize your Hugo posts with your DEV articles. The
synchronization is one way (Hugo to DEV). A Hugo post is only pushed when a
change is detected. When pushed to DEV, the Hugo article is applied two
transformations: the relative image links are absolutified, and the Hugo tags
are turned into Liquid tags.

USAGE
  hudevto [OPTION] (preview|diff) POST
  hudevto [OPTION] status [POST]
  hudevto [OPTION] push [POST]
  hudevto [OPTION] devto list

DESCRIPTION
In order to operate, hudevto requires you to have your DEV account configured
with "Publish to DEV Community from your blog's RSS". You can configure that at
https://dev.to/settings/extensions. DEV will create a draft article for
every Hugo post that you have published on your blog. For example, Let us
imagine that your Hugo blog layout is:

    .
    └── content
       ├── brick-chest.md
       ├── cloth-impossible.md
       └── powder-farmer.md

After configuring the RSS feed of your blog at https://maelvls.dev/index.xml,
DEV should create one draft article per post. You can check that these articles
have been created on DEV with:

    % hudevto devto list
    386001: unpublished at https://dev.to/maelvls/brick-chest/edit
    386002: unpublished at https://dev.to/maelvls/cloth-impossible/edit
    386003: unpublished at https://dev.to/maelvls/powder-farmer/edit

The next step is to map each article that you want to sync to DEV. Let us see
the state of the mapping:

    % hudevto status
    error: ./content/brick-chest.md: missing devtoId field in front matter, might be 386001: https://dev.to/maelvls/brick-chest/edit
    error: ./content/cloth-impossible.md: missing devtoId field in front matter, might be 386002: https://dev.to/maelvls/cloth-impossible/edit
    error: ./content/powder-farmer.md: missing devtoId field in front matter, might be 386003: https://dev.to/maelvls/powder-farmer/edit

At this point, you need to open each of your Hugo post and add some fields to
their front matters. For example, in ./content/brick-chest.md, we add this:

    devtoId: 386001       # This is the DEV ID as seen in hudevto devto list
    devtoPublished: true  # When false, the DEV article will stay a draft
    devtoSkip: false      # When true, hudevto will ignore this post.

The status should have changed:

    % hudevto status
    info: ./content/brick-chest.md will be pushed published to https://dev.to/maelvls/brick-chest/edit
    info: ./content/cloth-impossible.md will be pushed published to https://dev.to/maelvls/cloth-impossible/edit
    info: ./content/powder-farmer.md will be pushed published to https://dev.to/maelvls/powder-farmer/edit

Finally, you can push to DEV:

    % hudevto push
    success: ./content/brick-chest.md pushed to https://dev.to/maelvls/brick-chest-2588
    success: ./content/cloth-impossible.md pushed to https://dev.to/maelvls/cloth-impossible-95dc
    success: ./content/powder-farmer.md pushed to https://dev.to/maelvls/powder-farmer-6a18


COMMANDS
  hudevto status [POST]
      Shows the status of each post (or of a single post). The status shows
      whether it is mapped to a DEV article and if a push is required when the
      Hugo post has changes that are not on DEV yet.
  hudevto preview POST
      Displays a Markdown preview of the Hugo post that has been converted into
      the DEV article Markdown format. You can use this command to check that
      the tranformations were correctly applied.
  hudevto diff POST
      Displays a diff between the Hugo post and the DEV article. It is useful
      when you want to see what changes will be pushed.
  hudevto push [POST]
      Pushes the given Hugo Markdown post to DEV. If no post is given, then
      all posts are pushed.
  hudevto devto list
      Lists all the articles you have on your DEV account.

OPTIONS
  -apikey string
    	The API key for Dev.to. You can also set DEVTO_APIKEY instead.
  -debug
    	Print debug information such as the HTTP requests that are being made in curl format.
  -root string
    	Root directory of the Hugo project.
```

## Use it

First, copy your dev.to token from your dev.to settings and set it as an
environment variable:

```sh
export DEVTO_APIKEY=$(lpass show dev.to -p)
```

### List your dev.to articles

This is useful because I have dev.to configured with the RSS feed of my
blog so that dev.to automatically creates a draft of each of my new posts.

```sh
% hudevto devto list
410260: unpublished at https://dev.to/maelvls/it-s-always-the-dns-fault-3lg3-temp-slug-8953915/edit (It's always the DNS' fault)
365847: unpublished at https://dev.to/maelvls/stuff-about-wireshark-28c-temp-slug-8030102/edit (Stuff about Wireshark)
365846: unpublished at https://dev.to/maelvls/how-client-server-ssh-authentication-works-5e7-temp-slug-7868012/edit (How client-server SSH authentication works)
313908: unpublished at https://dev.to/maelvls/about-3896-temp-slug-7318594/edit (About)
365849: published at https://dev.to/maelvls/epic-journey-with-statically-and-dynamically-linked-libraries-a-so-1khn (Epic journey with statically and dynamically-linked libraries (.a, .so))
331169: published at https://dev.to/maelvls/github-actions-with-a-private-terraform-module-5b85 (Github Actions with a private Terraform module)
317339: published at https://dev.to/maelvls/learning-kubernetes-controllers-496j (Learning Kubernetes Controllers)
```

### Preview the Markdown content that will be pushed to dev.to

I use the `hudevto preview` command because I do some transformations and I need a way to preview the changes to make sure the Markdown and front matter make sense. The transformations are:

- Generate a new front matter which is used by dev.to for setting the dev.to post title and canonical URL;
- Change the Hugo "tags" into Liquid tags, such as:

  ```md
  {{< youtube 30a0WrfaS2A >}}
  ```

  is changed to the Liquid tag:

  ```md
  {% youtube 30a0WrfaS2A %}
  ```

- Add the base URL of the post to the markdown images so that images are not broken. ONLY WORKS if your images are stored along side your blog post, such as:

  ```sh
  % ls --tree ./content/2020/avoid-gke-lb-using-hostport
  ./content/2020/avoid-gke-lb-using-hostport
  ├── cost-load-balancer-gke.png
  ├── cover-external-dns.png
  ├── how-service-controller-works-on-gke.png
  ├── index.md                                             # The actual blog post.
  └── packet-routing-with-akrobateo.png
  ```

- I want to be able to add the base post URL to each image. For example,
  imagine that the post is

  https://maelvls.dev/you-should-write-comments/index.md

  then I need to replace the images, such as:

  ```markdown
  ![My image](cover-you-should-write-comments.png)
  ```

  with:

  ```text
  ![My image](/you-should-write-comments/cover-you-should-write-comments.png)
              <------ basePostURL ------>
              (basePostURL includes the leading / and trailing /)
  ```

  Since you can also embed `<img>` tags in markdown, these are also converted. For example,

  ```markdown
  <img alt="Super example" src="dnat-google-vpc-how-comes-back.svg" width="80%"/>
  ```

  becomes:

  ```markdown
  <img alt="Super example" src="/you-should-write-comments/dnat-google-vpc-how-comes-back.svg" width="80%"/>
  ```

  Only the following image extensions are converted: png, PNG, jpeg, JPG, jpg,
  gif, GIF, svg, SVG.

- The GitHub-style anchor IDs are converted to Devto anchor IDs. This is because
  GitHub-style anchor IDs, which is what Hugo produces, are different from the
  ones produced by Devto. For example, take the following Markdown:

  ```markdown
  [`go get -u` vs. `go.mod` (= _*Problem*_)](#go-get--u-vs-gomod--_problem_)
  ```

  becomes:

  ```markdown
  [`go get -u` vs. `go.mod` (= _*Problem*_)](#-raw-go-get-u-endraw-vs-raw-gomod-endraw-problem)
  ```

**Note:** that Hugo uses soft breaks for new lines as per the CommonMark
spec, but dev.to uses the "Markdown Here" conventions which use a hard
break on new lines; to work around that, see the below
[section](#hugos-hard-breaks-versus-devto-hard-breaks).

```sh
% hudevto preview ./content/2020/avoid-gke-lb-using-hostport/index.md
---
title: "Avoid GKE's expensive load balancer by using hostPort"
description: "I want to avoid using the expensive Google Network Load Balancer and instead do the load balancing in-cluster using akrobateo, which acts as a LoadBalancer controller."
published: true
tags: ""
date: 20200120T00:00Z
series: ""
canonical_url: "https://maelvls.dev/avoid-gke-lb-with-hostport/"
cover_image: "https://maelvls.dev/avoid-gke-lb-with-hostport/cover-external-dns.png"
---

> **⚠️ Update 25 April 2020**: Akrobateo has been EOL in January 2020 due to the company going out of business. Their blog post regarding the EOL isn't available anymore and was probably shut down. Fortunately, the Wayback Machine [has a snapshot of the post](https://web.archive.org/web/20200107111252/https://blog.kontena.io/farewell/) (7th January 2020). Here is an excerpt:
>
> > This is a sad day for team Kontena. We tried to build something amazing but our plans of creating business around open source software has failed. We couldn't build a sustainable business. Despite all the effort, highs and lows, as of today, Kontena has ceased operations. The team is no more and the official support for Kontena products is no more available.
>
> This is so sad... 😢 Note that the Github repo [kontena/akrobateo](https://github.com/kontena/akrobateo) is still there (and has not been archived yet), but their Docker registry has been shut down which means most of this post is broken.

In my spare time, I maintain a tiny "playground" Kubernetes cluster on [GKE](https://cloud.google.com/kubernetes-engine) (helm charts [here](https://github.com/maelvls/k.maelvls.dev)). I quickly realized that realized using `Service type=LoadBalancer` in GKE was spawning a _[Network Load Balancer](https://cloud.google.com/load-balancing/docs/network)_ which costs approximately **\$15 per month**! In this post, I present a way of avoiding the expensive Google Network Load Balancer by load balancing in-cluster using akrobateo, which acts as a Service type=LoadBalancer controller.
```

### Push one blog post to dev.to

```sh
% hudevto push ./content/2020/avoid-gke-lb-using-hostport/index.md
success: ./content/2020/avoid-gke-lb-using-hostport/index.md pushed published to https://dev.to/maelvls/avoid-gke-s-expensive-load-balancer-by-using-hostport-2ab9 (devtoId: 241275, devtoPublished: true)
```

### Push all blog posts to dev.to

```sh
% hudevto push
success: ./content/notes/dns.md pushed unpublished to https://dev.to/maelvls/it-s-always-the-dns-fault-3lg3-temp-slug-8953915/edit (devtoId: 410260, devtoPublished: false)
success: ./content/2020/deployment-available-condition/index.md pushed published to https://dev.to/maelvls/understanding-the-available-condition-of-a-kubernetes-deployment-51li (devtoId: 386691, devtoPublished: true)
success: ./content/2020/docker-proxy-registry-kind/index.md pushed published to https://dev.to/maelvls/pull-through-docker-registry-on-kind-clusters-cpo (devtoId: 410837, devtoPublished: true)
success: ./content/2020/mitmproxy-kubectl/index.md pushed published to https://dev.to/maelvls/using-mitmproxy-to-understand-what-kubectl-does-under-the-hood-36om (devtoId: 377876, devtoPublished: true)
success: ./content/2020/static-libraries-and-autoconf-hell/index.md pushed published to https://dev.to/maelvls/epic-journey-with-statically-and-dynamically-linked-libraries-a-so-1khn (devtoId: 365849, devtoPublished: true)
success: ./content/2020/gh-actions-with-tf-private-repo/index.md pushed published to https://dev.to/maelvls/github-actions-with-a-private-terraform-module-5b85 (devtoId: 331169, devtoPublished: true)
...
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
