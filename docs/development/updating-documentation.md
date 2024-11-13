# Updating Documentation

All documentation for `etcd-druid` resides in [docs](https://github.com/gardener/etcd-druid/tree/master/docs) directory. If you wish to update the existing documentation or create new documentation files then read on.

## Prerequisite: Setup mkdocs locally
[Material for mkdocs](https://squidfunk.github.io/mkdocs-material/) is used to generate github pages out of all the markdown files that are present under [docs](https://github.com/gardener/etcd-druid/tree/master/docs). To be able to locally validate that the documentation is rendered correctly it is recommended that you do the following setup.

* Install python3 if not already installed.
* Setup a virtual environment via 
```bash
python -m venv venv
```
* Activate the virtual environment 
```bash
source venv/bin/activate
```
* In the virtual environment install the packages.
```bash
(venv) > pip install mkdocs-material
(venv) > pip install pymdown-extensions
(venv) > pip install mkdocs-glightbox
(venv) > pip install mkdocs-pymdownx-material-extras
```
!!! note
    Complete list of packages installed should be in sync with [Github Actions Configuration](https://github.com/gardener/etcd-druid/blob/master/.github/workflows/publish-docs.yml#L23-L27).
* Serve the documentation
```bash
(venv) > mkdocs serve
```
You can now view the rendered documentation at `localhost:8000`. Any changes that you make to the docs will get hot-reloaded and you can immediately view the changes.

## Updating documentation

All documentation _should_ be in `markdown` _only_. Ensure that you take care of the following:

* [index.md](https://github.com/gardener/etcd-druid/blob/master/docs/index.md) is the home page for the documentation rendered as github pages. Please do not remove this file.
* If you are using a new feature (that is not already used) by `mkdocs` then ensure that it is properly configured in [mkdocs.yml](https://github.com/gardener/etcd-druid/blob/master/mkdocs.yml) and if new plugins or markdown extensions are used then ensure that you update [Github Actions Configuration](https://github.com/gardener/etcd-druid/blob/master/.github/workflows/publish-docs.yml#L23-L27) as well.
* If new files are getting added and you wish to show these files in github pages then ensure that you have added these files under appropriate sections under [navigation](https://github.com/gardener/etcd-druid/blob/master/mkdocs.yml#L70).
* If you are linking any file outside the [docs](https://github.com/gardener/etcd-druid/tree/master/docs) directory then relative links will not work on github pages. Please get the `https` link to the file or section of the file that you wish to link.

## Raise a Pull Request

Once you have done the documentation changes then follow the guide on [how to raise a PR](raising-a-pr.md).

!!! info
    Once the documentation update PR has been merged, you will be able to see the updated documentation [here](https://gardener.github.io/etcd-druid/).


