
## Notes

**Validation failed: Canonical url has already been taken** means that
another article of yours exists with the same `canonical_url` field in its
front matter; it often means that there is a duplicate article.

**Validation failed: Body markdown has already been taken** means that the
same markdown body already existings in one of your articles on dev.to.
Often means that there is a duplicate article.

**Validation failed: (<unknown>): could not find expected ':' while scanning a simple key at line 4 column 1**: you can use the command

```sh
hudevto push --markdown ./content/2020/gh-actions-with-tf-private-repo/index.md
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
